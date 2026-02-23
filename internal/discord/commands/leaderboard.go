package commands

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/sync/errgroup"
)

const (
	leadboardTimeout          = 20 * time.Second
	leadboardEmbedColor       = 0xF4B38B
	leadboardTrackedLimit     = 25
	leadboardFetchLimit       = 8
	leadboardMMRUnranked      = -1
	leadboardDescription      = "List of the best solo/duo players on this Discord server."
	leadboardRankedMMRBaseTop = 2800
	leadboardFieldValueLimit  = 1024
)

// -- Command Definition --
var LeadboardCommand = &discord.Command{
	Data: &discordgo.ApplicationCommand{
		Name:        "leaderboard",
		Description: "Show tracked players ranked by solo/duo MMR.",
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
		},
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
		},
	},
	Handler: handleLeadboard,
}

type leadboardRow struct {
	account postgres.TrackedAccount
	solo    *riot.LeagueEntry
	mmr     int
}

func handleLeadboard(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rt := currentRuntime()
	if !discord.RequireCommandConfigured(s, i, rt.Database != nil && strings.TrimSpace(rt.RiotAPIKey) != "") {
		return
	}

	guildID, ok := discord.RequireGuildCommand(s, i)
	if !ok {
		return
	}
	if !requireTrackConfig(s, i, rt.Database, guildID) {
		return
	}

	if err := discord.RunDeferredEmbedCommand(s, i, leadboardTimeout, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		rows, err := loadLeadboardRows(ctx, rt, guildID)
		if err != nil {
			return nil, fmt.Errorf("load leaderboard rows: %w", err)
		}
		if len(rows) == 0 {
			return []*discordgo.MessageEmbed{leadboardInfoEmbed("No tracked accounts found for this server.\nUse `/track add` first.")}, nil
		}

		sortLeadboardRows(rows)
		rankIcons := loadLeadboardRankIcons(ctx, rt.Database, rows)
		return buildLeadboardEmbeds(rows, rankIcons), nil
	}, func(error) string {
		return "Could not load leaderboard right now. Please try again."
	}); err != nil {
		slog.Error("Failed to run deferred /leadboard", "error", err)
	}
}

func loadLeadboardRows(ctx context.Context, rt Runtime, guildID string) ([]leadboardRow, error) {
	tracked, err := rt.Database.ListTrackedAccounts(ctx, guildID, leadboardTrackedLimit)
	if err != nil {
		return nil, fmt.Errorf("list tracked accounts: %w", err)
	}
	if len(tracked) == 0 {
		return []leadboardRow{}, nil
	}

	rows := make([]leadboardRow, len(tracked))
	for i, account := range tracked {
		rows[i] = leadboardRow{account: account, mmr: leadboardMMRUnranked}
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(leadboardFetchLimit)
	for idx := range rows {
		g.Go(func() error {
			region := riot.NormalizePlatformRegion(rows[idx].account.PlatformRegion)
			if region == "" {
				return nil
			}
			puuid := strings.TrimSpace(rows[idx].account.PUUID)
			if puuid == "" {
				return nil
			}

			entries, err := riot.FetchLeagueEntriesByPUUID(gctx, region, puuid, rt.RiotAPIKey)
			if err != nil {
				slog.Warn("Failed to fetch solo queue entry for /leadboard", "guildID", guildID, "riotID", rows[idx].account.RiotID(), "error", err)
				return nil
			}
			solo := riot.QueueEntry(entries, rankedSoloQueue)
			if solo == nil {
				return nil
			}

			entryCopy := *solo
			rows[idx].solo = &entryCopy
			rows[idx].mmr = soloQueueMMR(entryCopy)
			return nil
		})
	}
	_ = g.Wait()

	return rows, nil
}

func loadLeadboardRankIcons(ctx context.Context, db storage.SearchDB, rows []leadboardRow) map[string]string {
	entries := make([]riot.LeagueEntry, 0, len(rows))
	for _, row := range rows {
		if row.solo != nil {
			entries = append(entries, *row.solo)
		}
	}

	rankIcons, err := db.RankIconsByTiers(ctx, riot.RankTiersToLookup(entries))
	if err != nil {
		slog.Warn("Failed to load ranked tier icons for /leadboard", "error", err)
		return map[string]string{}
	}
	return rankIcons
}

func sortLeadboardRows(rows []leadboardRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].mmr != rows[j].mmr {
			return rows[i].mmr > rows[j].mmr
		}
		return strings.ToLower(rows[i].account.RiotID()) < strings.ToLower(rows[j].account.RiotID())
	})
}

func buildLeadboardEmbeds(rows []leadboardRow, rankIcons map[string]string) []*discordgo.MessageEmbed {
	nickLines := make([]string, 0, len(rows))
	rankLines := make([]string, 0, len(rows))
	winRateLines := make([]string, 0, len(rows))
	for _, row := range rows {
		nickLines = append(nickLines, row.account.RiotID())
		rankLines = append(rankLines, riot.RankedLineWithIcon(row.solo, rankIcons, "Unranked"))
		winRateLines = append(winRateLines, riot.RecordLineOr(row.solo, "W", "L", "-"))
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Leaderboard",
			IconURL: cdn.ProfileIconURL(5496),
		},
		Description: leadboardDescription,
		Color:       leadboardEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Nick", Value: leadboardFieldValue(nickLines), Inline: true},
			{Name: "Rank", Value: leadboardFieldValue(rankLines), Inline: true},
			{Name: "Win Rate", Value: leadboardFieldValue(winRateLines), Inline: true},
		},
	}
	discord.ApplyDefaultFooter(embed)
	return []*discordgo.MessageEmbed{embed}
}

func leadboardInfoEmbed(message string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Leaderboard",
			IconURL: cdn.ProfileIconURL(5496),
		},
		Description: strings.TrimSpace(message),
		Color:       leadboardEmbedColor,
	}
	discord.ApplyDefaultFooter(embed)
	return embed
}

func soloQueueMMR(entry riot.LeagueEntry) int {
	base := leadboardTierBaseMMR(entry.Tier)
	if base < 0 {
		return leadboardMMRUnranked
	}
	if riot.HideDivisionForTier(entry.Tier) {
		return base + entry.LeaguePoints
	}
	return base + leadboardSubTierMMR(entry.Rank) + entry.LeaguePoints
}

func leadboardTierBaseMMR(tier string) int {
	switch strings.ToUpper(strings.TrimSpace(tier)) {
	case "IRON":
		return 0
	case "BRONZE":
		return 400
	case "SILVER":
		return 800
	case "GOLD":
		return 1200
	case "PLATINUM":
		return 1600
	case "EMERALD":
		return 2000
	case "DIAMOND":
		return 2400
	case "MASTER", "GRANDMASTER", "CHALLENGER":
		return leadboardRankedMMRBaseTop
	default:
		return -1
	}
}

func leadboardSubTierMMR(subTier string) int {
	switch strings.ToUpper(strings.TrimSpace(subTier)) {
	case "IV":
		return 0
	case "III":
		return 100
	case "II":
		return 200
	case "I":
		return 300
	default:
		return 0
	}
}

func leadboardFieldValue(lines []string) string {
	if len(lines) == 0 {
		return "-"
	}
	value := strings.Join(lines, "\n")
	runes := []rune(value)
	if len(runes) <= leadboardFieldValueLimit {
		return value
	}
	return string(runes[:leadboardFieldValueLimit-3]) + "..."
}
