package commands

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bwmarrin/discordgo"
)

func TestRankedFieldValue(t *testing.T) {
	ranked := rankedFieldValue(&riot.LeagueEntry{
		QueueType:    rankedSoloQueue,
		Tier:         "DIAMOND",
		Rank:         "I",
		LeaguePoints: 90,
		Wins:         10,
		Losses:       10,
	}, nil)
	if ranked != "Diamond I 90LP\n50% 10W 10L" {
		t.Fatalf("rankedFieldValue() = %q", ranked)
	}

	unranked := rankedFieldValue(nil, nil)
	if unranked != "Unranked" {
		t.Fatalf("rankedFieldValue(nil) = %q", unranked)
	}
}

func TestRankedFieldValue_NoDivisionForApexTiers(t *testing.T) {
	value := rankedFieldValue(&riot.LeagueEntry{
		Tier:         "MASTER",
		Rank:         "I",
		LeaguePoints: 580,
		Wins:         10,
		Losses:       10,
	}, map[string]string{"master": "<:Master:1>"})
	if !strings.HasPrefix(value, "<:Master:1> Master 580LP\n") {
		t.Fatalf("rankedFieldValue(master) = %q", value)
	}
}

func TestRankedFieldValue_UsesUnrankedIcon(t *testing.T) {
	value := rankedFieldValue(nil, map[string]string{"unranked": "<:Unranked:1>"})
	if value != "<:Unranked:1> Unranked" {
		t.Fatalf("rankedFieldValue(nil) = %q", value)
	}
}

func TestQueueEntry(t *testing.T) {
	entries := []riot.LeagueEntry{
		{QueueType: rankedFlexQueue, Tier: "MASTER"},
		{QueueType: rankedSoloQueue, Tier: "CHALLENGER"},
	}

	solo := riot.QueueEntry(entries, rankedSoloQueue)
	if solo == nil || solo.Tier != "CHALLENGER" {
		t.Fatalf("queueEntry solo = %#v", solo)
	}

	missing := riot.QueueEntry(entries, "CHERRY")
	if missing != nil {
		t.Fatalf("queueEntry missing = %#v, want nil", missing)
	}
}

func TestBuildSearchEmbed(t *testing.T) {
	embed := buildSearchEmbed(
		riot.RiotAccount{GameName: "Bekko", TagLine: "Ekko"},
		riot.SummonerProfile{ProfileIconID: 7070, RevisionDate: 1770422047398, SummonerLevel: 1027},
		[]riot.LeagueEntry{
			{QueueType: rankedSoloQueue, Tier: "MASTER", Rank: "I", LeaguePoints: 580, Wins: 56, Losses: 43},
			{QueueType: rankedFlexQueue, Tier: "CHALLENGER", Rank: "I", LeaguePoints: 1389, Wins: 67, Losses: 22},
		},
		map[string]string{
			"master":     "<:Master:1>",
			"challenger": "<:Challenger:1>",
			"unranked":   "<:Unranked:1>",
		},
	)

	if embed.Title != "Bekko#Ekko" {
		t.Fatalf("embed.Title = %q", embed.Title)
	}
	if !strings.Contains(embed.Description, "**Last seen**: <t:1770422047:R>") {
		t.Fatalf("embed.Description = %q", embed.Description)
	}
	if embed.Author == nil || embed.Author.Name != "About Account" {
		t.Fatalf("embed.Author = %#v", embed.Author)
	}
	if embed.Footer == nil || embed.Footer.Text != "League API bot" {
		t.Fatalf("embed.Footer = %#v", embed.Footer)
	}
	if !strings.Contains(embed.Fields[0].Value, "<:Master:1> Master 580LP") {
		t.Fatalf("embed.Fields[0].Value = %q", embed.Fields[0].Value)
	}
	if strings.Contains(embed.Fields[0].Value, "Master I 580LP") {
		t.Fatalf("embed.Fields[0].Value should not contain division for master: %q", embed.Fields[0].Value)
	}
	if embed.Timestamp == "" {
		t.Fatalf("embed.Timestamp should not be empty")
	}
}

func TestBuildSearchEmbed_UsesEnglishText(t *testing.T) {
	embed := buildSearchEmbed(
		riot.RiotAccount{GameName: "Bekko", TagLine: "Ekko"},
		riot.SummonerProfile{ProfileIconID: 7070, RevisionDate: 0, SummonerLevel: 1},
		nil,
		nil,
	)

	if embed.Author == nil || embed.Author.Name != "About Account" {
		t.Fatalf("embed.Author = %#v", embed.Author)
	}
}

func TestParseAccountTargetOptions_MissingRequiredOption(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	_, _, _, errMsg := discord.ParseAccountTargetOptions(i, []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "nick", Value: "ValidNick#BR1"},
	})
	if errMsg != "The region is required." {
		t.Fatalf("unexpected message: %q", errMsg)
	}
}

func TestParseAccountTargetOptions_Valid(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	region, nick, tag, errMsg := discord.ParseAccountTargetOptions(i, []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "region", Value: "br1"},
		{Name: "nick", Value: "ValidNick#BR1"},
	})
	if errMsg != "" {
		t.Fatalf("unexpected validation error: %q", errMsg)
	}
	if region != "br1" || nick != "ValidNick" || tag != "BR1" {
		t.Fatalf("unexpected values: region=%q nick=%q tag=%q", region, nick, tag)
	}
}

func TestParseAccountTargetOptions_InvalidNickLength(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	_, _, _, errMsg := discord.ParseAccountTargetOptions(i, []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "region", Value: "br1"},
		{Name: "nick", Value: "ab#BR1"},
	})
	if errMsg != "The nick must be between 7 and 22 characters" {
		t.Fatalf("unexpected message: %q", errMsg)
	}
}

func TestParseAccountTargetOptions_InvalidNickFormat(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	_, _, _, errMsg := discord.ParseAccountTargetOptions(i, []*discordgo.ApplicationCommandInteractionDataOption{
		{Name: "region", Value: "br1"},
		{Name: "nick", Value: "validnick123"},
	})
	if errMsg != "The nick is invalid. Use format nickname#tagline." {
		t.Fatalf("unexpected message: %q", errMsg)
	}
}

func TestMapSearchDeferredError_AccountNotFound(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	err := fmt.Errorf("failed to fetch account: %w", fmt.Errorf("fetch account by riot id: %w", &riot.HTTPStatusError{
		URL:        "https://americas.api.riotgames.com/riot/account/v1/accounts/by-riot-id/ewaske/br11",
		StatusCode: http.StatusNotFound,
		Body:       `{"status":{"status_code":404}}`,
	}))
	got := mapSearchDeferredError(i, err, "ewaske", "br11")
	want := "Your account nick is invalid or you entered it incorrectly.\nNo results found for player `ewaske#br11`."
	if got != want {
		t.Fatalf("mapSearchDeferredError() = %q, want %q", got, want)
	}
}

func TestMapSearchDeferredError_Fallback(t *testing.T) {
	i := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{Locale: discordgo.Locale("en-US")},
	}
	got := mapSearchDeferredError(i, fmt.Errorf("boom"), "ewaske", "br11")
	if got != "Could not connect to Riot servers.\nPlease try again later." {
		t.Fatalf("unexpected fallback message: %q", got)
	}
}
