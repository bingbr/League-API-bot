package tracknotify

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"unicode"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/sync/errgroup"
)

func (s *Service) buildLiveEmbed(ctx context.Context, match *liveGuildMatch, playerRiotID string, queueName string) (*discordgo.MessageEmbed, error) {
	championIDs := collectLiveChampionIDs(match.Game.Players, match.Game.BannedChampions)
	champions := loadOrEmptyMap(func() (map[int]postgres.ChampionDisplay, error) {
		return s.database.ChampionDisplayByIDs(ctx, championIDs)
	})

	soloEntries := s.fetchSoloEntries(ctx, match.PlatformRegion, match.Game.Players)
	rankIcons := loadOrEmptyMap(func() (map[string]string, error) {
		return s.database.RankIconsByTiers(ctx, riot.RankTiersToLookupByPUUID(soloEntries))
	})

	queueName = strings.TrimSpace(queueName)
	if queueName == "" {
		return nil, errQueueNameUnavailable
	}

	mapDisplay, mapFound, err := s.database.MapDisplayByID(ctx, match.Game.MapID)
	if err != nil {
		return nil, fmt.Errorf("load map display: %w", err)
	}
	mapName := strings.TrimSpace(mapDisplay.Name)
	if !mapFound || mapName == "" {
		mapName = fmt.Sprintf("Map %d", match.Game.MapID)
	}

	blueTeam, redTeam := buildLiveTeamRows(match.Game.Players, champions, soloEntries, rankIcons)
	showBlueTeam, showRedTeam := liveTeamsToRender(match.Game.Players, blueTeam, redTeam)
	fields := buildLiveTeamFields(blueTeam, redTeam, showBlueTeam, showRedTeam)

	blueBans, redBans := bansLineLive(match.Game.BannedChampions, 100, champions), bansLineLive(match.Game.BannedChampions, 200, champions)
	if showBlueTeam && blueBans != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "🔵 Bans", Value: blueBans, Inline: true})
	}
	if showRedTeam && redBans != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "🔴 Bans", Value: redBans, Inline: true})
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Live Game",
			IconURL: cdn.ProfileIconURL(5376),
		},
		Color:       0xf9f9f9,
		Title:       strings.TrimSpace(playerRiotID),
		Description: formatLiveDescription(otherTrackedRiotIDs(match.TrackedByPUUID, playerRiotID), queueName, mapName),
		Fields:      fields,
	}
	discord.ApplyDefaultFooter(embed)
	return embed, nil
}

func (s *Service) fetchSoloEntries(ctx context.Context, platformRegion string, players []riot.LiveGamePlayer) map[string]*riot.LeagueEntry {
	platformRegion = riot.NormalizePlatformRegion(platformRegion)
	out := map[string]*riot.LeagueEntry{}

	var (
		mu      sync.Mutex
		g, gctx = errgroup.WithContext(ctx)
	)
	g.SetLimit(defaultFetchLimit)
	for _, player := range players {
		puuid := strings.TrimSpace(player.PUUID)
		if puuid == "" {
			continue
		}
		mu.Lock()
		_, seen := out[puuid]
		if !seen {
			out[puuid] = nil // reserve slot to prevent duplicates
		}
		mu.Unlock()
		if seen {
			continue
		}

		g.Go(func() error {
			entries, err := riot.FetchLeagueEntriesByPUUID(gctx, platformRegion, puuid, s.riotAPIKey)
			if err != nil {
				return nil
			}
			if entry := riot.QueueEntry(entries, soloQueueType); entry != nil {
				copyEntry := *entry
				mu.Lock()
				out[puuid] = &copyEntry
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return out
}

type liveTeamRows struct {
	Names    []string
	Ranks    []string
	WinRates []string
}

func buildLiveTeamFields(blueTeam, redTeam liveTeamRows, showBlueTeam, showRedTeam bool) (fields []*discordgo.MessageEmbedField) {
	if showBlueTeam {
		fields = append(fields,
			&discordgo.MessageEmbedField{Name: "🔵 Team", Value: joinOrDash(blueTeam.Names), Inline: true},
			&discordgo.MessageEmbedField{Name: "Rank", Value: joinOrDash(blueTeam.Ranks), Inline: true},
			&discordgo.MessageEmbedField{Name: "Win Rate", Value: joinOrDash(blueTeam.WinRates), Inline: true},
		)
	}
	if showRedTeam {
		fields = append(fields,
			&discordgo.MessageEmbedField{Name: "🔴 Team", Value: joinOrDash(redTeam.Names), Inline: true},
			&discordgo.MessageEmbedField{Name: "Rank", Value: joinOrDash(redTeam.Ranks), Inline: true},
			&discordgo.MessageEmbedField{Name: "Win Rate", Value: joinOrDash(redTeam.WinRates), Inline: true},
		)
	}
	return fields
}

func liveTeamsToRender(players []riot.LiveGamePlayer, blueTeam, redTeam liveTeamRows) (showBlueTeam bool, showRedTeam bool) {
	for _, p := range players {
		switch p.TeamID {
		case 100:
			showBlueTeam = true
		case 200:
			showRedTeam = true
		}
	}
	if !showBlueTeam && !showRedTeam {
		showBlueTeam = len(blueTeam.Names) > 0 || len(blueTeam.Ranks) > 0
		showRedTeam = len(redTeam.Names) > 0 || len(redTeam.Ranks) > 0
		if !showBlueTeam && !showRedTeam {
			return true, true
		}
	}
	return showBlueTeam, showRedTeam
}

func buildLiveTeamRows(
	players []riot.LiveGamePlayer,
	champions map[int]postgres.ChampionDisplay,
	solo map[string]*riot.LeagueEntry,
	rankIcons map[string]string,
) (blue, red liveTeamRows) {
	for _, player := range players {
		rowName := playerNameLine(player, champions)
		rowRank, rowWin := playerRankWinLines(player, solo, rankIcons)
		if player.TeamID == 200 {
			red.Names = append(red.Names, rowName)
			red.Ranks = append(red.Ranks, rowRank)
			red.WinRates = append(red.WinRates, rowWin)
			continue
		}
		blue.Names = append(blue.Names, rowName)
		blue.Ranks = append(blue.Ranks, rowRank)
		blue.WinRates = append(blue.WinRates, rowWin)
	}
	return blue, red
}

const maxRiotIDDisplayWidth = 24

func playerNameLine(player riot.LiveGamePlayer, champions map[int]postgres.ChampionDisplay) string {
	champion := champions[player.ChampionID]
	displayName := strings.TrimSpace(player.RiotID)

	if strings.TrimSpace(player.PUUID) == "" || displayName == "" {
		displayName = strings.TrimSpace(champion.Name)
		if displayName == "" {
			displayName = fmt.Sprintf("Champion %d", player.ChampionID)
		}
	} else {
		displayName = truncateRiotIDByWidth(displayName, maxRiotIDDisplayWidth)
	}

	if icon := strings.TrimSpace(champion.DiscordIcon); icon != "" {
		return fmt.Sprintf("%s %s", icon, displayName)
	}
	return displayName
}

func truncateRiotIDByWidth(riotID string, maxWidth int) string {
	if maxWidth <= 0 {
		return riotID
	}
	runes := []rune(riotID)
	width := 0
	for i, r := range runes {
		rw := runeDisplayWidth(r)
		if width+rw > maxWidth {
			return string(runes[:i]) + "..."
		}
		width += rw
	}
	return riotID
}

func runeDisplayWidth(r rune) int {
	if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
		return 2
	}
	if r >= 0xFF00 && r <= 0xFFEF {
		return 2
	}
	return 1
}

func playerRankWinLines(player riot.LiveGamePlayer, solo map[string]*riot.LeagueEntry, rankIcons map[string]string) (rank string, win string) {
	entry, ok := solo[strings.TrimSpace(player.PUUID)]
	if !ok {
		entry = nil
	}
	rank = riot.RankedLineWithIcon(entry, rankIcons, "Unranked")
	win = riot.RecordLineOr(entry, "W", "L", "-")
	return rank, win
}

func joinOrDash(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, "\n")
}

func collectLiveChampionIDs(players []riot.LiveGamePlayer, bans []riot.LiveGameBan) []int {
	seen := make(map[int]struct{})
	var out []int
	for _, p := range players {
		out = appendUniquePositiveID(out, seen, p.ChampionID)
	}
	for _, b := range bans {
		out = appendUniquePositiveID(out, seen, b.ChampionID)
	}
	return out
}

func bansLineLive(bans []riot.LiveGameBan, teamID int, champions map[int]postgres.ChampionDisplay) string {
	seen := make(map[int]struct{}, len(bans))
	championIDs := make([]int, 0, len(bans))
	for _, ban := range bans {
		if ban.TeamID != teamID || ban.ChampionID <= 0 {
			continue
		}
		championIDs = appendUniquePositiveID(championIDs, seen, ban.ChampionID)
	}
	return banIconsLine(championIDs, champions)
}
