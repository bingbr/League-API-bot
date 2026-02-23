package tracknotify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

type trackPostState struct {
	attempts    int
	nextRetryAt *time.Time
	abandonedAt *time.Time
	postedAt    *time.Time
	lastError   string
}

type trackNotifyFixture struct {
	ctx    context.Context
	db     *postgres.Database
	pool   *pgxpool.Pool
	prefix string
	svc    *Service
}

func TestTrackNotifyIntegration_PublishPostEmbedsSchedulesRetryOnFetchFailure(t *testing.T) {
	fx := newTrackNotifyFixture(t)
	notification := fx.createPendingNotification(t, "retry", "NA1")

	withFetchMatchStub(t, func(context.Context, string, string, string) (riot.MatchDetail, error) {
		return riot.MatchDetail{}, errors.New("fetch failed")
	})

	before := time.Now().UTC()
	fx.svc.publishPostEmbeds(fx.ctx, map[guildMatchKey]*liveGuildMatch{}, nil)

	state := fx.postState(t, notification.Key())
	if state.attempts != 1 {
		t.Fatalf("post_attempts = %d, want 1", state.attempts)
	}
	if state.nextRetryAt == nil {
		t.Fatalf("next_post_attempt_at = nil, want non-nil")
	}
	if state.nextRetryAt.Before(before.Add(postRetryBaseDelay - 2*time.Second)) {
		t.Fatalf("next_post_attempt_at = %v, expected delayed retry", state.nextRetryAt.UTC())
	}
	if state.abandonedAt != nil || state.postedAt != nil {
		t.Fatalf("unexpected post completion state: abandoned=%v posted=%v", state.abandonedAt, state.postedAt)
	}
	if !strings.Contains(state.lastError, "fetch match failed") {
		t.Fatalf("last_post_error = %q, expected fetch failure reason", state.lastError)
	}

	fx.assertPending(t, notification.Key(), time.Now().UTC(), false)
}

func TestTrackNotifyIntegration_PublishPostEmbedsAbandonsUnknownPlatform(t *testing.T) {
	fx := newTrackNotifyFixture(t)
	notification := fx.createPendingNotification(t, "abandon", "ZZ1")

	fx.svc.publishPostEmbeds(fx.ctx, map[guildMatchKey]*liveGuildMatch{}, nil)

	state := fx.postState(t, notification.Key())
	if state.attempts != 0 {
		t.Fatalf("post_attempts = %d, want 0", state.attempts)
	}
	if state.nextRetryAt != nil {
		t.Fatalf("next_post_attempt_at = %v, want nil", state.nextRetryAt)
	}
	if state.abandonedAt == nil || state.postedAt != nil {
		t.Fatalf("unexpected abandon/post state: abandoned=%v posted=%v", state.abandonedAt, state.postedAt)
	}
	if state.lastError != "platform has no continent mapping" {
		t.Fatalf("last_post_error = %q, want %q", state.lastError, "platform has no continent mapping")
	}

	fx.assertPending(t, notification.Key(), time.Now().UTC().Add(2*time.Hour), false)
}

func TestTrackNotifyIntegration_ResolvePostMatchCachesAndReusesSnapshot(t *testing.T) {
	fx := newTrackNotifyFixture(t)
	matchID := strings.ToUpper(fx.prefix) + "_MATCH_1"
	notification := postgres.TrackMatchNotification{
		GuildID:    fx.prefix + "_guild",
		PlatformID: "NA1",
		GameID:     404,
		MatchID:    matchID,
	}
	expected := riot.MatchDetail{
		Metadata: riot.MatchMetadata{
			MatchID: matchID,
			Players: []string{"puuid-1"},
		},
		Info: riot.MatchInfo{
			QueueID:      420,
			GameDuration: 1800,
		},
	}

	fetchCalls := 0
	withFetchMatchStub(t, func(context.Context, string, string, string) (riot.MatchDetail, error) {
		fetchCalls++
		return expected, nil
	})

	got, err := fx.svc.resolvePostMatch(fx.ctx, notification, "americas")
	if err != nil {
		t.Fatalf("resolvePostMatch(fetch) error = %v", err)
	}
	if got.Metadata.MatchID != matchID || fetchCalls != 1 {
		t.Fatalf("first resolve result mismatch: matchID=%q fetchCalls=%d", got.Metadata.MatchID, fetchCalls)
	}

	snapshot, found, err := fx.db.GetTrackMatchSnapshot(fx.ctx, matchID)
	if err != nil {
		t.Fatalf("GetTrackMatchSnapshot() error = %v", err)
	}
	if !found || snapshot.Info.QueueID != 420 {
		t.Fatalf("snapshot mismatch: found=%v queueID=%d", found, snapshot.Info.QueueID)
	}

	withFetchMatchStub(t, func(context.Context, string, string, string) (riot.MatchDetail, error) {
		t.Fatalf("fetchMatchByID should not be called when snapshot exists")
		return riot.MatchDetail{}, nil
	})

	got, err = fx.svc.resolvePostMatch(fx.ctx, notification, "americas")
	if err != nil {
		t.Fatalf("resolvePostMatch(snapshot) error = %v", err)
	}
	if got.Metadata.MatchID != matchID {
		t.Fatalf("resolvePostMatch(snapshot).Metadata.MatchID = %q, want %q", got.Metadata.MatchID, matchID)
	}
}

func newTrackNotifyFixture(t *testing.T) trackNotifyFixture {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	db, pool, prefix := openTrackNotifyIntegrationDB(t, ctx)
	return trackNotifyFixture{
		ctx:    ctx,
		db:     db,
		pool:   pool,
		prefix: prefix,
		svc: &Service{
			database:   db,
			logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
			riotAPIKey: "test-api-key",
		},
	}
}

func (fx trackNotifyFixture) createPendingNotification(t *testing.T, suffix, platformID string) postgres.TrackMatchNotification {
	t.Helper()

	guildID := fx.prefix + "_" + suffix + "_guild"
	matchID := strings.ToUpper(fx.prefix + "_" + suffix + "_match")
	if err := fx.db.UpsertTrackGuildConfig(fx.ctx, guildID, "channel-1"); err != nil {
		t.Fatalf("UpsertTrackGuildConfig() error = %v", err)
	}

	notification, err := fx.db.UpsertTrackMatchNotificationLive(fx.ctx, postgres.UpsertTrackMatchLiveInput{
		GuildID:        guildID,
		PlatformID:     platformID,
		GameID:         1001,
		MatchID:        matchID,
		QueueID:        420,
		QueueCategory:  queueCategoryPvP,
		PlayerPUUID:    "puuid-1",
		PlayerRiotID:   "Player#NA1",
		TrackedCount:   1,
		LiveChannelID:  "channel-1",
		LastLiveSeenAt: time.Now().UTC().Add(-time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertTrackMatchNotificationLive() error = %v", err)
	}
	if err := fx.db.MarkTrackMatchLivePosted(fx.ctx, notification.Key(), "channel-1", "live-message-1", time.Now().UTC()); err != nil {
		t.Fatalf("MarkTrackMatchLivePosted() error = %v", err)
	}
	return notification
}

func (fx trackNotifyFixture) postState(t *testing.T, key postgres.TrackMatchNotificationKey) trackPostState {
	t.Helper()

	row := fx.pool.QueryRow(fx.ctx, `
		SELECT post_attempts, next_post_attempt_at, post_abandoned_at, post_posted_at, last_post_error
		FROM track_match_notifications
		WHERE guild_id = $1 AND platform_id = $2 AND game_id = $3`,
		key.GuildID, key.PlatformID, key.GameID,
	)
	var state trackPostState
	if err := row.Scan(&state.attempts, &state.nextRetryAt, &state.abandonedAt, &state.postedAt, &state.lastError); err != nil {
		t.Fatalf("scan notification state error = %v", err)
	}
	return state
}

func (fx trackNotifyFixture) assertPending(t *testing.T, key postgres.TrackMatchNotificationKey, at time.Time, want bool) {
	t.Helper()

	list, err := fx.db.ListPendingTrackMatchNotifications(fx.ctx, at, 500)
	if err != nil {
		t.Fatalf("ListPendingTrackMatchNotifications() error = %v", err)
	}
	_, found := findTrackNotifyByKey(list, key)
	if found != want {
		t.Fatalf("pending state mismatch: got=%v want=%v key=%+v", found, want, key)
	}
}

func withFetchMatchStub(t *testing.T, fn func(context.Context, string, string, string) (riot.MatchDetail, error)) {
	t.Helper()
	originalFetch := fetchMatchByID
	fetchMatchByID = fn
	t.Cleanup(func() { fetchMatchByID = originalFetch })
}

func openTrackNotifyIntegrationDB(t *testing.T, ctx context.Context) (*postgres.Database, *pgxpool.Pool, string) {
	t.Helper()

	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run tracknotify postgres integration tests")
	}

	pool, err := postgres.NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("postgres.NewPool() error = %v", err)
	}
	t.Cleanup(pool.Close)

	db := postgres.NewDB(pool)
	if err := db.CreateTrackTable(ctx); err != nil {
		t.Fatalf("CreateTrackTable() error = %v", err)
	}

	prefix := trackNotifyIntegrationPrefix(t.Name())
	cleanupTrackNotifyIntegrationData(t, pool, prefix)
	t.Cleanup(func() { cleanupTrackNotifyIntegrationData(t, pool, prefix) })
	return db, pool, prefix
}

func trackNotifyIntegrationPrefix(testName string) string {
	sanitized := strings.ToLower(testName)
	sanitized = strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(sanitized)
	return "it_tn_" + sanitized + "_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
}

func cleanupTrackNotifyIntegrationData(t *testing.T, pool *pgxpool.Pool, prefix string) {
	t.Helper()
	if pool == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	guildLike := prefix + "%"
	if _, err := pool.Exec(ctx, `DELETE FROM track_match_notifications WHERE guild_id LIKE $1`, guildLike); err != nil {
		t.Fatalf("cleanup track_match_notifications: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM track_accounts WHERE guild_id LIKE $1`, guildLike); err != nil {
		t.Fatalf("cleanup track_accounts: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM track_guild_config WHERE guild_id LIKE $1`, guildLike); err != nil {
		t.Fatalf("cleanup track_guild_config: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM track_match_snapshots WHERE match_id ILIKE $1`, strings.ToUpper(prefix)+"%"); err != nil {
		t.Fatalf("cleanup track_match_snapshots: %v", err)
	}
}

func findTrackNotifyByKey(list []postgres.TrackMatchNotification, key postgres.TrackMatchNotificationKey) (postgres.TrackMatchNotification, bool) {
	for _, item := range list {
		if item.GuildID == key.GuildID && item.PlatformID == key.PlatformID && item.GameID == key.GameID {
			return item, true
		}
	}
	return postgres.TrackMatchNotification{}, false
}
