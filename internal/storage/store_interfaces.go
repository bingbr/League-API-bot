package storage

import (
	"context"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
)

type FreeWeekDB interface {
	GetFreeWeekRotation(ctx context.Context, platformRegion string) (riot.ChampionRotation, time.Time, time.Time, bool, error)
	UpsertFreeWeekRotation(ctx context.Context, platformRegion string, rotation riot.ChampionRotation, fetchedAt, expiresAt time.Time) error
	ChampionDisplayByIDs(ctx context.Context, ids []int) (map[int]postgres.ChampionDisplay, error)
}

type SearchDB interface {
	RankIconsByTiers(ctx context.Context, tiers []string) (map[string]string, error)
}

type TrackDB interface {
	UpsertTrackGuildConfig(ctx context.Context, guildID, channelID string) error
	DisableTrackGuildConfig(ctx context.Context, guildID string) error
	TrackGuildConfig(ctx context.Context, guildID string) (postgres.TrackGuildConfig, bool, error)
	AddTrackedAccount(ctx context.Context, account postgres.TrackedAccount) (bool, error)
	RemoveTrackedAccount(ctx context.Context, guildID, riotID string) (bool, error)
	ListTrackedAccounts(ctx context.Context, guildID string, limit int) ([]postgres.TrackedAccount, error)
}

type TrackNotifyDB interface {
	DisableTrackGuildConfig(ctx context.Context, guildID string) error
	ListTrackNotificationTargets(ctx context.Context) ([]postgres.TrackNotificationTarget, error)
	UpsertTrackMatchNotificationLive(ctx context.Context, input postgres.UpsertTrackMatchLiveInput) (postgres.TrackMatchNotification, error)
	MarkTrackMatchLivePosted(ctx context.Context, key postgres.TrackMatchNotificationKey, channelID, messageID string, postedAt time.Time) error
	ListPendingTrackMatchNotifications(ctx context.Context, now time.Time, limit int) ([]postgres.TrackMatchNotification, error)
	MarkTrackMatchPostRetry(ctx context.Context, key postgres.TrackMatchNotificationKey, attempts int, nextAttemptAt time.Time, lastError string) error
	MarkTrackMatchPostPosted(ctx context.Context, key postgres.TrackMatchNotificationKey, messageID string, postedAt time.Time) error
	AbandonTrackMatchNotification(ctx context.Context, key postgres.TrackMatchNotificationKey, abandonedAt time.Time, lastError string) error
	GetTrackMatchSnapshot(ctx context.Context, matchID string) (riot.MatchDetail, bool, error)
	UpsertTrackMatchSnapshot(ctx context.Context, match riot.MatchDetail) error
	CleanupTrackMatchNotifications(ctx context.Context, olderThan time.Time) (int64, error)
	QueueDisplayByID(ctx context.Context, queueID int) (postgres.QueueDisplay, bool, error)
	MapDisplayByID(ctx context.Context, mapID int) (postgres.MapDisplay, bool, error)
	SummonerSpellDisplayByIDs(ctx context.Context, ids []int) (map[int]postgres.SummonerSpellDisplay, error)
	RuneTreeDisplayByIDs(ctx context.Context, ids []int) (map[int]postgres.RuneTreeDisplay, error)
	RuneDisplayByIDs(ctx context.Context, ids []int) (map[int]postgres.RuneDisplay, error)
	ItemDisplayByIDs(ctx context.Context, ids []int) (map[int]postgres.ItemDisplay, error)
}

type CommandDB interface {
	FreeWeekDB
	SearchDB
	TrackDB
}
