package postgres

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
)

type trackFixture struct {
	ctx    context.Context
	db     *Database
	prefix string
}

func TestTrackIntegration_TrackedAccountLifecycle(t *testing.T) {
	fx := newTrackFixture(t)
	guildID := fx.prefix + "_acct_guild"

	if err := fx.db.UpsertTrackGuildConfig(fx.ctx, " "+guildID+" ", " channel-1 "); err != nil {
		t.Fatalf("UpsertTrackGuildConfig() error = %v", err)
	}

	created, err := fx.db.AddTrackedAccount(fx.ctx, TrackedAccount{
		GuildID:        " " + guildID + " ",
		PlatformRegion: " NA1 ",
		PUUID:          " puuid-1 ",
		NickName:       " Ahri ",
		TagLine:        " #NA1 ",
		AddedBy:        " admin ",
	})
	if err != nil || !created {
		t.Fatalf("AddTrackedAccount(create) = %v, %v; want true, nil", created, err)
	}

	created, err = fx.db.AddTrackedAccount(fx.ctx, TrackedAccount{
		GuildID:        guildID,
		PlatformRegion: "na1",
		PUUID:          "puuid-1",
		NickName:       " Zed ",
		TagLine:        " #NA1",
		AddedBy:        "ignored-on-update",
	})
	if err != nil || created {
		t.Fatalf("AddTrackedAccount(update) = %v, %v; want false, nil", created, err)
	}

	tracked := fx.listTracked(t, guildID)
	if len(tracked) != 1 {
		t.Fatalf("len(ListTrackedAccounts()) = %d, want 1", len(tracked))
	}
	if got := tracked[0]; got.PlatformRegion != "na1" || got.RiotID() != "Zed#NA1" {
		t.Fatalf("tracked account mismatch: platform=%q riotID=%q", got.PlatformRegion, got.RiotID())
	}

	removed, err := fx.db.RemoveTrackedAccount(fx.ctx, guildID, " zed#na1 ")
	if err != nil || !removed {
		t.Fatalf("RemoveTrackedAccount() = %v, %v; want true, nil", removed, err)
	}
	if got := fx.listTracked(t, guildID); len(got) != 0 {
		t.Fatalf("len(ListTrackedAccounts(after remove)) = %d, want 0", len(got))
	}
}

func TestTrackIntegration_MatchNotificationLifecycle(t *testing.T) {
	fx := newTrackFixture(t)
	notification := fx.createLiveNotification(t, "notify", " na1 ")
	key := notification.Key()

	if notification.PlatformID != "NA1" || notification.GuildID == "" || notification.GameID <= 0 {
		t.Fatalf("notification key normalization failed: %#v", notification.Key())
	}

	entry := fx.pendingByKey(t, key, time.Now().UTC(), true)
	if entry.LiveMessageID != "live-msg-1" {
		t.Fatalf("LiveMessageID = %q, want %q", entry.LiveMessageID, "live-msg-1")
	}

	nextRetry := time.Now().UTC().Add(30 * time.Minute)
	if err := fx.db.MarkTrackMatchPostRetry(fx.ctx, key, 1, nextRetry, "temporary post failure"); err != nil {
		t.Fatalf("MarkTrackMatchPostRetry() error = %v", err)
	}
	fx.pendingByKey(t, key, time.Now().UTC(), false)
	fx.pendingByKey(t, key, nextRetry.Add(time.Second), true)

	if err := fx.db.MarkTrackMatchPostPosted(fx.ctx, key, "post-msg-1", time.Now().UTC()); err != nil {
		t.Fatalf("MarkTrackMatchPostPosted() error = %v", err)
	}
	fx.pendingByKey(t, key, time.Now().UTC().Add(time.Hour), false)
}

func TestTrackIntegration_SnapshotRoundTripAndCleanup(t *testing.T) {
	fx := newTrackFixture(t)
	matchID := strings.ToUpper(fx.prefix) + "_SNAP_1"
	match := riot.MatchDetail{
		Metadata: riot.MatchMetadata{
			MatchID: matchID,
			Players: []string{"puuid-1"},
		},
		Info: riot.MatchInfo{
			QueueID:      440,
			GameDuration: 1800,
		},
	}

	if err := fx.db.UpsertTrackMatchSnapshot(fx.ctx, match); err != nil {
		t.Fatalf("UpsertTrackMatchSnapshot() error = %v", err)
	}
	snapshot, found, err := fx.db.GetTrackMatchSnapshot(fx.ctx, matchID)
	if err != nil || !found {
		t.Fatalf("GetTrackMatchSnapshot() = found:%v err:%v; want true, nil", found, err)
	}
	if snapshot.Metadata.MatchID != matchID || snapshot.Info.QueueID != 440 {
		t.Fatalf("snapshot mismatch: matchID=%q queueID=%d", snapshot.Metadata.MatchID, snapshot.Info.QueueID)
	}

	if _, err := fx.db.pool.Exec(fx.ctx, `UPDATE track_match_snapshots SET updated_at = $1 WHERE match_id = $2`, time.Now().UTC().Add(-48*time.Hour), matchID); err != nil {
		t.Fatalf("force snapshot age update error = %v", err)
	}
	deleted, err := fx.db.CleanupTrackMatchNotifications(fx.ctx, time.Now().UTC().Add(-24*time.Hour))
	if err != nil || deleted != 0 {
		t.Fatalf("CleanupTrackMatchNotifications() = %d, %v; want 0, nil", deleted, err)
	}

	_, found, err = fx.db.GetTrackMatchSnapshot(fx.ctx, matchID)
	if err != nil {
		t.Fatalf("GetTrackMatchSnapshot(after cleanup) error = %v", err)
	}
	if found {
		t.Fatalf("snapshot should be removed by cleanup when stale and unreferenced")
	}
}

func newTrackFixture(t *testing.T) trackFixture {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	db, prefix := openTrackIntegrationDB(t, ctx)
	return trackFixture{ctx: ctx, db: db, prefix: prefix}
}

func (fx trackFixture) createLiveNotification(t *testing.T, suffix, platformID string) TrackMatchNotification {
	t.Helper()

	guildID := fx.prefix + "_" + suffix + "_guild"
	matchID := strings.ToUpper(fx.prefix + "_" + suffix + "_1001")
	if err := fx.db.UpsertTrackGuildConfig(fx.ctx, guildID, "live-channel"); err != nil {
		t.Fatalf("UpsertTrackGuildConfig() error = %v", err)
	}
	notification, err := fx.db.UpsertTrackMatchNotificationLive(fx.ctx, UpsertTrackMatchLiveInput{
		GuildID:        " " + guildID + " ",
		PlatformID:     platformID,
		GameID:         1001,
		MatchID:        matchID,
		QueueID:        420,
		QueueCategory:  " kpvp ",
		PlayerPUUID:    " puuid-1 ",
		PlayerRiotID:   " Ahri#NA1 ",
		TrackedCount:   2,
		LiveChannelID:  " track-channel ",
		LastLiveSeenAt: time.Now().UTC().Add(-2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("UpsertTrackMatchNotificationLive() error = %v", err)
	}
	if err := fx.db.MarkTrackMatchLivePosted(fx.ctx, notification.Key(), " track-channel ", " live-msg-1 ", time.Now().UTC()); err != nil {
		t.Fatalf("MarkTrackMatchLivePosted() error = %v", err)
	}
	return notification
}

func (fx trackFixture) listTracked(t *testing.T, guildID string) []TrackedAccount {
	t.Helper()
	out, err := fx.db.ListTrackedAccounts(fx.ctx, guildID, 10)
	if err != nil {
		t.Fatalf("ListTrackedAccounts() error = %v", err)
	}
	return out
}

func (fx trackFixture) pendingByKey(t *testing.T, key TrackMatchNotificationKey, at time.Time, want bool) TrackMatchNotification {
	t.Helper()

	list, err := fx.db.ListPendingTrackMatchNotifications(fx.ctx, at, 500)
	if err != nil {
		t.Fatalf("ListPendingTrackMatchNotifications() error = %v", err)
	}
	entry, found := findNotificationByKey(list, key)
	if found != want {
		t.Fatalf("pending state mismatch: got=%v want=%v key=%+v", found, want, key)
	}
	return entry
}

func openTrackIntegrationDB(t *testing.T, ctx context.Context) (*Database, string) {
	t.Helper()

	dbURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if dbURL == "" {
		t.Skip("set TEST_DATABASE_URL to run postgres integration tests")
	}

	pool, err := NewPool(ctx, dbURL)
	if err != nil {
		t.Fatalf("NewPool() error = %v", err)
	}
	db := NewDB(pool)
	t.Cleanup(pool.Close)

	if err := db.CreateTrackTable(ctx); err != nil {
		t.Fatalf("CreateTrackTable() error = %v", err)
	}

	prefix := integrationPrefix(t.Name())
	cleanupTrackIntegrationData(t, db, prefix)
	t.Cleanup(func() { cleanupTrackIntegrationData(t, db, prefix) })
	return db, prefix
}

func integrationPrefix(testName string) string {
	sanitized := strings.ToLower(testName)
	sanitized = strings.NewReplacer("/", "_", " ", "_", "-", "_").Replace(sanitized)
	return "it_" + sanitized + "_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
}

func cleanupTrackIntegrationData(t *testing.T, db *Database, prefix string) {
	t.Helper()
	if db == nil || db.pool == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	guildPrefix := prefix + "%"
	if _, err := db.pool.Exec(ctx, `DELETE FROM track_match_notifications WHERE guild_id LIKE $1`, guildPrefix); err != nil {
		t.Fatalf("cleanup track_match_notifications: %v", err)
	}
	if _, err := db.pool.Exec(ctx, `DELETE FROM track_accounts WHERE guild_id LIKE $1`, guildPrefix); err != nil {
		t.Fatalf("cleanup track_accounts: %v", err)
	}
	if _, err := db.pool.Exec(ctx, `DELETE FROM track_guild_config WHERE guild_id LIKE $1`, guildPrefix); err != nil {
		t.Fatalf("cleanup track_guild_config: %v", err)
	}
	if _, err := db.pool.Exec(ctx, `DELETE FROM track_match_snapshots WHERE match_id ILIKE $1`, prefix+"%"); err != nil {
		t.Fatalf("cleanup track_match_snapshots: %v", err)
	}
}

func findNotificationByKey(list []TrackMatchNotification, key TrackMatchNotificationKey) (TrackMatchNotification, bool) {
	for _, item := range list {
		if item.GuildID == key.GuildID && item.PlatformID == key.PlatformID && item.GameID == key.GameID {
			return item, true
		}
	}
	return TrackMatchNotification{}, false
}
