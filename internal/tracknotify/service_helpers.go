package tracknotify

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
)

func httpStatusIs(err error, statusCode int) bool {
	statusErr, ok := errors.AsType[*riot.HTTPStatusError](err)
	return ok && statusErr.StatusCode == statusCode
}

func isDiscordMissingAccess(err error) bool {
	restErr, ok := errors.AsType[*discordgo.RESTError](err)
	if !ok || restErr == nil || restErr.Message == nil {
		return false
	}
	code := restErr.Message.Code
	return code == 50001 || code == 50013
}

func selectPlayerTracked(values map[string]string) (playerPUUID, playerRiotID string, trackedCount int) {
	pairs := collectTrackedPairs(values)
	if len(pairs) == 0 {
		return "", "", 0
	}
	return pairs[0].PUUID, pairs[0].RiotID, len(pairs)
}

func collectTrackedPairs(values map[string]string) []trackedPair {
	pairs := make([]trackedPair, 0, len(values))
	for puuid, riotID := range values {
		puuid = strings.TrimSpace(puuid)
		if puuid == "" {
			continue
		}
		riotID = strings.TrimSpace(riotID)
		if riotID == "" {
			riotID = puuid
		}
		pairs = append(pairs, trackedPair{PUUID: puuid, RiotID: riotID})
	}
	if len(pairs) == 0 {
		return nil
	}
	slices.SortFunc(pairs, func(a, b trackedPair) int {
		if c := cmp.Compare(strings.ToLower(a.RiotID), strings.ToLower(b.RiotID)); c != 0 {
			return c
		}
		return cmp.Compare(a.PUUID, b.PUUID)
	})
	return pairs
}

func otherTrackedRiotIDs(values map[string]string, subjectRiotID string) []string {
	pairs := collectTrackedPairs(values)
	if len(pairs) == 0 {
		return nil
	}
	subjectRiotID = strings.TrimSpace(subjectRiotID)
	out := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		if subjectRiotID != "" && strings.EqualFold(pair.RiotID, subjectRiotID) {
			continue
		}
		out = append(out, pair.RiotID)
	}
	return out
}

func trackedTargetsByGuildPlatform(targets []postgres.TrackNotificationTarget) map[guildPlatformKey]map[string]string {
	out := make(map[guildPlatformKey]map[string]string)
	for _, target := range targets {
		guildID := strings.TrimSpace(target.GuildID)
		platformID := strings.ToUpper(riot.NormalizePlatformRegion(target.PlatformRegion))
		puuid := strings.TrimSpace(target.PUUID)
		if guildID == "" || platformID == "" || puuid == "" {
			continue
		}

		key := guildPlatformKey{
			GuildID:    guildID,
			PlatformID: platformID,
		}
		byPUUID, ok := out[key]
		if !ok {
			byPUUID = make(map[string]string)
			out[key] = byPUUID
		}

		riotID := strings.TrimSpace(target.RiotID())
		if riotID == "" {
			riotID = puuid
		}
		byPUUID[puuid] = riotID
	}
	return out
}

func trackedPairsInMatch(players []riot.MatchPlayer, trackedByPUUID map[string]string) []trackedPair {
	if len(players) == 0 || len(trackedByPUUID) == 0 {
		return nil
	}

	matched := make(map[string]string)
	for _, player := range players {
		puuid := strings.TrimSpace(player.PUUID)
		if puuid == "" {
			continue
		}
		riotID, ok := trackedByPUUID[puuid]
		if !ok {
			continue
		}
		riotID = strings.TrimSpace(riotID)
		if riotID == "" {
			riotID = matchPlayerRiotID(player)
		}
		matched[puuid] = riotID
	}
	return collectTrackedPairs(matched)
}

func formatTrackedPlayersLine(riotIDs []string) string {
	players := make([]string, 0, len(riotIDs))
	for _, riotID := range riotIDs {
		riotID = strings.TrimSpace(riotID)
		if riotID != "" {
			players = append(players, fmt.Sprintf("**%s**", riotID))
		}
	}
	switch len(players) {
	case 0:
		return ""
	case 1:
		return players[0]
	case 2:
		return players[0] + " and " + players[1]
	default:
		return strings.Join(players[:len(players)-1], ", ") + " and " + players[len(players)-1]
	}
}

func formatLiveDescription(tracked []string, queueName, mapName string) string {
	trackedLine := formatTrackedPlayersLine(tracked)
	playLine := fmt.Sprintf("Is playing %s on %s.", strings.TrimSpace(queueName), strings.TrimSpace(mapName))
	if trackedLine == "" {
		return playLine
	}
	return fmt.Sprintf("%s\nPlaying with %s.", playLine, trackedLine)
}

func queueSupportsPostGame(category string) bool {
	return strings.EqualFold(strings.TrimSpace(category), queueCategoryPvP)
}

func postRetryDelay(attempt int) time.Duration {
	if attempt <= 1 {
		return postRetryBaseDelay
	}
	return min(postRetryBaseDelay<<min(attempt-1, 30), postRetryMaxDelay)
}

func truncateError(message string) string {
	if len(message) <= 500 {
		return strings.TrimSpace(message)
	}
	return strings.TrimSpace(message[:500])
}
