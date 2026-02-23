package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/bingbr/League-API-bot/internal/config"
	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/discord/commands"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage/logs"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bingbr/League-API-bot/internal/tracknotify"
)

const (
	dbTimeout             = 5 * time.Second
	syncTimeout           = 5 * time.Minute
	cdnSyncInterval       = 24 * time.Hour
	riotValidationTimeout = 10 * time.Second
	riotValidationRegion  = "br1"
)

func Run(ctx context.Context, cfg config.Config) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Setup Database
	db, closeDB, err := connectDB(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer closeDB()

	// Initial check for log schema
	if db != nil {
		if err := db.CreateLogTable(ctx); err != nil {
			return fmt.Errorf("init log schema: %w", err)
		}
		if err := db.CreateTrackTable(ctx); err != nil {
			return fmt.Errorf("init track schema: %w", err)
		}
	}

	// Setup Logger, validate Riot API and create Discord Bot
	logger := setupLogger(cfg, db)
	if loaded, err := riot.ConfigureRateLimitsFromFile(cfg.RateLimitCfg); err != nil {
		return fmt.Errorf("configure riot rate limits: %w", err)
	} else if loaded {
		logger.Info("Riot rate limits configured", "path", cfg.RateLimitCfg)
	}
	if err := validateRiotAPIKeyOnStartup(ctx, cfg.RiotAPIKey, logger); err != nil {
		return err
	}
	commands.ConfigureRuntime(commands.Runtime{RiotAPIKey: cfg.RiotAPIKey, Database: db})
	bot, err := discord.NewBot(cfg.DiscordToken, cfg.GuildID, cfg.IsDev, discord.WithRegistry(buildRegistry()), discord.WithLogger(logger))
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	// Background Services
	if err := cdn.StartAutoUpdate(ctx, cdnSyncInterval, logger); err != nil {
		logger.Warn("Failed to init version refresher", "err", err)
	}

	// Async Tasks (Schemas, CDN, Emojis)
	cancelAsync := func() {}
	if db != nil {
		asyncCtx, cancel := context.WithCancel(ctx)
		cancelAsync = cancel
		goSafe(logger, "run_async_tasks", func() {
			runAsyncTasks(asyncCtx, db, bot.Session(), cfg, logger)
		})
	}
	defer cancelAsync()
	return bot.Run()
}

func validateRiotAPIKeyOnStartup(ctx context.Context, apiKey string, logger *slog.Logger) error {
	if strings.TrimSpace(apiKey) == "" {
		return fmt.Errorf("validate riot api key: key is empty")
	}
	checkCtx, cancel := context.WithTimeout(ctx, riotValidationTimeout)
	defer cancel()
	if _, err := riot.FetchChampionRotation(checkCtx, riotValidationRegion, apiKey); err != nil {
		if logger != nil {
			logger.Error("Riot API key validation failed", "region", riotValidationRegion, "error", err)
		}
		return fmt.Errorf("validate riot api key: %w", err)
	}

	if logger != nil {
		logger.Info("Riot API key validated", "region", riotValidationRegion)
	}
	return nil
}

func connectDB(ctx context.Context, url string) (*postgres.Database, func(), error) {
	if url == "" {
		return nil, func() {}, nil
	}
	// Retry logic to handle transient connection issues at startup, such as the database not being ready yet.
	var lastErr error
	for i := range 3 {
		if i > 0 {
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, nil, fmt.Errorf("database connection canceled: %w", ctx.Err())
			case <-timer.C:
			}
		}
		tCtx, cancel := context.WithTimeout(ctx, dbTimeout)
		pool, err := postgres.NewPool(tCtx, url)
		cancel()

		if err == nil {
			return postgres.NewDB(pool), pool.Close, nil
		}
		lastErr = err
	}
	return nil, nil, fmt.Errorf("database connection failed after retries: %w", lastErr)
}

func setupLogger(cfg config.Config, database *postgres.Database) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	var handler slog.Handler = slog.NewTextHandler(os.Stderr, opts)

	if database != nil {
		dbHandler := logs.NewDBHandler(database, slog.LevelDebug, 2*time.Second)
		handler = slog.NewMultiHandler(handler, dbHandler)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	if cfg.IsDev {
		logger.Info("Development mode enabled")
	}
	return logger
}

func runAsyncTasks(ctx context.Context, db *postgres.Database, session *discordgo.Session, cfg config.Config, logger *slog.Logger) {
	if err := db.CreateFreeWeekTable(ctx); err != nil {
		logger.Error("Schema error (freeweek)", "err", err)
		return
	}
	if err := db.CreateRiotCDNTable(ctx); err != nil {
		logger.Error("Schema error (riot_cdn)", "err", err)
		return
	}
	if strings.TrimSpace(cfg.RiotAPIKey) != "" && session != nil {
		notifier := tracknotify.NewService(db, session, cfg.RiotAPIKey, logger)
		goSafe(logger, "track_notify_loop", func() {
			notifier.Run(ctx)
		})
	}

	// Initial sync at startup
	syncCDN(ctx, db, session, logger)

	// Keep data/emojis updated without restart.
	ticker := time.NewTicker(cdnSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			syncCDN(ctx, db, session, logger)
		}
	}
}

// Run CDN & Emoji Sync
func syncCDN(ctx context.Context, db *postgres.Database, session *discordgo.Session, logger *slog.Logger) {
	versions, err := cdn.NewClient().FetchVersions(ctx)
	if err != nil || len(versions) == 0 {
		logger.Error("Failed to fetch Riot versions", "err", err)
		return
	}
	ver := versions[0]

	// Check if core CDN data needs sync
	syncNonRank := true
	cur, found, err := db.GetRiotCDNSyncVersion(ctx, "cdn_data")
	if err != nil {
		logger.Warn("Failed to read sync version; forcing sync", "err", err)
	} else if found && cur == ver {
		syncNonRank = false
	}

	// Bootstrap safeguard: if emoji manifest is empty, force a full non-rank sync.
	if !syncNonRank {
		entries, entriesErr := db.EmojiEntries(ctx)
		if entriesErr != nil {
			logger.Warn("Failed to read emoji manifest; forcing non-rank emoji sync", "err", entriesErr)
			syncNonRank = true
		} else if len(entries) == 0 {
			syncNonRank = true
		}
	}
	// Metadata safeguard: queue/map tables are required by live/post embeds.
	if !syncNonRank {
		hasQueueAndMapData, metadataErr := db.HasQueueAndMapData(ctx)
		if metadataErr != nil {
			logger.Warn("Failed to verify queue/map metadata; forcing non-rank sync", "err", metadataErr)
			syncNonRank = true
		} else if !hasQueueAndMapData {
			logger.Info("Queue/map metadata missing; forcing non-rank sync")
			syncNonRank = true
		}
	}
	// Start Emoji Sync. Rank sync always runs; non-rank sync only when needed.
	if session != nil {
		errCh, err := cdn.StartApplicationEmojiSync(ctx, session, ver, syncNonRank, db, logger)
		if err != nil {
			logger.Error("Failed to start emoji sync", "err", err)
		} else if errCh != nil {
			// Monitor background sync errors
			goSafe(logger, "monitor_emoji_sync", func() {
				for err := range errCh {
					if err != nil {
						logger.Error("Emoji sync error", "err", err)
					}
				}
			})
		}
	}

	if !syncNonRank {
		return // Already up to date
	}

	logger.Info("Starting CDN metadata sync...", "version", ver)
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()
	if err := cdn.SyncBasicData(ctx, db, ver); err != nil {
		logger.Error("CDN metadata sync failed", "err", err)
		return
	}
	if err := db.UpsertRiotCDNSyncVersion(ctx, "cdn_data", ver); err != nil {
		logger.Warn("Failed to save sync version", "err", err)
	}
	logger.Info("CDN metadata sync complete!", "version", ver)
}

func buildRegistry() *discord.Registry {
	r := discord.NewRegistry()
	r.Add(commands.FreeWeekCommand)
	r.Add(commands.SearchCommand)
	r.Add(commands.TrackCommand)
	r.Add(commands.LeadboardCommand)
	return r
}

func goSafe(logger *slog.Logger, task string, fn func()) {
	if logger == nil {
		logger = slog.Default()
	}
	task = strings.TrimSpace(task)
	if task == "" {
		task = "unnamed"
	}
	if fn == nil {
		logger.Error("Background task not started: nil func", "task", task)
		return
	}

	taskLogger := logger.With("task", task)
	go func() {
		startedAt := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				taskLogger.Error("Background task panicked", "panic", recovered, "elapsed", time.Since(startedAt), "stack", string(debug.Stack()))
			}
		}()
		fn()
	}()
}
