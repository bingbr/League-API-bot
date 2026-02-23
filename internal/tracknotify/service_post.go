package tracknotify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
)

var fetchMatchByID = riot.FetchMatchByID

func (s *Service) publishPostEmbeds(ctx context.Context, active map[guildMatchKey]*liveGuildMatch, targets []postgres.TrackNotificationTarget) {
	now := time.Now().UTC()
	pending, err := s.database.ListPendingTrackMatchNotifications(ctx, now, defaultPostPendingLimit)
	if err != nil {
		s.logger.Warn("Failed to list pending post notifications", "error", err)
		return
	}
	s.logger.Debug("Track notify pending post notifications", "count", len(pending))
	if len(pending) == 0 {
		return
	}

	targetsByGuildPlatform := trackedTargetsByGuildPlatform(targets)
	for _, notification := range pending {
		key := guildMatchKey{GuildID: notification.GuildID, PlatformID: notification.PlatformID, GameID: notification.GameID}
		if _, live := active[key]; live {
			continue
		}

		if now.Sub(notification.LastLiveSeenAt) > postAbandonAfter {
			s.abandonPostNotification(ctx, notification, now, "post-game timeout exceeded", "Failed to abandon post notification")
			continue
		}
		if !queueSupportsPostGame(notification.QueueCategory) {
			s.abandonPostNotification(ctx, notification, now, "queue category has no post-game", "Failed to abandon non-post-game queue", "queueID", notification.QueueID)
			continue
		}

		continent := riot.PlatformContinent(strings.ToLower(notification.PlatformID))
		if continent == "" {
			s.abandonPostNotification(ctx, notification, now, "platform has no continent mapping", "Failed to abandon notification without continent mapping")
			continue
		}

		match, err := s.resolvePostMatch(ctx, notification, continent)
		if err != nil {
			s.schedulePostRetry(ctx, notification, now, fmt.Sprintf("fetch match failed: %v", err))
			continue
		}
		queueDisplay, found, err := s.database.QueueDisplayByID(ctx, match.Info.QueueID)
		if err != nil {
			s.schedulePostRetry(ctx, notification, now, fmt.Sprintf("load queue display failed: %v", err))
			continue
		}
		queueName := strings.TrimSpace(queueDisplay.Name)
		if !found || queueName == "" {
			s.abandonPostNotification(ctx, notification, now, errQueueNameUnavailable.Error(), "Failed to abandon non-postable notification")
			continue
		}
		if !queueSupportsPostGame(queueDisplay.GameSelectCategory) {
			s.abandonPostNotification(ctx, notification, now, errQueueHasNoPostGame.Error(), "Failed to abandon non-postable notification")
			continue
		}

		guildPlatform := guildPlatformKey{GuildID: notification.GuildID, PlatformID: strings.ToUpper(strings.TrimSpace(notification.PlatformID))}
		trackedPairs := trackedPairsInMatch(match.Info.Players, targetsByGuildPlatform[guildPlatform])
		if len(trackedPairs) == 0 {
			trackedPairs = []trackedPair{{
				PUUID:  notification.PlayerPUUID,
				RiotID: notification.PlayerRiotID,
			}}
		}

		embeds := make([]*discordgo.MessageEmbed, 0, len(trackedPairs))
		for _, pair := range trackedPairs {
			perPlayer := notification
			perPlayer.PlayerPUUID = pair.PUUID
			perPlayer.PlayerRiotID = pair.RiotID
			perPlayer.TrackedCount = len(trackedPairs)

			embed, err := s.buildPostEmbed(ctx, perPlayer, match, queueName)
			if err != nil {
				s.logger.Warn("Failed to build post embed for tracked player", "guildID", notification.GuildID, "platformID", notification.PlatformID, "gameID", notification.GameID, "playerPUUID", pair.PUUID, "error", err)
				continue
			}
			embeds = append(embeds, embed)
		}
		if len(embeds) == 0 {
			s.schedulePostRetry(ctx, notification, now, "build post embed failed: no tracked players found in match")
			continue
		}

		lastMessageID, sendErr := s.sendPostEmbedBatches(notification, embeds)
		if sendErr != nil {
			if s.disableGuildTrackingOnAccessLoss(ctx, notification.GuildID, notification.LiveChannelID, sendErr) {
				s.abandonPostNotification(ctx, notification, now, "discord access lost to configured channel", "Failed to abandon post notification after disabling tracking")
				continue
			}
			s.schedulePostRetry(ctx, notification, now, fmt.Sprintf("send post embed failed: %v", sendErr))
			continue
		}

		if err := s.database.MarkTrackMatchPostPosted(ctx, notification.Key(), lastMessageID, now); err != nil {
			s.logger.Warn("Failed to persist post notification message", "guildID", notification.GuildID, "platformID", notification.PlatformID, "gameID", notification.GameID, "error", err)
		}
	}
}

func (s *Service) resolvePostMatch(ctx context.Context, notification postgres.TrackMatchNotification, continent string) (riot.MatchDetail, error) {
	matchID := strings.TrimSpace(notification.MatchID)
	if matchID == "" {
		return riot.MatchDetail{}, fmt.Errorf("match id is required")
	}

	snapshot, found, err := s.database.GetTrackMatchSnapshot(ctx, matchID)
	if err != nil {
		if s.logger != nil {
			s.logger.Warn("Failed to load track match snapshot", "guildID", notification.GuildID, "platformID", notification.PlatformID, "gameID", notification.GameID, "matchID", matchID, "error", err)
		}
	} else if found {
		return snapshot, nil
	}

	match, err := fetchMatchByID(ctx, continent, matchID, s.riotAPIKey)
	if err != nil {
		return riot.MatchDetail{}, err
	}
	if err := s.database.UpsertTrackMatchSnapshot(ctx, match); err != nil && s.logger != nil {
		s.logger.Warn("Failed to cache track match snapshot", "guildID", notification.GuildID, "platformID", notification.PlatformID, "gameID", notification.GameID, "matchID", matchID, "error", err)
	}
	return match, nil
}

func (s *Service) schedulePostRetry(ctx context.Context, notification postgres.TrackMatchNotification, now time.Time, reason string) {
	attempts := notification.PostAttempts + 1
	nextAttempt := now.Add(postRetryDelay(attempts))
	err := s.database.MarkTrackMatchPostRetry(ctx, notification.Key(), attempts, nextAttempt, truncateError(reason))
	if err != nil {
		s.logger.Warn("Failed to schedule post retry", "guildID", notification.GuildID, "platformID", notification.PlatformID, "gameID", notification.GameID, "error", err)
	}
}

func (s *Service) abandonPostNotification(ctx context.Context, notification postgres.TrackMatchNotification, now time.Time, reason string, logMessage string, extraFields ...any) {
	if err := s.database.AbandonTrackMatchNotification(ctx, notification.Key(), now, reason); err != nil {
		fields := []any{
			"guildID", notification.GuildID,
			"platformID", notification.PlatformID,
			"gameID", notification.GameID,
		}
		fields = append(fields, extraFields...)
		fields = append(fields, "error", err)
		s.logger.Warn(logMessage, fields...)
	}
}

func (s *Service) sendPostEmbedBatches(notification postgres.TrackMatchNotification, embeds []*discordgo.MessageEmbed) (string, error) {
	lastMessageID := ""
	for idx := 0; idx < len(embeds); idx += 10 {
		end := min(idx+10, len(embeds))
		send := &discordgo.MessageSend{Embeds: embeds[idx:end]}
		if idx == 0 {
			send.Reference = &discordgo.MessageReference{
				MessageID:       notification.LiveMessageID,
				ChannelID:       notification.LiveChannelID,
				GuildID:         notification.GuildID,
				FailIfNotExists: new(false),
			}
		}

		msg, err := s.session.ChannelMessageSendComplex(notification.LiveChannelID, send)
		if err != nil {
			return "", err
		}
		lastMessageID = msg.ID
	}
	return lastMessageID, nil
}
