package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/jackc/pgx/v5"
)

type ChampionDisplay struct {
	ChampionID  int
	Name        string
	DiscordIcon string
}

var _ cdn.Database = (*Database)(nil)

func executeBatch(ctx context.Context, tx pgx.Tx, b *pgx.Batch) error {
	br := tx.SendBatch(ctx, b)
	for range b.Len() {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return err
		}
	}
	return br.Close()
}

func runQueuedBatch(ctx context.Context, tx pgx.Tx, op string, queue func(*pgx.Batch) error) error {
	b := &pgx.Batch{}
	if err := queue(b); err != nil {
		return err
	}
	if err := executeBatch(ctx, tx, b); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func queueUpsertQueues(b *pgx.Batch, query string, queues []cdn.Queue, fetchedAt time.Time) error {
	for _, queue := range queues {
		if queue.QueueID < 0 || strings.TrimSpace(queue.Name) == "" {
			continue
		}
		payload, err := json.Marshal(queue)
		if err != nil {
			return fmt.Errorf("marshal queue: %w", err)
		}
		b.Queue(query, queue.QueueID, strings.TrimSpace(queue.Name), strings.TrimSpace(queue.GameSelectCategory), payload, fetchedAt)
	}
	return nil
}

func queueUpsertMaps(b *pgx.Batch, query string, maps []cdn.GameMap, fetchedAt time.Time) error {
	for _, gameMap := range maps {
		if gameMap.MapID <= 0 {
			continue
		}
		payload, err := json.Marshal(gameMap)
		if err != nil {
			return fmt.Errorf("marshal map: %w", err)
		}
		b.Queue(query, gameMap.MapID, strings.TrimSpace(gameMap.Name), payload, fetchedAt)
	}
	return nil
}

func queueUpsertChampions(b *pgx.Batch, query, version string, champs map[string]cdn.Champion, fetchedAt time.Time) error {
	for id, champ := range champs {
		championID, err := strconv.Atoi(champ.Key)
		if err != nil {
			return fmt.Errorf("parse champion key %q for %s: %w", champ.Key, id, err)
		}
		b.Queue(query, championID, version, champ.Name, fetchedAt)
	}
	return nil
}

func queueUpsertItems(b *pgx.Batch, query, version string, items map[string]cdn.Item, fetchedAt time.Time) error {
	for id, item := range items {
		tags := item.Tags
		if tags == nil {
			tags = []string{}
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return fmt.Errorf("marshal item %s: %w", id, err)
		}
		b.Queue(query, id, version, item.Name, item.Plaintext, tags, payload, fetchedAt)
	}
	return nil
}

func queueUpsertSummonerSpells(b *pgx.Batch, query, version string, spells map[string]cdn.SummonerSpell, fetchedAt time.Time) error {
	for id, spell := range spells {
		spellID, err := strconv.Atoi(spell.Key)
		if err != nil {
			return fmt.Errorf("parse summoner spell key %q for %s: %w", spell.Key, id, err)
		}
		b.Queue(query, spellID, version, spell.Name, fetchedAt)
	}
	return nil
}

func queueUpsertRunes(b *pgx.Batch, queryRune string, queryRuneTree string, trees []cdn.RuneTree, fetchedAt time.Time) error {
	for _, tree := range trees {
		treePayload, err := json.Marshal(tree)
		if err != nil {
			return fmt.Errorf("marshal rune tree %d: %w", tree.ID, err)
		}
		b.Queue(queryRuneTree, tree.ID, tree.Key, tree.Name, tree.Icon, treePayload, fetchedAt)

		for _, slot := range tree.Slots {
			for _, rune := range slot.Runes {
				runePayload, err := json.Marshal(rune)
				if err != nil {
					return fmt.Errorf("marshal rune %d: %w", rune.ID, err)
				}
				b.Queue(queryRune, rune.ID, tree.ID, rune.Key, rune.Name, rune.Icon, rune.ShortDesc, rune.LongDesc, runePayload, fetchedAt)
			}
		}
	}
	return nil
}

func queueDiscordIconUpdates(b *pgx.Batch, icons cdn.DiscordIcons, fetchedAt time.Time) {
	for id, icon := range icons.Champions {
		b.Queue(updateRiotCDNChampionDiscordIconSQL, id, icon, fetchedAt)
	}
	for id, icon := range icons.SummonerSpells {
		b.Queue(updateRiotCDNSummonerSpellDiscordIconSQL, id, icon, fetchedAt)
	}
	for id, icon := range icons.RuneTrees {
		b.Queue(updateRiotCDNRuneTreeDiscordIconSQL, id, icon, fetchedAt)
	}
	for id, icon := range icons.Runes {
		b.Queue(updateRiotCDNRuneDiscordIconSQL, id, icon, fetchedAt)
	}
	for tier, icon := range icons.Ranks {
		if t := riot.NormalizeRankTier(tier); t != "" {
			b.Queue(upsertRiotCDNRankedTierDiscordIconSQL, t, icon, fetchedAt)
		}
	}
}

func queueUpsertEmojiEntries(b *pgx.Batch, query string, entries []cdn.EmojiEntry, now time.Time) {
	for _, entry := range entries {
		if strings.TrimSpace(entry.AssetKey) == "" || strings.TrimSpace(entry.EmojiID) == "" || strings.TrimSpace(entry.EmojiName) == "" ||
			strings.TrimSpace(entry.DiscordIcon) == "" || strings.TrimSpace(entry.ContentHash) == "" {
			continue
		}
		var assetID any
		if entry.AssetID != nil {
			assetID = *entry.AssetID
		}
		var sourceContentLength any
		if entry.SourceContentLength > 0 {
			sourceContentLength = entry.SourceContentLength
		}
		b.Queue(query,
			entry.AssetKey, entry.Kind, assetID, strings.TrimSpace(entry.Tier), entry.EmojiName, entry.EmojiID,
			entry.DiscordIcon, entry.SourceURL, entry.ContentHash, strings.TrimSpace(entry.SourceETag),
			strings.TrimSpace(entry.SourceLastModified), sourceContentLength, now,
		)
	}
}

func (s *Database) CreateRiotCDNTable(ctx context.Context) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "create riot cdn schema", func(b *pgx.Batch) error {
			b.Queue(createRiotCDNSyncStateSQL)
			b.Queue(createRiotCDNQueuesSQL)
			b.Queue(createRiotCDNMapsSQL)
			b.Queue(createRiotCDNChampionsSQL)
			b.Queue(createRiotCDNItemsSQL)
			b.Queue(createRiotCDNSummonerSpellsSQL)
			b.Queue(createRiotCDNRuneTreesSQL)
			b.Queue(createRiotCDNRunesSQL)
			b.Queue(createRiotCDNRankedTiersSQL)
			b.Queue(createRiotCDNEmojiSQL)
			return nil
		})
	})
}

func (s *Database) UpsertQueues(ctx context.Context, queues []cdn.Queue, fetchedAt time.Time) error {
	query := `
	INSERT INTO riot_cdn_queues (queue_id, name, game_select_category, data, fetched_at)
	VALUES ($1, $2, $3, $4, $5)
	ON CONFLICT (queue_id) DO UPDATE
	SET name = excluded.name,
		game_select_category = excluded.game_select_category,
		data = excluded.data,
		fetched_at = excluded.fetched_at`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert queues batch", func(b *pgx.Batch) error {
			return queueUpsertQueues(b, query, queues, fetchedAt)
		})
	})
}

func (s *Database) UpsertMaps(ctx context.Context, maps []cdn.GameMap, fetchedAt time.Time) error {
	query := `
	INSERT INTO riot_cdn_maps (map_id, name, data, fetched_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (map_id) DO UPDATE
	SET name = excluded.name,
		data = excluded.data,
		fetched_at = excluded.fetched_at`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert maps batch", func(b *pgx.Batch) error {
			return queueUpsertMaps(b, query, maps, fetchedAt)
		})
	})
}

func (s *Database) UpsertChampions(ctx context.Context, version string, champs map[string]cdn.Champion, fetchedAt time.Time) error {
	query := `
	INSERT INTO riot_cdn_champions (champion_id, version, name, fetched_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (champion_id) DO UPDATE
	SET version = excluded.version,
		name = excluded.name,
		fetched_at = GREATEST(riot_cdn_champions.fetched_at, excluded.fetched_at)`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert champions batch", func(b *pgx.Batch) error {
			return queueUpsertChampions(b, query, version, champs, fetchedAt)
		})
	})
}

func (s *Database) UpsertItems(ctx context.Context, version string, items map[string]cdn.Item, fetchedAt time.Time) error {
	query := `
	INSERT INTO riot_cdn_items (item_id, version, name, plaintext, tags, data, fetched_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (item_id) DO UPDATE
	SET version = excluded.version,
		name = excluded.name,
		plaintext = excluded.plaintext,
		tags = excluded.tags,
		data = excluded.data,
		fetched_at = GREATEST(riot_cdn_items.fetched_at, excluded.fetched_at)`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert items batch", func(b *pgx.Batch) error {
			return queueUpsertItems(b, query, version, items, fetchedAt)
		})
	})
}

func (s *Database) UpsertSummonerSpells(ctx context.Context, version string, spells map[string]cdn.SummonerSpell, fetchedAt time.Time) error {
	query := `
	INSERT INTO riot_cdn_summoner_spells (spell_id, version, name, fetched_at)
	VALUES ($1, $2, $3, $4)
	ON CONFLICT (spell_id) DO UPDATE
	SET version = excluded.version,
		name = excluded.name,
		fetched_at = GREATEST(riot_cdn_summoner_spells.fetched_at, excluded.fetched_at)`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert summoner spells batch", func(b *pgx.Batch) error {
			return queueUpsertSummonerSpells(b, query, version, spells, fetchedAt)
		})
	})
}

func (s *Database) UpsertRunes(ctx context.Context, trees []cdn.RuneTree, fetchedAt time.Time) error {
	queryRune := `
	INSERT INTO riot_cdn_runes (
		rune_id, tree_id, key, name, icon, short_desc, long_desc, data, fetched_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	ON CONFLICT (rune_id) DO UPDATE
	SET tree_id = excluded.tree_id,
		key = excluded.key,
		name = excluded.name,
		icon = excluded.icon,
		short_desc = excluded.short_desc,
		long_desc = excluded.long_desc,
		data = excluded.data,
		fetched_at = GREATEST(riot_cdn_runes.fetched_at, excluded.fetched_at)`
	queryRuneTree := `
	INSERT INTO riot_cdn_rune_trees (
		tree_id, key, name, icon, data, fetched_at
	)
	VALUES ($1, $2, $3, $4, $5, $6)
	ON CONFLICT (tree_id) DO UPDATE
	SET key = excluded.key,
		name = excluded.name,
		icon = excluded.icon,
		data = excluded.data,
		fetched_at = GREATEST(riot_cdn_rune_trees.fetched_at, excluded.fetched_at)`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert runes batch", func(b *pgx.Batch) error {
			return queueUpsertRunes(b, queryRune, queryRuneTree, trees, fetchedAt)
		})
	})
}

func (s *Database) UpsertDiscordIcons(ctx context.Context, icons cdn.DiscordIcons, fetchedAt time.Time) error {
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert discord icons batch", func(b *pgx.Batch) error {
			queueDiscordIconUpdates(b, icons, fetchedAt)
			return nil
		})
	})
}

func (s *Database) EmojiEntries(ctx context.Context) (map[string]cdn.EmojiEntry, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	query := `
	SELECT asset_key,
		kind,
		asset_id,
		tier,
		emoji_name,
		emoji_id,
		discord_icon,
		source_url,
		content_hash,
		source_etag,
		source_last_modified,
		source_content_length
	FROM riot_cdn_emoji_manifest`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query emoji manifest: %w", err)
	}
	defer rows.Close()

	entries, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (cdn.EmojiEntry, error) {
		var (
			entry                                cdn.EmojiEntry
			assetID                              *int32
			tier, sourceETag, sourceLastModified *string
			sourceContentLength                  *int64
		)
		if err := row.Scan(&entry.AssetKey, &entry.Kind, &assetID, &tier, &entry.EmojiName, &entry.EmojiID, &entry.DiscordIcon,
			&entry.SourceURL, &entry.ContentHash, &sourceETag, &sourceLastModified, &sourceContentLength,
		); err != nil {
			return cdn.EmojiEntry{}, err
		}
		if assetID != nil {
			entry.AssetID = new(int(*assetID))
		}
		if tier != nil {
			entry.Tier = *tier
		}
		if sourceETag != nil {
			entry.SourceETag = *sourceETag
		}
		if sourceLastModified != nil {
			entry.SourceLastModified = *sourceLastModified
		}
		if sourceContentLength != nil {
			entry.SourceContentLength = *sourceContentLength
		}
		return entry, nil
	})
	if err != nil {
		return nil, fmt.Errorf("collect emoji manifest rows: %w", err)
	}
	result := make(map[string]cdn.EmojiEntry, len(entries))
	for _, entry := range entries {
		result[entry.AssetKey] = entry
	}
	return result, nil
}

func (s *Database) UpsertEmojiEntries(ctx context.Context, entries []cdn.EmojiEntry) error {
	if len(entries) == 0 {
		return nil
	}
	query := `
	INSERT INTO riot_cdn_emoji_manifest (
		asset_key, kind, asset_id, tier, emoji_name, emoji_id, discord_icon,
		source_url, content_hash, source_etag, source_last_modified, source_content_length, updated_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	ON CONFLICT (asset_key) DO UPDATE
	SET kind = excluded.kind,
		asset_id = excluded.asset_id,
		tier = excluded.tier,
		emoji_name = excluded.emoji_name,
		emoji_id = excluded.emoji_id,
		discord_icon = excluded.discord_icon,
		source_url = excluded.source_url,
		content_hash = excluded.content_hash,
		source_etag = excluded.source_etag,
		source_last_modified = excluded.source_last_modified,
		source_content_length = excluded.source_content_length,
		updated_at = excluded.updated_at`
	return s.withTx(ctx, func(tx pgx.Tx) error {
		return runQueuedBatch(ctx, tx, "upsert emoji manifest batch", func(b *pgx.Batch) error {
			queueUpsertEmojiEntries(b, query, entries, time.Now().UTC())
			return nil
		})
	})
}

func (s *Database) DeleteEmojiEntries(ctx context.Context, assetKeys []string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if len(assetKeys) == 0 {
		return nil
	}
	var normalized []string
	for _, key := range assetKeys {
		if k := strings.TrimSpace(key); k != "" {
			normalized = append(normalized, k)
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	query := `
	DELETE FROM riot_cdn_emoji_manifest
	WHERE asset_key = ANY($1)`
	if _, err := s.pool.Exec(ctx, query, normalized); err != nil {
		return fmt.Errorf("delete emoji manifest entries: %w", err)
	}
	return nil
}

func (s *Database) RankIconsByTiers(ctx context.Context, tiers []string) (map[string]string, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if len(tiers) == 0 {
		return map[string]string{}, nil
	}

	seen := make(map[string]struct{}, len(tiers))
	var normalized []string
	for _, tier := range tiers {
		if t := riot.NormalizeRankTier(tier); t != "" {
			if _, ok := seen[t]; !ok {
				seen[t] = struct{}{}
				normalized = append(normalized, t)
			}
		}
	}
	if len(normalized) == 0 {
		return map[string]string{}, nil
	}
	query := `
	SELECT tier,
		discord_icon
	FROM riot_cdn_ranked_tiers
	WHERE tier = ANY($1)`
	rows, err := s.pool.Query(ctx, query, normalized)
	if err != nil {
		return nil, fmt.Errorf("query ranked tier icons: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string, len(normalized))
	for rows.Next() {
		var tier, icon string
		if err := rows.Scan(&tier, &icon); err != nil {
			return nil, fmt.Errorf("scan ranked tier icon: %w", err)
		}
		result[tier] = icon
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ranked tier icons rows: %w", err)
	}
	return result, nil
}

func (s *Database) ChampionDisplayByIDs(ctx context.Context, ids []int) (map[int]ChampionDisplay, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[int]ChampionDisplay{}, nil
	}

	idList := uniquePositiveInt32s(ids)
	if len(idList) == 0 {
		return map[int]ChampionDisplay{}, nil
	}

	query := `
	SELECT champion_id,
		COALESCE(name, '') AS name,
		COALESCE(discord_icon, '')
	FROM riot_cdn_champions
	WHERE champion_id = ANY($1)`
	rows, err := s.pool.Query(ctx, query, idList)
	if err != nil {
		return nil, fmt.Errorf("query champion display by ids: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ChampionDisplay])
	if err != nil {
		return nil, fmt.Errorf("collect champion display rows: %w", err)
	}

	result := make(map[int]ChampionDisplay, len(collected))
	for _, champion := range collected {
		result[champion.ChampionID] = champion
	}
	return result, nil
}

func (s *Database) GetRiotCDNSyncVersion(ctx context.Context, syncKey string) (string, bool, error) {
	if err := s.ensureReady(); err != nil {
		return "", false, err
	}
	if strings.TrimSpace(syncKey) == "" {
		return "", false, ErrSyncKeyRequired
	}
	query := `
	SELECT version
	FROM riot_cdn_sync_state
	WHERE sync_key = $1`
	var version string
	err := s.pool.QueryRow(ctx, query, syncKey).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query riot cdn sync version %q: %w", syncKey, err)
	}
	return version, true, nil
}

func (s *Database) UpsertRiotCDNSyncVersion(ctx context.Context, syncKey, version string) error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if strings.TrimSpace(syncKey) == "" {
		return ErrSyncKeyRequired
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("version is required")
	}

	query := `
	INSERT INTO riot_cdn_sync_state (sync_key, version, updated_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (sync_key) DO UPDATE
	SET version = excluded.version,
		updated_at = excluded.updated_at`
	if _, err := s.pool.Exec(ctx, query, syncKey, version, time.Now().UTC()); err != nil {
		return fmt.Errorf("upsert riot cdn sync version %q: %w", syncKey, err)
	}
	return nil
}
