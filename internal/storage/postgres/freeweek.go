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

func (db *Database) CreateFreeWeekTable(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS riot_free_week_rotations (
		platform_region text PRIMARY KEY,
		free_champion_ids int[] NOT NULL DEFAULT '{}',
		free_champion_ids_for_new_players int[] NOT NULL DEFAULT '{}',
		max_new_player_level int NOT NULL,
		fetched_at timestamptz NOT NULL,
		expires_at timestamptz NOT NULL
	)`
	return db.createTable(ctx, query, "create free week schema")
}

func (db *Database) UpsertFreeWeekRotation(ctx context.Context, platformRegion string, rotation riot.ChampionRotation, fetchedAt, expiresAt time.Time) error {
	if err := db.ensureReady(); err != nil {
		return err
	}

	platformRegion = strings.ToLower(strings.TrimSpace(platformRegion))
	if fetchedAt.IsZero() {
		fetchedAt = time.Now().UTC()
	}
	if expiresAt.IsZero() {
		expiresAt = fetchedAt.Add(24 * time.Hour)
	}
	query := `
	INSERT INTO riot_free_week_rotations (
		platform_region,
		free_champion_ids,
		free_champion_ids_for_new_players,
		max_new_player_level,
		fetched_at,
		expires_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (platform_region) DO UPDATE
	SET free_champion_ids = excluded.free_champion_ids,
		free_champion_ids_for_new_players = excluded.free_champion_ids_for_new_players,
		max_new_player_level = excluded.max_new_player_level,
		fetched_at = excluded.fetched_at,
		expires_at = excluded.expires_at`
	if _, err := db.pool.Exec(ctx,
		query,
		platformRegion, rotation.FreeChampionIDs, rotation.FreeChampionIDsForNewPlayers, rotation.MaxNewPlayerLevel, fetchedAt.UTC(), expiresAt.UTC(),
	); err != nil {
		return fmt.Errorf("upsert free week rotation %s: %w", platformRegion, err)
	}
	return nil
}

func (db *Database) GetFreeWeekRotation(ctx context.Context, platformRegion string) (riot.ChampionRotation, time.Time, time.Time, bool, error) {
	if err := db.ensureReady(); err != nil {
		return riot.ChampionRotation{}, time.Time{}, time.Time{}, false, err
	}

	platformRegion = strings.ToLower(strings.TrimSpace(platformRegion))
	query := `
	SELECT free_champion_ids,
		free_champion_ids_for_new_players,
		max_new_player_level,
		fetched_at,
		expires_at
	FROM riot_free_week_rotations
	WHERE platform_region = $1`
	var maxNewPlayerLevel int
	var fetchedAt, expiresAt time.Time
	var freeChampionIDs, freeChampionIDsForNewPlayers []int
	err := db.pool.QueryRow(ctx, query, platformRegion).Scan(&freeChampionIDs, &freeChampionIDsForNewPlayers, &maxNewPlayerLevel, &fetchedAt, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return riot.ChampionRotation{}, time.Time{}, time.Time{}, false, nil
		}
		return riot.ChampionRotation{}, time.Time{}, time.Time{}, false, fmt.Errorf("get free week rotation %s: %w", platformRegion, err)
	}

	rotation := riot.ChampionRotation{
		FreeChampionIDs:              freeChampionIDs,
		FreeChampionIDsForNewPlayers: freeChampionIDsForNewPlayers,
		MaxNewPlayerLevel:            maxNewPlayerLevel,
	}
	return rotation, fetchedAt.UTC(), expiresAt.UTC(), true, nil
}
