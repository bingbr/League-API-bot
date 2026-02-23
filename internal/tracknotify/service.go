package tracknotify

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage"
	"github.com/bwmarrin/discordgo"
)

const (
	defaultPollInterval     = 10 * time.Second
	defaultLoopTimeout      = 45 * time.Second
	defaultRetentionDays    = 7
	defaultPostPendingLimit = 200
	defaultFetchLimit       = 32
	postRetryBaseDelay      = 15 * time.Second
	postRetryMaxDelay       = 5 * time.Minute
	postAbandonAfter        = 2 * time.Hour
	soloQueueType           = "RANKED_SOLO_5x5"
	queueCategoryPvP        = "kpvp"
)

var (
	errQueueNameUnavailable = errors.New("queue name unavailable")
	errQueueHasNoPostGame   = errors.New("queue has no post-game")
)

type Database interface {
	storage.TrackNotifyDB
	storage.SearchDB
	storage.FreeWeekDB
}

type Service struct {
	database     Database
	session      *discordgo.Session
	riotAPIKey   string
	logger       *slog.Logger
	pollInterval time.Duration
	loopTimeout  time.Duration
}

type guildMatchKey struct {
	GuildID    string
	PlatformID string
	GameID     int64
}

type liveGuildMatch struct {
	Key            guildMatchKey
	GuildID        string
	ChannelID      string
	PlatformRegion string
	PlatformID     string
	Game           *riot.LiveGame
	TrackedByPUUID map[string]string
}

type targetProbeKey struct {
	PlatformRegion string
	PUUID          string
}

type liveFetchStats struct {
	Checked   int
	LiveGames int
	NotInGame int
	Errors    int
}

type trackedPair struct {
	PUUID  string
	RiotID string
}

type guildPlatformKey struct {
	GuildID    string
	PlatformID string
}

func NewService(db Database, session *discordgo.Session, riotAPIKey string, logger *slog.Logger) *Service {
	return &Service{
		database:     db,
		session:      session,
		riotAPIKey:   strings.TrimSpace(riotAPIKey),
		logger:       logger,
		pollInterval: defaultPollInterval,
		loopTimeout:  defaultLoopTimeout,
	}
}

func (s *Service) disableGuildTrackingOnAccessLoss(ctx context.Context, guildID, channelID string, sendErr error) bool {
	if !isDiscordMissingAccess(sendErr) {
		return false
	}
	guildID = strings.TrimSpace(guildID)
	channelID = strings.TrimSpace(channelID)
	if guildID == "" {
		return false
	}
	if err := s.database.DisableTrackGuildConfig(ctx, guildID); err != nil {
		s.logger.Warn("Failed to disable tracking after Discord access loss", "guildID", guildID, "channelID", channelID, "error", err, "cause", sendErr)
		return false
	}
	s.logger.Warn("Disabled tracking for guild after Discord access loss", "guildID", guildID, "channelID", channelID, "cause", sendErr)
	return true
}
