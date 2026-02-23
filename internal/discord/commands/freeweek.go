package commands

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
)

const (
	freeWeekTimeout       = 15 * time.Second
	freeWeekEmbedColor    = 0x54fafa
	freeWeekIconID        = 4520
	defaultPlatformRegion = "br1"
)

// -- Command Definition --
var FreeWeekCommand = &discord.Command{
	Data: &discordgo.ApplicationCommand{
		Name:        "free",
		Description: "View information about free champion rotation.",
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
			discordgo.ApplicationIntegrationUserInstall,
		},
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
			discordgo.InteractionContextBotDM,
			discordgo.InteractionContextPrivateChannel,
		},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "week",
				Description: "View information about the free champion rotation",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	},
	Handler: handleFreeWeek,
}

type Runtime struct {
	RiotAPIKey     string
	PlatformRegion string
	Database       storage.CommandDB
}

var (
	runtimeMu sync.RWMutex
	runtime   Runtime
)

func ConfigureRuntime(cfg Runtime) {
	runtimeMu.Lock()
	runtime = Runtime{
		RiotAPIKey:     strings.TrimSpace(cfg.RiotAPIKey),
		PlatformRegion: runtimePlatformRegion(cfg.PlatformRegion),
		Database:       cfg.Database,
	}
	runtimeMu.Unlock()
}

func currentRuntime() Runtime {
	runtimeMu.RLock()
	defer runtimeMu.RUnlock()
	return runtime
}

func runtimePlatformRegion(region string) string {
	if normalized := riot.NormalizePlatformRegion(region); normalized != "" {
		return normalized
	}
	return defaultPlatformRegion
}

func handleFreeWeek(s *discordgo.Session, i *discordgo.InteractionCreate) {
	rt := currentRuntime()
	if !discord.RequireCommandConfigured(s, i, rt.Database != nil && strings.TrimSpace(rt.RiotAPIKey) != "" && strings.TrimSpace(rt.PlatformRegion) != "") {
		return
	}

	if err := discord.RunDeferredEmbedCommand(s, i, freeWeekTimeout, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		rotation, err := loadFreeWeekRotation(ctx, rt)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch free rotation: %w", err)
		}

		allIDs := mergeChampionIDs(rotation.FreeChampionIDs, rotation.FreeChampionIDsForNewPlayers)
		champs, err := rt.Database.ChampionDisplayByIDs(ctx, allIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to load champion names: %w", err)
		}

		embed := buildFreeWeekEmbed(rotation, champs)
		discord.ApplyDefaultFooter(embed)
		return []*discordgo.MessageEmbed{embed}, nil
	}, nil); err != nil {
		slog.Error("Failed to handle deferred freeweek interaction", "error", err)
	}
}

func loadFreeWeekRotation(ctx context.Context, rt Runtime) (riot.ChampionRotation, error) {
	now := time.Now().UTC()

	cached, fetchedAt, expiresAt, found, err := rt.Database.GetFreeWeekRotation(ctx, rt.PlatformRegion)
	if err != nil {
		slog.Warn("Cache fetch failed", "region", rt.PlatformRegion, "error", err)
	}

	if found && expiresAt.After(now) {
		return cached, nil
	}

	rotation, err := riot.FetchChampionRotation(ctx, rt.PlatformRegion, rt.RiotAPIKey)
	if err != nil {
		if found {
			slog.Warn("Refresh failed, serving stale cache", "region", rt.PlatformRegion, "age", now.Sub(fetchedAt), "error", err)
			return cached, nil
		}
		return riot.ChampionRotation{}, err
	}

	if err := rt.Database.UpsertFreeWeekRotation(ctx, rt.PlatformRegion, rotation, now, nextRotationExpiryUTC(now)); err != nil {
		slog.Warn("Cache update failed", "region", rt.PlatformRegion, "error", err)
	}
	return rotation, nil
}

// Riot updates the free week rotation every Wednesday at 12:00 UTC.
func nextRotationExpiryUTC(now time.Time) time.Time {
	t := now.UTC()
	daysUntilWed := (int(time.Wednesday) - int(t.Weekday()) + 7) % 7
	if daysUntilWed == 0 && t.Hour() >= 12 {
		daysUntilWed = 7
	}
	d := t.AddDate(0, 0, daysUntilWed)
	return time.Date(d.Year(), d.Month(), d.Day(), 12, 0, 0, 0, time.UTC)
}

func buildFreeWeekEmbed(rotation riot.ChampionRotation, displays map[int]postgres.ChampionDisplay) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Free Champions of the Week",
			IconURL: cdn.ProfileIconURL(freeWeekIconID),
		},
		Description: "Champions that can be played without having\nto purchase them with RP or essence.",
		Color:       freeWeekEmbedColor,
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Champions",
				Value:  strings.Join(championLines(rotation.FreeChampionIDs, displays), "\n"),
				Inline: true,
			},
			{
				Name:   fmt.Sprintf("For New Players (<= %d)", rotation.MaxNewPlayerLevel),
				Value:  strings.Join(championLines(rotation.FreeChampionIDsForNewPlayers, displays), "\n"),
				Inline: true,
			},
		},
	}
}

func championLines(ids []int, displays map[int]postgres.ChampionDisplay) []string {
	if len(ids) == 0 {
		return []string{"No champions available at the moment."}
	}

	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		entry, ok := displays[id]
		name := strings.TrimSpace(entry.Name)
		if !ok || name == "" {
			lines = append(lines, fmt.Sprintf("ID %d", id))
			continue
		}
		if icon := strings.TrimSpace(entry.DiscordIcon); icon != "" {
			lines = append(lines, fmt.Sprintf("%s %s", icon, name))
		} else {
			lines = append(lines, name)
		}
	}
	return lines
}

func mergeChampionIDs(sets ...[]int) []int {
	seen := make(map[int]struct{})
	out := make([]int, 0)
	for _, set := range sets {
		for _, id := range set {
			if _, ok := seen[id]; !ok {
				seen[id] = struct{}{}
				out = append(out, id)
			}
		}
	}
	return out
}
