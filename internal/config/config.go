package config

import (
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"
)

const (
	riotAPIKeyLength    = 42
	riotAPIKeyPattern   = `^RGAPI-[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`
	discordTokenPattern = `^[\w-]{23,28}\.[\w-]{6,7}\.[\w-]{27,}$`
	defaultRateLimitCfg = "config.toml"
)

var (
	riotAPIKeyRegex   = regexp.MustCompile(riotAPIKeyPattern)
	discordTokenRegex = regexp.MustCompile(discordTokenPattern)
)

type Config struct {
	DiscordToken string
	GuildID      string
	IsDev        bool
	DatabaseURL  string
	RiotAPIKey   string
	RateLimitCfg string
	LogLevel     slog.Level
}

func Parse() (Config, error) {
	token := strings.TrimSpace(os.Getenv("DISCORD_TOKEN"))
	if token == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is not set")
	}
	if err := validateDiscordToken(token); err != nil {
		return Config{}, err
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	isDev := env == "dev"
	logLevel := inferLogLevel(env)

	guildID := ""
	if isDev {
		guildID = strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID"))
		if guildID == "" {
			return Config{}, fmt.Errorf("DISCORD_GUILD_ID is required in development mode")
		}
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is not set")
	}

	riotAPIKey := strings.TrimSpace(os.Getenv("RIOT_API_KEY"))
	if riotAPIKey == "" {
		return Config{}, fmt.Errorf("RIOT_API_KEY is not set")
	}
	if err := validateRiotAPIKey(riotAPIKey); err != nil {
		return Config{}, err
	}

	rateLimitCfg := strings.TrimSpace(os.Getenv("RIOT_RATE_LIMIT_CONFIG"))
	if rateLimitCfg == "" {
		rateLimitCfg = defaultRateLimitCfg
	}

	return Config{
		DiscordToken: token,
		GuildID:      guildID,
		IsDev:        isDev,
		DatabaseURL:  databaseURL,
		RiotAPIKey:   riotAPIKey,
		RateLimitCfg: rateLimitCfg,
		LogLevel:     logLevel,
	}, nil
}

func inferLogLevel(appEnv string) slog.Level {
	if appEnv == "debug" {
		return slog.LevelDebug
	}
	return slog.LevelInfo
}

func validateRiotAPIKey(key string) error {
	if len(key) != riotAPIKeyLength {
		return fmt.Errorf("RIOT_API_KEY has invalid length %d", len(key))
	}
	if !riotAPIKeyRegex.MatchString(key) {
		return fmt.Errorf("RIOT_API_KEY format is invalid")
	}
	return nil
}

func validateDiscordToken(token string) error {
	if !discordTokenRegex.MatchString(token) {
		return fmt.Errorf("DISCORD_TOKEN format is invalid")
	}
	return nil
}
