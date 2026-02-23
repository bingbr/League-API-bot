package tracknotify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
)

func TestPublishPostEmbeds_AbandonOnMissingContinentHandlesDBErrorAndContinues(t *testing.T) {
	now := time.Now().UTC()
	db := &postPublishTestDB{
		pending: []postgres.TrackMatchNotification{
			{
				GuildID:        "g1",
				PlatformID:     "xx1",
				GameID:         101,
				QueueCategory:  queueCategoryPvP,
				LastLiveSeenAt: now,
			},
			{
				GuildID:        "g1",
				PlatformID:     "yy1",
				GameID:         202,
				QueueCategory:  queueCategoryPvP,
				LastLiveSeenAt: now,
			},
		},
		abandonErr: errors.New("write failed"),
	}
	var logs bytes.Buffer
	service := &Service{
		database: db,
		logger:   slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	}

	service.publishPostEmbeds(context.Background(), map[guildMatchKey]*liveGuildMatch{}, nil)

	if len(db.abandonCalls) != 2 {
		t.Fatalf("abandon calls = %d, want 2", len(db.abandonCalls))
	}
	for _, call := range db.abandonCalls {
		if call.reason != "platform has no continent mapping" {
			t.Fatalf("abandon reason = %q, want platform has no continent mapping", call.reason)
		}
	}

	logOutput := logs.String()
	if !strings.Contains(logOutput, "Failed to abandon notification without continent mapping") {
		t.Fatalf("logs missing warning message: %s", logOutput)
	}
	if !strings.Contains(logOutput, "guildID=g1") || !strings.Contains(logOutput, "platformID=xx1") || !strings.Contains(logOutput, "gameID=101") {
		t.Fatalf("logs missing abandon context fields: %s", logOutput)
	}
}

func TestResolvePostMatchUsesSnapshot(t *testing.T) {
	called := false
	withFetchMatchByIDStub(t, func(context.Context, string, string, string) (riot.MatchDetail, error) {
		called = true
		return riot.MatchDetail{}, errors.New("fetch should not be called")
	})

	match := riot.MatchDetail{
		Metadata: riot.MatchMetadata{MatchID: "NA1_123"},
		Info:     riot.MatchInfo{QueueID: 420},
	}
	db := &postPublishTestDB{
		snapshots: map[string]riot.MatchDetail{
			"NA1_123": match,
		},
	}
	service := newPostTestService(db, io.Discard)

	got, err := service.resolvePostMatch(context.Background(), postgres.TrackMatchNotification{
		GuildID:    "g1",
		PlatformID: "NA1",
		GameID:     123,
		MatchID:    "NA1_123",
	}, "americas")
	if err != nil {
		t.Fatalf("resolvePostMatch() error = %v", err)
	}
	if called {
		t.Fatalf("expected fetchMatchByID not to be called when snapshot exists")
	}
	if got.Metadata.MatchID != "NA1_123" {
		t.Fatalf("match id = %q, want %q", got.Metadata.MatchID, "NA1_123")
	}
	if len(db.upsertedSnapshots) != 0 {
		t.Fatalf("upserted snapshots = %d, want 0", len(db.upsertedSnapshots))
	}
}

func TestResolvePostMatchFetchesAndCachesOnSnapshotMiss(t *testing.T) {
	match := riot.MatchDetail{
		Metadata: riot.MatchMetadata{MatchID: "EUW1_456"},
		Info:     riot.MatchInfo{QueueID: 440},
	}
	withFetchMatchByIDStub(t, func(_ context.Context, continent, matchID, _ string) (riot.MatchDetail, error) {
		if continent != "europe" {
			t.Fatalf("continent = %q, want %q", continent, "europe")
		}
		if matchID != "EUW1_456" {
			t.Fatalf("matchID = %q, want %q", matchID, "EUW1_456")
		}
		return match, nil
	})

	db := &postPublishTestDB{}
	service := newPostTestService(db, io.Discard)

	got, err := service.resolvePostMatch(context.Background(), postgres.TrackMatchNotification{
		GuildID:    "g1",
		PlatformID: "EUW1",
		GameID:     456,
		MatchID:    "EUW1_456",
	}, "europe")
	if err != nil {
		t.Fatalf("resolvePostMatch() error = %v", err)
	}
	if got.Metadata.MatchID != "EUW1_456" {
		t.Fatalf("match id = %q, want %q", got.Metadata.MatchID, "EUW1_456")
	}
	if len(db.upsertedSnapshots) != 1 {
		t.Fatalf("upserted snapshots = %d, want 1", len(db.upsertedSnapshots))
	}
	if db.upsertedSnapshots[0].Metadata.MatchID != "EUW1_456" {
		t.Fatalf("cached match id = %q, want %q", db.upsertedSnapshots[0].Metadata.MatchID, "EUW1_456")
	}
}

type abandonCall struct {
	key    postgres.TrackMatchNotificationKey
	reason string
}

type postPublishTestDB struct {
	pending           []postgres.TrackMatchNotification
	pendingErr        error
	abandonErr        error
	abandonCalls      []abandonCall
	snapshots         map[string]riot.MatchDetail
	snapshotErr       error
	upsertSnapshotErr error
	upsertedSnapshots []riot.MatchDetail
}

func (d *postPublishTestDB) ListTrackNotificationTargets(context.Context) ([]postgres.TrackNotificationTarget, error) {
	return nil, nil
}

func (d *postPublishTestDB) UpsertTrackMatchNotificationLive(context.Context, postgres.UpsertTrackMatchLiveInput) (postgres.TrackMatchNotification, error) {
	panic("unexpected call to UpsertTrackMatchNotificationLive")
}

func (d *postPublishTestDB) DisableTrackGuildConfig(context.Context, string) error {
	return nil
}

func (d *postPublishTestDB) MarkTrackMatchLivePosted(context.Context, postgres.TrackMatchNotificationKey, string, string, time.Time) error {
	panic("unexpected call to MarkTrackMatchLivePosted")
}

func (d *postPublishTestDB) ListPendingTrackMatchNotifications(context.Context, time.Time, int) ([]postgres.TrackMatchNotification, error) {
	return d.pending, d.pendingErr
}

func (d *postPublishTestDB) MarkTrackMatchPostRetry(context.Context, postgres.TrackMatchNotificationKey, int, time.Time, string) error {
	panic("unexpected call to MarkTrackMatchPostRetry")
}

func (d *postPublishTestDB) MarkTrackMatchPostPosted(context.Context, postgres.TrackMatchNotificationKey, string, time.Time) error {
	panic("unexpected call to MarkTrackMatchPostPosted")
}

func (d *postPublishTestDB) AbandonTrackMatchNotification(_ context.Context, key postgres.TrackMatchNotificationKey, _ time.Time, reason string) error {
	d.abandonCalls = append(d.abandonCalls, abandonCall{key: key, reason: reason})
	return d.abandonErr
}

func (d *postPublishTestDB) GetTrackMatchSnapshot(_ context.Context, matchID string) (riot.MatchDetail, bool, error) {
	if d.snapshotErr != nil {
		return riot.MatchDetail{}, false, d.snapshotErr
	}
	match, ok := d.snapshots[matchID]
	return match, ok, nil
}

func (d *postPublishTestDB) UpsertTrackMatchSnapshot(_ context.Context, match riot.MatchDetail) error {
	d.upsertedSnapshots = append(d.upsertedSnapshots, match)
	return d.upsertSnapshotErr
}

func (d *postPublishTestDB) CleanupTrackMatchNotifications(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (d *postPublishTestDB) QueueDisplayByID(context.Context, int) (postgres.QueueDisplay, bool, error) {
	panic("unexpected call to QueueDisplayByID")
}

func (d *postPublishTestDB) MapDisplayByID(context.Context, int) (postgres.MapDisplay, bool, error) {
	panic("unexpected call to MapDisplayByID")
}

func (d *postPublishTestDB) SummonerSpellDisplayByIDs(context.Context, []int) (map[int]postgres.SummonerSpellDisplay, error) {
	panic("unexpected call to SummonerSpellDisplayByIDs")
}

func (d *postPublishTestDB) RuneTreeDisplayByIDs(context.Context, []int) (map[int]postgres.RuneTreeDisplay, error) {
	panic("unexpected call to RuneTreeDisplayByIDs")
}

func (d *postPublishTestDB) RuneDisplayByIDs(context.Context, []int) (map[int]postgres.RuneDisplay, error) {
	panic("unexpected call to RuneDisplayByIDs")
}

func (d *postPublishTestDB) ItemDisplayByIDs(context.Context, []int) (map[int]postgres.ItemDisplay, error) {
	panic("unexpected call to ItemDisplayByIDs")
}

func (d *postPublishTestDB) RankIconsByTiers(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (d *postPublishTestDB) GetFreeWeekRotation(context.Context, string) (riot.ChampionRotation, time.Time, time.Time, bool, error) {
	return riot.ChampionRotation{}, time.Time{}, time.Time{}, false, nil
}

func (d *postPublishTestDB) UpsertFreeWeekRotation(context.Context, string, riot.ChampionRotation, time.Time, time.Time) error {
	return nil
}

func (d *postPublishTestDB) ChampionDisplayByIDs(context.Context, []int) (map[int]postgres.ChampionDisplay, error) {
	return map[int]postgres.ChampionDisplay{}, nil
}

func withFetchMatchByIDStub(t *testing.T, fn func(context.Context, string, string, string) (riot.MatchDetail, error)) {
	t.Helper()
	original := fetchMatchByID
	fetchMatchByID = fn
	t.Cleanup(func() { fetchMatchByID = original })
}

func newPostTestService(db *postPublishTestDB, sink io.Writer) *Service {
	return &Service{
		database: db,
		logger:   slog.New(slog.NewTextHandler(sink, nil)),
	}
}
