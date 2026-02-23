package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/sync/errgroup"
)

const (
	searchTimeout    = 15 * time.Second
	searchEmbedColor = 0x2b2b2b
	rankedSoloQueue  = "RANKED_SOLO_5x5"
	rankedFlexQueue  = "RANKED_FLEX_SR"
)

// -- Command Definition --
var SearchCommand = &discord.Command{
	Data: &discordgo.ApplicationCommand{
		Name:        "search",
		Description: "View information about an account.",
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
			discordgo.ApplicationIntegrationUserInstall,
		},
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		Options: discord.AccountTargetOptions(),
	},
	Handler: handleSearch,
}

func handleSearch(s *discordgo.Session, i *discordgo.InteractionCreate) {
	runtime := currentRuntime()
	if !discord.RequireCommandConfigured(s, i, strings.TrimSpace(runtime.RiotAPIKey) != "") {
		return
	}

	options := i.ApplicationCommandData().Options
	region, nick, tag, validationErr := discord.ParseAccountTargetOptions(i, options)
	if validationErr != "" {
		discord.RespondWithError(s, i, validationErr)
		return
	}

	if err := discord.RunDeferredEmbedCommand(s, i, searchTimeout, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		account, summoner, entries, err := loadSearchData(ctx, runtime.RiotAPIKey, region, nick, tag)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch account: %w", err)
		}

		rankIcons := map[string]string{}
		if runtime.Database != nil {
			icons, err := runtime.Database.RankIconsByTiers(ctx, riot.RankTiersToLookup(entries))
			if err != nil {
				slog.Warn("Failed to load ranked tier icons", "error", err)
			} else {
				rankIcons = icons
			}
		}
		return []*discordgo.MessageEmbed{buildSearchEmbed(account, summoner, entries, rankIcons)}, nil
	}, func(err error) string {
		return mapSearchDeferredError(i, err, nick, tag)
	}); err != nil {
		slog.Error("Failed to handle deferred search interaction", "error", err)
	}
}

func mapSearchDeferredError(i *discordgo.InteractionCreate, err error, nick, tag string) string {
	if msg, ok := discord.MapAccountNotFoundHint(i, err, nick, tag); ok {
		return msg
	}
	return "Could not connect to Riot servers.\nPlease try again later."
}

func loadSearchData(ctx context.Context, riotAPIKey, region, nick, tag string) (riot.RiotAccount, riot.SummonerProfile, []riot.LeagueEntry, error) {
	platformRegion := riot.NormalizePlatformRegion(region)
	if platformRegion == "" {
		return riot.RiotAccount{}, riot.SummonerProfile{}, nil, fmt.Errorf("region is invalid")
	}

	account, err := riot.FetchAccountByRiotID(ctx, platformRegion, nick, tag, riotAPIKey)
	if err != nil {
		return riot.RiotAccount{}, riot.SummonerProfile{}, nil, err
	}

	var (
		summoner riot.SummonerProfile
		entries  []riot.LeagueEntry
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		summoner, err = riot.FetchSummonerByPUUID(gctx, platformRegion, account.PUUID, riotAPIKey)
		return err
	})
	g.Go(func() error {
		var err error
		entries, err = riot.FetchLeagueEntriesByPUUID(gctx, platformRegion, account.PUUID, riotAPIKey)
		return err
	})
	if err := g.Wait(); err != nil {
		return riot.RiotAccount{}, riot.SummonerProfile{}, nil, err
	}
	return account, summoner, entries, nil
}

func buildSearchEmbed(account riot.RiotAccount, summoner riot.SummonerProfile, entries []riot.LeagueEntry, rankIcons map[string]string) *discordgo.MessageEmbed {
	solo := riot.QueueEntry(entries, rankedSoloQueue)
	flex := riot.QueueEntry(entries, rankedFlexQueue)
	lastSeen := "Unknown"
	if summoner.RevisionDate > 0 {
		lastSeen = fmt.Sprintf("<t:%d:R>", summoner.RevisionDate/1000)
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "About Account",
			IconURL: cdn.ProfileIconURL(29),
		},
		Title:       fmt.Sprintf("%s#%s", strings.TrimSpace(account.GameName), strings.TrimSpace(account.TagLine)),
		Description: fmt.Sprintf("**Last seen**: %s **Level**: %d", lastSeen, summoner.SummonerLevel),
		Color:       searchEmbedColor,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: cdn.ProfileIconURL(summoner.ProfileIconID),
		},
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Solo", Value: rankedFieldValue(solo, rankIcons), Inline: true},
			{Name: "Flex", Value: rankedFieldValue(flex, rankIcons), Inline: true},
		},
	}
	discord.ApplyDefaultFooter(embed)
	return embed
}

func rankedFieldValue(entry *riot.LeagueEntry, rankIcons map[string]string) string {
	if entry == nil {
		return riot.RankedLineWithIcon(nil, rankIcons, "Unranked")
	}
	rankLine := riot.RankedLineWithIcon(entry, rankIcons, "Unranked")
	recordLine := riot.RecordLineOr(entry, "W", "L", "-")
	return fmt.Sprintf("%s\n%s", rankLine, recordLine)
}
