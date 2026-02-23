package config

import (
	"log/slog"
	"testing"
)

const (
	validDiscordToken = "abcdefghijklmnopqrstuvwx.ABCDEF.abcdefghijklmnopqrstuvwxyz1"
	validRiotAPIKey   = "RGAPI-a565b300-bfcc-4d63-aa62-6cbdc77e0fd3"
	validDatabaseURL  = "postgresql://user:secret@postgres:5432/db"
)

func TestParse_MissingToken(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("DISCORD_TOKEN", "")
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected error when DISCORD_TOKEN is not set")
	}
}

func TestParse_DevMissingGuild(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected error when DISCORD_GUILD_ID is missing in dev")
	}
}

func TestParse_ProdOk(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "guild")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.IsDev {
		t.Fatalf("expected IsDev false in prod")
	}
	if cfg.GuildID != "" {
		t.Fatalf("expected GuildID empty in prod, got %q", cfg.GuildID)
	}
	if cfg.DatabaseURL != validDatabaseURL {
		t.Fatalf("unexpected DatabaseURL: %q", cfg.DatabaseURL)
	}
	if cfg.RiotAPIKey != validRiotAPIKey {
		t.Fatalf("unexpected RiotAPIKey: %q", cfg.RiotAPIKey)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected info log level in prod, got %v", cfg.LogLevel)
	}
}

func TestParse_RiotAPIKeyValid(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.RiotAPIKey != validRiotAPIKey {
		t.Fatalf("unexpected RiotAPIKey: %q", cfg.RiotAPIKey)
	}
}

func TestParse_RiotAPIKeyInvalidLength(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", "RGAPI-a565b300-bfcc-4d63-aa62-6cbdc77e0fd")

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected invalid RIOT_API_KEY length error")
	}
}

func TestParse_RiotAPIKeyInvalidFormat(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", "RGAPI-a565b300-bfcc-4d63-aa62-6cbdc77e0fg3")

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected invalid RIOT_API_KEY format error")
	}
}

func TestParse_MissingDatabaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", "")
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected missing DATABASE_URL error")
	}
}

func TestParse_MissingRiotAPIKey(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", "")

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected missing RIOT_API_KEY error")
	}
}

func TestParse_DebugEnvUsesDebugLogLevel(t *testing.T) {
	t.Setenv("APP_ENV", "debug")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != slog.LevelDebug {
		t.Fatalf("expected debug log level, got %v", cfg.LogLevel)
	}
}

func TestParse_DevEnvUsesInfoLogLevel(t *testing.T) {
	t.Setenv("APP_ENV", "dev")
	t.Setenv("DISCORD_TOKEN", validDiscordToken)
	t.Setenv("DISCORD_GUILD_ID", "guild")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	cfg, err := Parse()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("expected info log level, got %v", cfg.LogLevel)
	}
}

func TestParse_InvalidDiscordTokenFormat(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	t.Setenv("DISCORD_TOKEN", "not-a-discord-token")
	t.Setenv("DISCORD_GUILD_ID", "")
	t.Setenv("DATABASE_URL", validDatabaseURL)
	t.Setenv("RIOT_API_KEY", validRiotAPIKey)

	_, err := Parse()
	if err == nil {
		t.Fatalf("expected invalid DISCORD_TOKEN format error")
	}
}
