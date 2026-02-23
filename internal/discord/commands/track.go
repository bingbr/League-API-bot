package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"

	"github.com/bwmarrin/discordgo"
)

const (
	trackDbTimeout         = 5 * time.Second
	trackAddTimeout        = 15 * time.Second
	trackEmbedInfoColor    = 0x57F287
	trackAutocompleteLimit = 25
	trackRequiredPerms     = discordgo.PermissionViewChannel | discordgo.PermissionSendMessages | discordgo.PermissionEmbedLinks
)

// -- Command Definition --
var TrackCommand = &discord.Command{
	Data: &discordgo.ApplicationCommand{
		Name:        "track",
		Description: "Track a player.",
		IntegrationTypes: &[]discordgo.ApplicationIntegrationType{
			discordgo.ApplicationIntegrationGuildInstall,
		},
		Contexts: &[]discordgo.InteractionContextType{
			discordgo.InteractionContextGuild,
		},
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Add an account to track.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     discord.AccountTargetOptions(),
			},
			{
				Name:        "remove",
				Description: "Stop tracking an account.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:         discordgo.ApplicationCommandOptionString,
						Name:         "account",
						Description:  "Select account in format nickname#tagline.",
						MinLength:    new(7),
						MaxLength:    22,
						Autocomplete: true,
						Required:     true,
					},
				},
			},
			{
				Name:        "config",
				Description: "Define where you want tracked information to be posted.",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options: []*discordgo.ApplicationCommandOption{
					{
						Name:         "channel",
						Description:  "Select the #channel that will be in use by the bot.",
						Type:         discordgo.ApplicationCommandOptionChannel,
						Autocomplete: false,
						Required:     true,
						ChannelTypes: []discordgo.ChannelType{
							discordgo.ChannelTypeGuildText,
						},
					},
				},
			},
		},
	},
	Handler: track,
}

func track(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommandAutocomplete {
		handleTrackAutocomplete(s, i)
		return
	}
	guildID, ok := discord.RequireGuildCommand(s, i)
	if !ok {
		return
	}
	rt := currentRuntime()
	if !discord.RequireCommandConfigured(s, i, rt.Database != nil) {
		return
	}
	subcommand, options, ok := trackSubcommand(i)
	if !ok {
		discord.RespondWithError(s, i, "Invalid command input.")
		return
	}

	switch subcommand {
	case "config":
		handleTrackConfig(s, i, rt.Database, guildID, options)
	case "add":
		handleTrackAdd(s, i, rt, guildID, options)
	case "remove":
		handleTrackRemove(s, i, rt.Database, guildID, options)
	default:
		discord.RespondWithError(s, i, "Invalid command input.")
	}
}

func handleTrackConfig(s *discordgo.Session, i *discordgo.InteractionCreate, db storage.TrackDB, guildID string, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if !hasManageServerPermission(i) {
		discord.RespondWithError(s, i, "You need the `Manage Server` permission to use `/track config`.")
		return
	}
	channelID := discord.OptionValueByName(options, "channel")
	if channelID == "" {
		discord.RespondWithError(s, i, "The channel is required.")
		return
	}
	if err := ensureTrackConfigChannelPerms(s, channelID); err != nil {
		slog.Warn("Rejected /track config: bot cannot post in channel", "guildID", guildID, "channelID", channelID, "error", err)
		discord.RespondWithError(s, i, "I can't post tracking updates in this channel.\nGrant `View Channel`, `Send Messages`, and `Embed Links`, then run `/track config` again.")
		return
	}

	if err := withTrackDBTimeout(func(ctx context.Context) error {
		return db.UpsertTrackGuildConfig(ctx, guildID, channelID)
	}); err != nil {
		slog.Error("Failed to save /track config", "guildID", guildID, "channelID", channelID, "error", err)
		discord.RespondWithError(s, i, "Could not save tracking configuration. Please try again.")
		return
	}

	if err := discord.RespondWithEmbed(s, i, trackInfoEmbed(fmt.Sprintf("Account tracking updates will be sent to <#%s>.", channelID))); err != nil {
		slog.Error("Failed to respond /track config", "error", err)
	}
}

func handleTrackAdd(s *discordgo.Session, i *discordgo.InteractionCreate, rt Runtime, guildID string, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if rt.RiotAPIKey == "" {
		discord.RespondWithError(s, i, "The command is not configured.")
		return
	}
	if !requireTrackConfig(s, i, rt.Database, guildID) {
		return
	}

	region, nickname, tagline, validationErr := discord.ParseAccountTargetOptions(i, options)
	if validationErr != "" {
		discord.RespondWithError(s, i, validationErr)
		return
	}

	_, userID := discord.InteractionUserID(i)
	if err := discord.RunDeferredEmbedCommand(s, i, trackAddTimeout, func(ctx context.Context) ([]*discordgo.MessageEmbed, error) {
		account, err := riot.FetchAccountByRiotID(ctx, region, nickname, tagline, rt.RiotAPIKey)
		if err != nil {
			return nil, fmt.Errorf("fetch track account by riot id: %w", err)
		}

		created, err := rt.Database.AddTrackedAccount(ctx, postgres.TrackedAccount{
			GuildID:        guildID,
			PlatformRegion: region,
			PUUID:          account.PUUID,
			NickName:       account.GameName,
			TagLine:        account.TagLine,
			AddedBy:        userID,
		})
		if err != nil {
			return nil, fmt.Errorf("add tracked account: %w", err)
		}

		accountLabel := riot.FormatRiotID(account.GameName, account.TagLine)
		message := fmt.Sprintf("Tracking `%s` on this server.", accountLabel)
		if !created {
			message = fmt.Sprintf("`%s` is already being tracked on this server.", accountLabel)
		}

		return []*discordgo.MessageEmbed{trackInfoEmbed(message)}, nil
	}, func(err error) string {
		return mapTrackAddDeferredError(i, err, nickname, tagline)
	}); err != nil {
		slog.Error("Failed to run deferred /track add", "error", err)
	}
}

func handleTrackRemove(s *discordgo.Session, i *discordgo.InteractionCreate, db storage.TrackDB, guildID string, options []*discordgo.ApplicationCommandInteractionDataOption) {
	if !requireTrackConfig(s, i, db, guildID) {
		return
	}

	account := discord.OptionValueByName(options, "account")
	gameName, tagLine, err := riot.SplitRiotID(account)
	if err != nil {
		discord.RespondWithError(s, i, "Select an account from the list.")
		return
	}
	accountLabel := riot.FormatRiotID(gameName, tagLine)

	removed, err := withTrackDBTimeoutValue(func(ctx context.Context) (bool, error) {
		return db.RemoveTrackedAccount(ctx, guildID, accountLabel)
	})
	if err != nil {
		slog.Error("Failed to remove tracked account", "guildID", guildID, "account", accountLabel, "error", err)
		discord.RespondWithError(s, i, "Could not remove this account from tracking right now. Please try again.")
		return
	}

	message := fmt.Sprintf("Stopped tracking `%s` on this server.", accountLabel)
	if !removed {
		message = fmt.Sprintf("No tracked account found for `%s` on this server.", accountLabel)
	}
	if err := discord.RespondWithEmbed(s, i, trackInfoEmbed(message)); err != nil {
		slog.Error("Failed to respond /track remove", "error", err)
	}
}

func handleTrackAutocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	subcommand, options, ok := trackSubcommand(i)
	if !ok || subcommand != "remove" {
		respondTrackAutocompleteChoices(s, i, nil)
		return
	}

	guildID := strings.TrimSpace(i.GuildID)
	rt := currentRuntime()
	if guildID == "" || rt.Database == nil {
		respondTrackAutocompleteChoices(s, i, nil)
		return
	}

	_, configured, err := loadTrackGuildConfig(rt.Database, guildID)
	if err != nil || !configured {
		if err != nil {
			slog.Warn("Failed to load track config for autocomplete", "guildID", guildID, "error", err)
		}
		respondTrackAutocompleteChoices(s, i, nil)
		return
	}

	trackedAccounts, err := withTrackDBTimeoutValue(func(ctx context.Context) ([]postgres.TrackedAccount, error) {
		return rt.Database.ListTrackedAccounts(ctx, guildID, trackAutocompleteLimit)
	})
	if err != nil {
		slog.Warn("Failed to list tracked accounts for autocomplete", "guildID", guildID, "error", err)
		respondTrackAutocompleteChoices(s, i, nil)
		return
	}

	choices := buildTrackAutocompleteChoices(trackedAccounts, discord.FocusedOptionValue(options))
	respondTrackAutocompleteChoices(s, i, choices)
}

func buildTrackAutocompleteChoices(tracked []postgres.TrackedAccount, query string) []*discordgo.ApplicationCommandOptionChoice {
	query = strings.ToLower(strings.TrimSpace(query))
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, trackAutocompleteLimit)
	for _, account := range tracked {
		label := riot.FormatRiotID(account.NickName, account.TagLine)
		if query != "" && !strings.Contains(strings.ToLower(label), query) {
			continue
		}
		labelRunes := []rune(label)
		if len(labelRunes) > 100 {
			label = string(labelRunes[:100])
		}
		choices = append(choices, &discordgo.ApplicationCommandOptionChoice{
			Name:  label,
			Value: label,
		})
		if len(choices) >= trackAutocompleteLimit {
			break
		}
	}
	return choices
}

func respondTrackAutocompleteChoices(s *discordgo.Session, i *discordgo.InteractionCreate, choices []*discordgo.ApplicationCommandOptionChoice) {
	if s == nil || i == nil || i.Interaction == nil {
		return
	}
	if choices == nil {
		choices = []*discordgo.ApplicationCommandOptionChoice{}
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choices,
		},
	}); err != nil {
		slog.Error("Failed to respond /track autocomplete", "error", err)
	}
}

func trackSubcommand(i *discordgo.InteractionCreate) (string, []*discordgo.ApplicationCommandInteractionDataOption, bool) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 || options[0] == nil {
		return "", nil, false
	}
	return options[0].Name, options[0].Options, true
}

func requireTrackConfig(s *discordgo.Session, i *discordgo.InteractionCreate, db storage.TrackDB, guildID string) bool {
	_, configured, err := loadTrackGuildConfig(db, guildID)
	if err != nil {
		slog.Error("Failed to load track config", "guildID", guildID, "error", err)
		discord.RespondWithError(s, i, "Could not read tracking configuration. Please try again.")
		return false
	}
	if !configured {
		discord.RespondWithError(s, i, "The bot is not configured for this server yet.\nUse `/track config` first.")
		return false
	}
	return true
}

func hasManageServerPermission(i *discordgo.InteractionCreate) bool {
	if i == nil || i.Member == nil {
		return false
	}
	perms := i.Member.Permissions
	return perms&discordgo.PermissionManageGuild != 0 || perms&discordgo.PermissionAdministrator != 0
}

func ensureTrackConfigChannelPerms(s *discordgo.Session, channelID string) error {
	if s == nil || s.State == nil || s.State.User == nil {
		return fmt.Errorf("discord session state unavailable")
	}
	botUserID := strings.TrimSpace(s.State.User.ID)
	channelID = strings.TrimSpace(channelID)
	if botUserID == "" || channelID == "" {
		return fmt.Errorf("bot user or channel is empty")
	}

	perms, err := s.UserChannelPermissions(botUserID, channelID)
	if err != nil {
		return fmt.Errorf("read channel permissions: %w", err)
	}
	if perms&trackRequiredPerms != trackRequiredPerms {
		missing := missingTrackChannelPermNames(perms)
		return fmt.Errorf("missing permissions: %s", strings.Join(missing, ", "))
	}
	return nil
}

func missingTrackChannelPermNames(perms int64) []string {
	missing := make([]string, 0, 3)
	if perms&discordgo.PermissionViewChannel == 0 {
		missing = append(missing, "View Channel")
	}
	if perms&discordgo.PermissionSendMessages == 0 {
		missing = append(missing, "Send Messages")
	}
	if perms&discordgo.PermissionEmbedLinks == 0 {
		missing = append(missing, "Embed Links")
	}
	return missing
}

func trackInfoEmbed(message string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Tracking Information",
			IconURL: cdn.ProfileIconURL(5704),
		},
		Description: message,
		Color:       trackEmbedInfoColor,
	}
	discord.ApplyDefaultFooter(embed)
	return embed
}

func mapTrackAddDeferredError(i *discordgo.InteractionCreate, err error, nickname, tagline string) string {
	if msg, ok := discord.MapAccountNotFoundHint(i, err, nickname, tagline); ok {
		return msg
	}
	if _, ok := errors.AsType[*riot.HTTPStatusError](err); ok {
		return "Could not connect to Riot servers.\nPlease try again later."
	}
	return "Could not add this account to tracking right now. Please try again."
}

func withTrackDBTimeout(fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), trackDbTimeout)
	defer cancel()
	return fn(ctx)
}

func withTrackDBTimeoutValue[T any](fn func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), trackDbTimeout)
	defer cancel()
	return fn(ctx)
}

func withTrackDBTimeoutLookup[T any](fn func(ctx context.Context) (T, bool, error)) (T, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), trackDbTimeout)
	defer cancel()
	return fn(ctx)
}

func loadTrackGuildConfig(db storage.TrackDB, guildID string) (postgres.TrackGuildConfig, bool, error) {
	cfg, found, err := withTrackDBTimeoutLookup(func(ctx context.Context) (postgres.TrackGuildConfig, bool, error) {
		cfg, found, err := db.TrackGuildConfig(ctx, guildID)
		return cfg, found, err
	})
	if err != nil || !found || strings.TrimSpace(cfg.ChannelID) == "" {
		return cfg, false, err
	}
	return cfg, true, nil
}
