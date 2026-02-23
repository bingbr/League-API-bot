package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
)

type QueueDisplay struct {
	QueueID            int
	Name               string
	GameSelectCategory string
}

type MapDisplay struct {
	MapID int
	Name  string
}

type SummonerSpellDisplay struct {
	SpellID     int
	Name        string
	DiscordIcon string
}

type RuneTreeDisplay struct {
	TreeID      int
	Name        string
	DiscordIcon string
}

type RuneDisplay struct {
	RuneID      int
	TreeID      int
	Name        string
	DiscordIcon string
}

type ItemDisplay struct {
	ItemID string
	Name   string
}

func (db *Database) HasQueueAndMapData(ctx context.Context) (bool, error) {
	if err := db.ensureReady(); err != nil {
		return false, err
	}

	queryQueue := `
	SELECT EXISTS (
		SELECT 1
		FROM riot_cdn_queues
		WHERE COALESCE(name, '') <> ''
	)`
	queryMap := `
	SELECT EXISTS (
		SELECT 1
		FROM riot_cdn_maps
		WHERE map_id > 0
		AND COALESCE(name, '') <> ''
	)`
	var (
		queueExists bool
		mapExists   bool
	)
	if err := db.pool.QueryRow(ctx, queryQueue).Scan(&queueExists); err != nil {
		return false, fmt.Errorf("check queue data existence: %w", err)
	}
	if err := db.pool.QueryRow(ctx, queryMap).Scan(&mapExists); err != nil {
		return false, fmt.Errorf("check map data existence: %w", err)
	}
	return queueExists && mapExists, nil
}

func (db *Database) QueueDisplayByID(ctx context.Context, queueID int) (QueueDisplay, bool, error) {
	query := `
	SELECT queue_id, 
		   COALESCE(name, ''), 
		   COALESCE(game_select_category, '') 
	FROM riot_cdn_queues 
	WHERE queue_id = $1`
	return queryDisplayByID[QueueDisplay](ctx, db, queueID, query, "query queue display by id")
}

func (db *Database) MapDisplayByID(ctx context.Context, mapID int) (MapDisplay, bool, error) {
	query := `
	SELECT map_id,
		COALESCE(name, '')
	FROM riot_cdn_maps
	WHERE map_id = $1`
	return queryDisplayByID[MapDisplay](ctx, db, mapID, query, "query map display by id")
}

func (db *Database) SummonerSpellDisplayByIDs(ctx context.Context, ids []int) (map[int]SummonerSpellDisplay, error) {
	query := `
	SELECT spell_id,
		COALESCE(name, '') AS name,
		COALESCE(discord_icon, '')
	FROM riot_cdn_summoner_spells
	WHERE spell_id = ANY($1)`
	return collectDisplayByPositiveIDs(ctx, db, ids, query,
		"query summoner spell display by ids",
		"collect summoner spell display rows",
		func(row SummonerSpellDisplay) int { return row.SpellID },
	)
}

func (db *Database) RuneTreeDisplayByIDs(ctx context.Context, ids []int) (map[int]RuneTreeDisplay, error) {
	query := `
	SELECT tree_id,
		COALESCE(name, '') AS name,
		COALESCE(discord_icon, '')
	FROM riot_cdn_rune_trees
	WHERE tree_id = ANY($1)`
	return collectDisplayByPositiveIDs(ctx, db, ids, query,
		"query rune tree display by ids",
		"collect rune tree display rows",
		func(row RuneTreeDisplay) int { return row.TreeID },
	)
}

func (db *Database) RuneDisplayByIDs(ctx context.Context, ids []int) (map[int]RuneDisplay, error) {
	query := `
	SELECT rune_id,
       tree_id,
       COALESCE(name, '') AS name,
       COALESCE(discord_icon, '')
	FROM riot_cdn_runes
	WHERE rune_id = ANY($1)`
	return collectDisplayByPositiveIDs(ctx, db, ids, query,
		"query rune display by ids",
		"collect rune display rows",
		func(row RuneDisplay) int { return row.RuneID },
	)
}

func (db *Database) ItemDisplayByIDs(ctx context.Context, ids []int) (map[int]ItemDisplay, error) {
	if err := db.ensureReady(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[int]ItemDisplay{}, nil
	}

	unique := uniquePositiveInt32s(ids)
	if len(unique) == 0 {
		return map[int]ItemDisplay{}, nil
	}
	stringIDs := make([]string, len(unique))
	for i, v := range unique {
		stringIDs[i] = strconv.Itoa(int(v))
	}

	query := `
	SELECT item_id,
       COALESCE(name, '') AS name
	FROM riot_cdn_items
	WHERE item_id = ANY($1)`
	rows, err := db.pool.Query(ctx, query, stringIDs)
	if err != nil {
		return nil, fmt.Errorf("query item display by ids: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ItemDisplay])
	if err != nil {
		return nil, fmt.Errorf("collect item display rows: %w", err)
	}

	out := make(map[int]ItemDisplay, len(collected))
	for _, row := range collected {
		if id, err := strconv.Atoi(row.ItemID); err == nil {
			out[id] = row
		}
	}
	return out, nil
}

func uniquePositiveInt32s(ids []int) []int32 {
	seen := make(map[int]struct{}, len(ids))
	out := make([]int32, 0, len(ids))
	for _, v := range ids {
		if v > 0 {
			if _, ok := seen[v]; !ok {
				seen[v] = struct{}{}
				out = append(out, int32(v))
			}
		}
	}
	return out
}

func collectDisplayByPositiveIDs[T any](
	ctx context.Context, db *Database, ids []int,
	query string, queryErr string, collectErr string, keyFn func(T) int,
) (map[int]T, error) {
	if err := db.ensureReady(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[int]T{}, nil
	}

	intIDs := uniquePositiveInt32s(ids)
	if len(intIDs) == 0 {
		return map[int]T{}, nil
	}

	rows, err := db.pool.Query(ctx, query, intIDs)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", queryErr, err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByPos[T])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", collectErr, err)
	}

	out := make(map[int]T, len(collected))
	for _, row := range collected {
		key := keyFn(row)
		if key > 0 {
			out[key] = row
		}
	}
	return out, nil
}

func queryDisplayByID[T any](ctx context.Context, db *Database, id int, query, op string) (T, bool, error) {
	var zero T
	if err := db.ensureReady(); err != nil {
		return zero, false, err
	}
	if id <= 0 {
		return zero, false, nil
	}

	rows, err := db.pool.Query(ctx, query, id)
	if err != nil {
		return zero, false, fmt.Errorf("%s %d: %w", op, id, err)
	}
	defer rows.Close()

	out, err := pgx.CollectOneRow(rows, pgx.RowToStructByPos[T])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return zero, false, nil
		}
		return zero, false, fmt.Errorf("%s %d: %w", op, id, err)
	}
	return out, true, nil
}
