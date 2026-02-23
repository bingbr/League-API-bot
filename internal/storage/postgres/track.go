package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/jackc/pgx/v5"
)

type TrackGuildConfig struct {
	GuildID   string
	ChannelID string
	UpdatedAt time.Time
}

type TrackedAccount struct {
	GuildID        string
	PlatformRegion string
	PUUID          string
	NickName       string
	TagLine        string
	AddedBy        string
	AddedAt        time.Time
}

func (a TrackedAccount) RiotID() string {
	return riot.FormatRiotID(a.NickName, a.TagLine)
}

func (db *Database) CreateTrackTable(ctx context.Context) error {
	return db.withTx(ctx, func(tx pgx.Tx) error {
		b := &pgx.Batch{}
		b.Queue(createTrackGuildConfigSQL)
		b.Queue(createTrackAccountsSQL)
		b.Queue(createTrackAccountsLookupIdxSQL)
		b.Queue(createTrackMatchNotificationsSQL)
		b.Queue(createTrackMatchNotificationsPostIdxSQL)
		b.Queue(createTrackMatchNotificationsMatchIDIdxSQL)
		b.Queue(createTrackMatchNotificationsUpdatedAtIdxSQL)
		b.Queue(createTrackMatchSnapshotsSQL)
		b.Queue(createTrackMatchSnapshotsUpdatedAtIdxSQL)
		if err := executeBatch(ctx, tx, b); err != nil {
			return fmt.Errorf("create track schema: %w", err)
		}
		return nil
	})
}

func (db *Database) UpsertTrackGuildConfig(ctx context.Context, guildID, channelID string) error {
	if err := db.ensureReady(); err != nil {
		return err
	}

	guildID, channelID = strings.TrimSpace(guildID), strings.TrimSpace(channelID)
	query := `
	INSERT INTO track_guild_config (guild_id, channel_id)
	VALUES ($1, $2)
	ON CONFLICT (guild_id) DO UPDATE
	SET channel_id = excluded.channel_id,
		updated_at = now()`
	if _, err := db.pool.Exec(ctx, query, guildID, channelID); err != nil {
		return fmt.Errorf("upsert track guild config %s: %w", guildID, err)
	}
	return nil
}

func (db *Database) DisableTrackGuildConfig(ctx context.Context, guildID string) error {
	if err := db.ensureReady(); err != nil {
		return err
	}

	guildID = strings.TrimSpace(guildID)
	query := `
	UPDATE track_guild_config
	SET channel_id = '',
		updated_at = now()
	WHERE guild_id = $1`
	if _, err := db.pool.Exec(ctx, query, guildID); err != nil {
		return fmt.Errorf("disable track guild config %s: %w", guildID, err)
	}
	return nil
}

func (db *Database) TrackGuildConfig(ctx context.Context, guildID string) (TrackGuildConfig, bool, error) {
	if err := db.ensureReady(); err != nil {
		return TrackGuildConfig{}, false, err
	}

	guildID = strings.TrimSpace(guildID)
	query := `
	SELECT guild_id, channel_id, updated_at
	FROM track_guild_config
	WHERE guild_id = $1`
	var cfg TrackGuildConfig
	err := db.pool.QueryRow(ctx, query, guildID).Scan(&cfg.GuildID, &cfg.ChannelID, &cfg.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return TrackGuildConfig{}, false, nil
		}
		return TrackGuildConfig{}, false, fmt.Errorf("get track guild config %s: %w", guildID, err)
	}
	cfg.UpdatedAt = cfg.UpdatedAt.UTC()
	return cfg, true, nil
}

func (db *Database) AddTrackedAccount(ctx context.Context, account TrackedAccount) (bool, error) {
	if err := db.ensureReady(); err != nil {
		return false, err
	}

	account.GuildID = strings.TrimSpace(account.GuildID)
	account.PlatformRegion = riot.NormalizePlatformRegion(account.PlatformRegion)
	account.PUUID = strings.TrimSpace(account.PUUID)
	account.NickName = strings.TrimSpace(account.NickName)
	account.TagLine = strings.TrimPrefix(strings.TrimSpace(account.TagLine), "#")
	account.AddedBy = strings.TrimSpace(account.AddedBy)
	query := `
	WITH inserted AS (
		INSERT INTO track_accounts (guild_id, puuid, platform_region, game_name, tag_line, added_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (guild_id, puuid) DO NOTHING
		RETURNING true AS created
	),
	updated AS (
		UPDATE track_accounts
		SET platform_region = $3,
			game_name = $4,
			tag_line = $5
		WHERE guild_id = $1
		AND puuid = $2
		AND NOT EXISTS (SELECT 1 FROM inserted)
		RETURNING false AS created
	)
	SELECT created FROM inserted
	UNION ALL
	SELECT created FROM updated
	LIMIT 1`
	var created bool
	if err := db.pool.QueryRow(ctx, query, account.GuildID, account.PUUID, account.PlatformRegion, account.NickName, account.TagLine, account.AddedBy).Scan(&created); err != nil {
		return false, fmt.Errorf("upsert tracked account %s/%s: %w", account.GuildID, account.PUUID, err)
	}
	return created, nil
}

func (db *Database) RemoveTrackedAccount(ctx context.Context, guildID, riotID string) (bool, error) {
	if err := db.ensureReady(); err != nil {
		return false, err
	}

	guildID = strings.TrimSpace(guildID)
	gameName, tagLine, err := riot.SplitRiotID(riotID)
	if err != nil {
		return false, err
	}
	query := `
	DELETE FROM track_accounts
	WHERE guild_id = $1
		AND lower(game_name) = lower($2)
		AND lower(tag_line) = lower($3)`
	result, err := db.pool.Exec(ctx, query, guildID, gameName, tagLine)
	if err != nil {
		return false, fmt.Errorf("remove tracked account %s/%s#%s: %w", guildID, gameName, tagLine, err)
	}
	return result.RowsAffected() > 0, nil
}

func (db *Database) ListTrackedAccounts(ctx context.Context, guildID string, limit int) ([]TrackedAccount, error) {
	if err := db.ensureReady(); err != nil {
		return nil, err
	}

	guildID = strings.TrimSpace(guildID)
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	query := `
	SELECT guild_id,
       platform_region,
       puuid,
       game_name,
       tag_line,
       added_by,
       added_at
	FROM track_accounts
	WHERE guild_id = $1
	ORDER BY lower(game_name), lower(tag_line)
	LIMIT $2`
	rows, err := db.pool.Query(ctx, query, guildID, limit)
	if err != nil {
		return nil, fmt.Errorf("list tracked accounts for %s: %w", guildID, err)
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[TrackedAccount])
	if err != nil {
		return nil, fmt.Errorf("collect tracked accounts: %w", err)
	}
	for i := range out {
		out[i].AddedAt = out[i].AddedAt.UTC()
	}

	return out, nil
}

const createTrackGuildConfigSQL = `
CREATE TABLE IF NOT EXISTS track_guild_config (
    guild_id text PRIMARY KEY,
    channel_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
)`

const createTrackAccountsSQL = `
CREATE TABLE IF NOT EXISTS track_accounts (
    guild_id text NOT NULL REFERENCES track_guild_config (guild_id) ON DELETE CASCADE,
    puuid text NOT NULL,
    platform_region text NOT NULL,
    game_name text NOT NULL,
    tag_line text NOT NULL,
    added_by text NOT NULL DEFAULT '',
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, puuid)
)`

const createTrackAccountsLookupIdxSQL = `
CREATE INDEX IF NOT EXISTS track_accounts_lookup_idx
ON track_accounts (guild_id, lower(game_name), lower(tag_line))`
