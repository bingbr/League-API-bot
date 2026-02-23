package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/jackc/pgx/v5"
)

type TrackNotificationTarget struct {
	GuildID        string
	ChannelID      string
	PlatformRegion string
	PUUID          string
	NickName       string
	TagLine        string
}

func (t TrackNotificationTarget) RiotID() string {
	return riot.FormatRiotID(t.NickName, t.TagLine)
}

type TrackMatchNotificationKey struct {
	GuildID    string
	PlatformID string
	GameID     int64
}

type TrackMatchNotification struct {
	GuildID, PlatformID string
	GameID              int64
	MatchID             string
	QueueID             int
	QueueCategory       string
	PlayerPUUID         string
	PlayerRiotID        string
	TrackedCount        int
	LiveChannelID       string
	LiveMessageID       string
	LivePostedAt        *time.Time
	LastLiveSeenAt      time.Time
	PostMessageID       string
	PostPostedAt        *time.Time
	PostAttempts        int
	NextPostAttemptAt   *time.Time
	PostAbandonedAt     *time.Time
	LastPostError       string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (n TrackMatchNotification) Key() TrackMatchNotificationKey {
	return TrackMatchNotificationKey{
		GuildID:    n.GuildID,
		PlatformID: n.PlatformID,
		GameID:     n.GameID,
	}
}

type UpsertTrackMatchLiveInput struct {
	GuildID        string
	PlatformID     string
	GameID         int64
	MatchID        string
	QueueID        int
	QueueCategory  string
	PlayerPUUID    string
	PlayerRiotID   string
	TrackedCount   int
	LiveChannelID  string
	LastLiveSeenAt time.Time
}

func (db *Database) ListTrackNotificationTargets(ctx context.Context) ([]TrackNotificationTarget, error) {
	if err := db.ensureReady(); err != nil {
		return nil, err
	}
	query := `
	SELECT a.guild_id,
		c.channel_id,
		a.platform_region,
		a.puuid,
		a.game_name,
		a.tag_line
	FROM track_accounts a
	JOIN track_guild_config c
	ON c.guild_id = a.guild_id
	ORDER BY a.guild_id, a.platform_region, a.puuid`
	rows, err := db.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list track notification targets: %w", err)
	}
	defer rows.Close()

	targets, err := pgx.CollectRows(rows, pgx.RowToStructByPos[TrackNotificationTarget])
	if err != nil {
		return nil, fmt.Errorf("collect track notification targets: %w", err)
	}

	out := make([]TrackNotificationTarget, 0, len(targets))
	for _, target := range targets {
		target.GuildID = strings.TrimSpace(target.GuildID)
		target.ChannelID = strings.TrimSpace(target.ChannelID)
		target.PlatformRegion = riot.NormalizePlatformRegion(target.PlatformRegion)
		target.PUUID = strings.TrimSpace(target.PUUID)
		target.NickName = strings.TrimSpace(target.NickName)
		target.TagLine = strings.TrimPrefix(strings.TrimSpace(target.TagLine), "#")
		if target.GuildID == "" || target.ChannelID == "" || target.PlatformRegion == "" || target.PUUID == "" {
			continue
		}
		out = append(out, target)
	}
	return out, nil
}

func (db *Database) UpsertTrackMatchNotificationLive(ctx context.Context, input UpsertTrackMatchLiveInput) (TrackMatchNotification, error) {
	if err := db.ensureReady(); err != nil {
		return TrackMatchNotification{}, err
	}

	key := normalizeTrackMatchKey(TrackMatchNotificationKey{
		GuildID:    input.GuildID,
		PlatformID: input.PlatformID,
		GameID:     input.GameID,
	})
	input.GuildID = key.GuildID
	input.PlatformID = key.PlatformID
	input.GameID = key.GameID
	input.MatchID = strings.TrimSpace(input.MatchID)
	input.QueueCategory = strings.TrimSpace(input.QueueCategory)
	input.PlayerPUUID = strings.TrimSpace(input.PlayerPUUID)
	input.PlayerRiotID = strings.TrimSpace(input.PlayerRiotID)
	input.LiveChannelID = strings.TrimSpace(input.LiveChannelID)
	if input.LastLiveSeenAt.IsZero() {
		input.LastLiveSeenAt = time.Now().UTC()
	}
	input.LastLiveSeenAt = input.LastLiveSeenAt.UTC()

	query := `
	INSERT INTO track_match_notifications (
		guild_id,
		platform_id,
		game_id,
		match_id,
		queue_id,
		queue_category,
		player_puuid,
		player_riot_id,
		tracked_count,
		live_channel_id,
		last_live_seen_at
	)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	ON CONFLICT (guild_id, platform_id, game_id) DO UPDATE
	SET match_id = excluded.match_id,
		queue_id = excluded.queue_id,
		queue_category = excluded.queue_category,
		player_puuid = excluded.player_puuid,
		player_riot_id = excluded.player_riot_id,
		tracked_count = excluded.tracked_count,
		live_channel_id = excluded.live_channel_id,
		last_live_seen_at = excluded.last_live_seen_at,
		updated_at = now()
	RETURNING guild_id,
			platform_id,
			game_id,
			match_id,
			queue_id,
			queue_category,
			player_puuid,
			player_riot_id,
			tracked_count,
			live_channel_id,
			live_message_id,
			live_posted_at,
			last_live_seen_at,
			post_message_id,
			post_posted_at,
			post_attempts,
			next_post_attempt_at,
			post_abandoned_at,
			last_post_error,
			created_at,
			updated_at`
	row := db.pool.QueryRow(
		ctx, query,
		input.GuildID, input.PlatformID,
		input.GameID, input.MatchID, input.QueueID, input.QueueCategory,
		input.PlayerPUUID, input.PlayerRiotID, input.TrackedCount, input.LiveChannelID, input.LastLiveSeenAt,
	)
	notification, err := scanTrackMatchNotificationRow(row)
	if err != nil {
		return TrackMatchNotification{}, fmt.Errorf("upsert track match notification live %s/%s/%d: %w", input.GuildID, input.PlatformID, input.GameID, err)
	}
	return notification, nil
}

func (db *Database) MarkTrackMatchLivePosted(ctx context.Context, key TrackMatchNotificationKey, channelID, messageID string, postedAt time.Time) error {
	channelID = strings.TrimSpace(channelID)
	messageID = strings.TrimSpace(messageID)
	postedAt = utcNowIfZero(postedAt)

	query := `
	UPDATE track_match_notifications
	SET live_channel_id = $4,
		live_message_id = $5,
		live_posted_at = $6,
		updated_at = now()
	WHERE guild_id = $1
	AND platform_id = $2
	AND game_id = $3
	AND (live_message_id IS NULL OR live_message_id = '')`
	return db.execTrackMatchNotificationUpdate(ctx, "mark track match live posted", key, query, channelID, messageID, postedAt)
}

func (db *Database) ListPendingTrackMatchNotifications(ctx context.Context, now time.Time, limit int) ([]TrackMatchNotification, error) {
	if err := db.ensureReady(); err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()
	if limit <= 0 || limit > 500 {
		limit = 200
	}

	query := `
	SELECT guild_id,
		platform_id,
		game_id,
		match_id,
		queue_id,
		queue_category,
		player_puuid,
		player_riot_id,
		tracked_count,
		live_channel_id,
		live_message_id,
		live_posted_at,
		last_live_seen_at,
		post_message_id,
		post_posted_at,
		post_attempts,
		next_post_attempt_at,
		post_abandoned_at,
		last_post_error,
		created_at,
		updated_at
	FROM track_match_notifications
	WHERE live_posted_at IS NOT NULL
	AND post_posted_at IS NULL
	AND post_abandoned_at IS NULL
	AND (next_post_attempt_at IS NULL OR next_post_attempt_at <= $1)
	ORDER BY last_live_seen_at ASC
	LIMIT $2`
	rows, err := db.pool.Query(ctx, query, now, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending track match notifications: %w", err)
	}
	defer rows.Close()

	out := make([]TrackMatchNotification, 0, limit)
	for rows.Next() {
		notification, err := scanTrackMatchNotificationRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pending track match notification: %w", err)
		}
		out = append(out, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending track match notifications: %w", err)
	}
	return out, nil
}

func (db *Database) MarkTrackMatchPostRetry(ctx context.Context, key TrackMatchNotificationKey, attempts int, nextAttemptAt time.Time, lastError string) error {
	if attempts < 0 {
		attempts = 0
	}
	nextAttemptAt = utcNowIfZero(nextAttemptAt)
	lastError = strings.TrimSpace(lastError)

	query := `
	UPDATE track_match_notifications
	SET post_attempts = $4,
		next_post_attempt_at = $5,
		last_post_error = $6,
		updated_at = now()
	WHERE guild_id = $1
	AND platform_id = $2
	AND game_id = $3
	AND post_posted_at IS NULL
	AND post_abandoned_at IS NULL`
	return db.execTrackMatchNotificationUpdate(ctx, "mark track match post retry", key, query, attempts, nextAttemptAt, lastError)
}

func (db *Database) MarkTrackMatchPostPosted(ctx context.Context, key TrackMatchNotificationKey, messageID string, postedAt time.Time) error {
	messageID = strings.TrimSpace(messageID)
	postedAt = utcNowIfZero(postedAt)

	query := `
	UPDATE track_match_notifications
	SET post_message_id = $4,
		post_posted_at = $5,
		next_post_attempt_at = NULL,
		last_post_error = '',
		updated_at = now()
	WHERE guild_id = $1
	AND platform_id = $2
	AND game_id = $3`
	return db.execTrackMatchNotificationUpdate(ctx, "mark track match post posted", key, query, messageID, postedAt)
}

func (db *Database) AbandonTrackMatchNotification(ctx context.Context, key TrackMatchNotificationKey, abandonedAt time.Time, lastError string) error {
	abandonedAt = utcNowIfZero(abandonedAt)
	lastError = strings.TrimSpace(lastError)

	query := `
	UPDATE track_match_notifications
	SET post_abandoned_at = $4,
		next_post_attempt_at = NULL,
		last_post_error = $5,
		updated_at = now()
	WHERE guild_id = $1
	AND platform_id = $2
	AND game_id = $3
	AND post_posted_at IS NULL`
	return db.execTrackMatchNotificationUpdate(ctx, "abandon track match notification", key, query, abandonedAt, lastError)
}

func (db *Database) GetTrackMatchSnapshot(ctx context.Context, matchID string) (riot.MatchDetail, bool, error) {
	if err := db.ensureReady(); err != nil {
		return riot.MatchDetail{}, false, err
	}

	matchID = strings.TrimSpace(matchID)
	if matchID == "" {
		return riot.MatchDetail{}, false, fmt.Errorf("match id is required")
	}

	query := `
	SELECT payload
	FROM track_match_snapshots
	WHERE match_id = $1`
	var payload []byte
	err := db.pool.QueryRow(ctx, query, matchID).Scan(&payload)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return riot.MatchDetail{}, false, nil
		}
		return riot.MatchDetail{}, false, fmt.Errorf("get track match snapshot %q: %w", matchID, err)
	}

	var match riot.MatchDetail
	if err := json.Unmarshal(payload, &match); err != nil {
		return riot.MatchDetail{}, false, fmt.Errorf("decode track match snapshot %q: %w", matchID, err)
	}
	if strings.TrimSpace(match.Metadata.MatchID) == "" {
		match.Metadata.MatchID = matchID
	}
	return match, true, nil
}

func (db *Database) UpsertTrackMatchSnapshot(ctx context.Context, match riot.MatchDetail) error {
	if err := db.ensureReady(); err != nil {
		return err
	}

	matchID := strings.TrimSpace(match.Metadata.MatchID)
	if matchID == "" {
		return fmt.Errorf("match id is required")
	}
	payload, err := json.Marshal(match)
	if err != nil {
		return fmt.Errorf("marshal track match snapshot %q: %w", matchID, err)
	}

	query := `
	INSERT INTO track_match_snapshots (match_id, payload, updated_at)
	VALUES ($1, $2, $3)
	ON CONFLICT (match_id) DO UPDATE
	SET payload = excluded.payload,
		updated_at = excluded.updated_at`
	if _, err := db.pool.Exec(ctx, query, matchID, payload, time.Now().UTC()); err != nil {
		return fmt.Errorf("upsert track match snapshot %q: %w", matchID, err)
	}
	return nil
}

func (db *Database) CleanupTrackMatchNotifications(ctx context.Context, olderThan time.Time) (int64, error) {
	if err := db.ensureReady(); err != nil {
		return 0, err
	}
	if olderThan.IsZero() {
		olderThan = time.Now().UTC().AddDate(0, 0, -7)
	}
	olderThan = olderThan.UTC()

	query := `
	DELETE FROM track_match_notifications
	WHERE updated_at < $1`
	result, err := db.pool.Exec(ctx, query, olderThan)
	if err != nil {
		return 0, fmt.Errorf("cleanup track match notifications: %w", err)
	}

	cleanupSnapshotsQuery := `
	DELETE FROM track_match_snapshots s
	WHERE s.updated_at < $1
	AND NOT EXISTS (
		SELECT 1
		FROM track_match_notifications n
		WHERE n.match_id = s.match_id
	)`
	if _, err := db.pool.Exec(ctx, cleanupSnapshotsQuery, olderThan); err != nil {
		return 0, fmt.Errorf("cleanup track match snapshots: %w", err)
	}
	return result.RowsAffected(), nil
}

func normalizeTrackMatchKey(key TrackMatchNotificationKey) TrackMatchNotificationKey {
	key.GuildID = strings.TrimSpace(key.GuildID)
	key.PlatformID = strings.ToUpper(strings.TrimSpace(key.PlatformID))
	if key.GameID < 0 {
		key.GameID = 0
	}
	return key
}

func utcNowIfZero(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

func (db *Database) execTrackMatchNotificationUpdate(ctx context.Context, op string, key TrackMatchNotificationKey, query string, args ...any) error {
	if err := db.ensureReady(); err != nil {
		return err
	}
	key = normalizeTrackMatchKey(key)

	params := make([]any, 0, 3+len(args))
	params = append(params, key.GuildID, key.PlatformID, key.GameID)
	params = append(params, args...)

	if _, err := db.pool.Exec(ctx, query, params...); err != nil {
		return fmt.Errorf("%s %s/%s/%d: %w", op, key.GuildID, key.PlatformID, key.GameID, err)
	}
	return nil
}

func scanTrackMatchNotificationRow(row pgx.Row) (TrackMatchNotification, error) {
	return scanTrackMatchNotification(row.Scan)
}

func scanTrackMatchNotificationRows(rows pgx.Rows) (TrackMatchNotification, error) {
	return scanTrackMatchNotification(rows.Scan)
}

func scanTrackMatchNotification(scan func(dest ...any) error) (TrackMatchNotification, error) {
	var notification TrackMatchNotification
	var (
		liveChannelID *string
		liveMessageID *string
		postMessageID *string
	)
	err := scan(
		&notification.GuildID, &notification.PlatformID,
		&notification.GameID, &notification.MatchID, &notification.QueueID, &notification.QueueCategory,
		&notification.PlayerPUUID, &notification.PlayerRiotID, &notification.TrackedCount, &liveChannelID,
		&liveMessageID, &notification.LivePostedAt, &notification.LastLiveSeenAt, &postMessageID,
		&notification.PostPostedAt, &notification.PostAttempts, &notification.NextPostAttemptAt,
		&notification.PostAbandonedAt, &notification.LastPostError,
		&notification.CreatedAt, &notification.UpdatedAt,
	)
	if err != nil {
		return TrackMatchNotification{}, err
	}
	notification.LiveChannelID = nullableText(liveChannelID)
	notification.LiveMessageID = nullableText(liveMessageID)
	notification.PostMessageID = nullableText(postMessageID)
	normalizeTrackMatchNotificationTimes(&notification)
	return notification, nil
}

func nullableText(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func normalizeTrackMatchNotificationTimes(n *TrackMatchNotification) {
	if n == nil {
		return
	}
	n.QueueCategory = strings.TrimSpace(n.QueueCategory)
	n.LastLiveSeenAt = n.LastLiveSeenAt.UTC()
	n.CreatedAt = n.CreatedAt.UTC()
	n.UpdatedAt = n.UpdatedAt.UTC()
	n.LivePostedAt = utcPtr(n.LivePostedAt)
	n.PostPostedAt = utcPtr(n.PostPostedAt)
	n.NextPostAttemptAt = utcPtr(n.NextPostAttemptAt)
	n.PostAbandonedAt = utcPtr(n.PostAbandonedAt)
}

const createTrackMatchNotificationsSQL = `
CREATE TABLE IF NOT EXISTS track_match_notifications (
    guild_id text NOT NULL REFERENCES track_guild_config (guild_id) ON DELETE CASCADE,
    platform_id text NOT NULL,
    game_id bigint NOT NULL,
    match_id text NOT NULL,
    queue_id int NOT NULL DEFAULT 0,
    queue_category text NOT NULL DEFAULT '',
    player_puuid text NOT NULL,
    player_riot_id text NOT NULL,
    tracked_count int NOT NULL,
    live_channel_id text,
    live_message_id text,
    live_posted_at timestamptz,
    last_live_seen_at timestamptz NOT NULL,
    post_message_id text,
    post_posted_at timestamptz,
    post_attempts int NOT NULL DEFAULT 0,
    next_post_attempt_at timestamptz,
    post_abandoned_at timestamptz,
    last_post_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (guild_id, platform_id, game_id)
)`

const createTrackMatchNotificationsPostIdxSQL = `
CREATE INDEX IF NOT EXISTS track_match_notifications_post_idx
ON track_match_notifications (post_posted_at, post_abandoned_at, next_post_attempt_at)`

const createTrackMatchNotificationsMatchIDIdxSQL = `
CREATE INDEX IF NOT EXISTS track_match_notifications_match_id_idx
ON track_match_notifications (match_id)`

const createTrackMatchNotificationsUpdatedAtIdxSQL = `
CREATE INDEX IF NOT EXISTS track_match_notifications_updated_at_idx
ON track_match_notifications (updated_at)`

const createTrackMatchSnapshotsSQL = `
CREATE TABLE IF NOT EXISTS track_match_snapshots (
    match_id text PRIMARY KEY,
    payload jsonb NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
)`

const createTrackMatchSnapshotsUpdatedAtIdxSQL = `
CREATE INDEX IF NOT EXISTS track_match_snapshots_updated_at_idx
ON track_match_snapshots (updated_at)`
