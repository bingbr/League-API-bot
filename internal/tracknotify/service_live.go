package tracknotify

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/sync/errgroup"
)

func (s *Service) buildActiveMatches(ctx context.Context, targets []postgres.TrackNotificationTarget) map[guildMatchKey]*liveGuildMatch {
	active := map[guildMatchKey]*liveGuildMatch{}
	if len(targets) == 0 {
		return active
	}

	probeKeys := make(map[targetProbeKey]struct{}, len(targets))
	for _, target := range targets {
		key := targetProbeKey{
			PlatformRegion: riot.NormalizePlatformRegion(target.PlatformRegion),
			PUUID:          strings.TrimSpace(target.PUUID),
		}
		if key.PlatformRegion == "" || key.PUUID == "" {
			continue
		}
		probeKeys[key] = struct{}{}
	}

	results, stats := s.fetchLiveGames(ctx, probeKeys)
	s.logger.Debug("Track notify live probe summary", "targets", len(targets), "uniqueProbes", len(probeKeys), "checked", stats.Checked, "liveGames", stats.LiveGames, "notInGame", stats.NotInGame, "errors", stats.Errors)
	for _, target := range targets {
		platformRegion := riot.NormalizePlatformRegion(target.PlatformRegion)
		puuid := strings.TrimSpace(target.PUUID)
		if platformRegion == "" || puuid == "" {
			continue
		}

		game, ok := results[targetProbeKey{PlatformRegion: platformRegion, PUUID: puuid}]
		if !ok || game == nil {
			continue
		}

		platformID := strings.ToUpper(strings.TrimSpace(game.PlatformID))
		if platformID == "" {
			platformID = strings.ToUpper(platformRegion)
		}
		key := guildMatchKey{GuildID: strings.TrimSpace(target.GuildID), PlatformID: platformID, GameID: game.GameID}
		if key.GuildID == "" || key.PlatformID == "" || key.GameID <= 0 {
			continue
		}

		entry, ok := active[key]
		if !ok {
			entry = &liveGuildMatch{
				Key:            key,
				GuildID:        key.GuildID,
				ChannelID:      strings.TrimSpace(target.ChannelID),
				PlatformRegion: platformRegion,
				PlatformID:     platformID,
				Game:           game,
				TrackedByPUUID: make(map[string]string),
			}
			active[key] = entry
		}

		riotID := target.RiotID()
		if riotID == "" {
			riotID = puuid
		}
		entry.TrackedByPUUID[puuid] = riotID
	}
	return active
}

func (s *Service) fetchLiveGames(ctx context.Context, keys map[targetProbeKey]struct{}) (map[targetProbeKey]*riot.LiveGame, liveFetchStats) {
	results := make(map[targetProbeKey]*riot.LiveGame, len(keys))
	stats := liveFetchStats{}
	if len(keys) == 0 {
		return results, stats
	}

	var mu sync.Mutex
	var g, gctx = errgroup.WithContext(ctx)
	g.SetLimit(defaultFetchLimit)
	for key := range keys {
		g.Go(func() error {
			game, err := riot.FetchActiveGameBySummoner(gctx, key.PlatformRegion, key.PUUID, s.riotAPIKey)

			mu.Lock()
			stats.Checked++
			if err != nil {
				if httpStatusIs(err, 404) {
					stats.NotInGame++
				} else {
					stats.Errors++
				}
			} else {
				results[key] = &game
				stats.LiveGames++
			}
			mu.Unlock()

			if err != nil && !httpStatusIs(err, 404) {
				s.logger.Warn("Live game check failed", "platformRegion", key.PlatformRegion, "puuid", key.PUUID, "error", err)
			}
			return nil
		})
	}
	_ = g.Wait()
	return results, stats
}

func (s *Service) publishLiveEmbeds(ctx context.Context, active map[guildMatchKey]*liveGuildMatch) {
	if len(active) == 0 {
		s.logger.Debug("No live matches to publish in this tick")
		return
	}
	for _, match := range active {
		playerPUUID, playerRiotID, trackedCount := selectPlayerTracked(match.TrackedByPUUID)
		if playerPUUID == "" || playerRiotID == "" || trackedCount <= 0 {
			s.logger.Debug("Skipping live match without valid tracked player", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID)
			continue
		}
		queueDisplay, found, err := s.database.QueueDisplayByID(ctx, match.Game.GameQueueConfigID)
		if err != nil {
			s.logger.Warn("Failed to load queue display for live match", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "queueID", match.Game.GameQueueConfigID, "error", err)
			continue
		}
		if !found || strings.TrimSpace(queueDisplay.Name) == "" {
			s.logger.Debug("Skipping live match without queue name", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "queueID", match.Game.GameQueueConfigID)
			continue
		}

		notification, err := s.database.UpsertTrackMatchNotificationLive(ctx, postgres.UpsertTrackMatchLiveInput{
			GuildID:        match.GuildID,
			PlatformID:     match.PlatformID,
			GameID:         match.Game.GameID,
			MatchID:        riot.BuildMatchID(match.PlatformID, match.Game.GameID),
			PlayerPUUID:    playerPUUID,
			PlayerRiotID:   playerRiotID,
			TrackedCount:   trackedCount,
			QueueID:        match.Game.GameQueueConfigID,
			QueueCategory:  strings.TrimSpace(queueDisplay.GameSelectCategory),
			LiveChannelID:  match.ChannelID,
			LastLiveSeenAt: time.Now().UTC(),
		})
		if err != nil {
			s.logger.Error("Failed to upsert live notification", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "error", err)
			continue
		}
		if strings.TrimSpace(notification.LiveMessageID) != "" {
			s.logger.Debug("Live notification already posted", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "messageID", notification.LiveMessageID)
			continue
		}

		queueName := strings.TrimSpace(queueDisplay.Name)
		embed, err := s.buildLiveEmbed(ctx, match, playerRiotID, queueName)
		if err != nil {
			s.logger.Warn("Failed to build live embed", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "error", err)
			continue
		}

		msg, err := s.session.ChannelMessageSendComplex(match.ChannelID, &discordgo.MessageSend{
			Embeds: []*discordgo.MessageEmbed{embed},
		})
		if err != nil {
			if s.disableGuildTrackingOnAccessLoss(ctx, match.GuildID, match.ChannelID, err) {
				continue
			}
			s.logger.Warn("Failed to send live embed", "guildID", match.GuildID, "channelID", match.ChannelID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "error", err)
			continue
		}
		s.logger.Info("Live notification posted", "guildID", match.GuildID, "channelID", match.ChannelID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "messageID", msg.ID)
		if err := s.database.MarkTrackMatchLivePosted(ctx, notification.Key(), match.ChannelID, msg.ID, time.Now().UTC()); err != nil {
			s.logger.Warn("Failed to persist live notification message", "guildID", match.GuildID, "platformID", match.PlatformID, "gameID", match.Game.GameID, "error", err)
		}
	}
}
