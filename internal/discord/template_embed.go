package discord

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bwmarrin/discordgo"
)

type DeferredEmbedExecutor func(ctx context.Context) ([]*discordgo.MessageEmbed, error)
type DeferredErrorMapper func(err error) string

const (
	defaultInteractionAckWindow                     = 3 * time.Second
	defaultDeferSafetyMargin                        = 500 * time.Millisecond
	discordErrCodeAlreadyAck, discordErrCodeUnknown = 40060, 10062
)

var (
	interactionAckWindow = defaultInteractionAckWindow
	deferSafetyMargin    = defaultDeferSafetyMargin

	interactionRespond = func(s *discordgo.Session, i *discordgo.Interaction, resp *discordgo.InteractionResponse) error {
		return s.InteractionRespond(i, resp)
	}
	interactionResponseEdit = func(s *discordgo.Session, i *discordgo.Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
		return s.InteractionResponseEdit(i, edit)
	}
	interactionResponseDelete = func(s *discordgo.Session, i *discordgo.Interaction) error {
		return s.InteractionResponseDelete(i)
	}
	followupMessageCreate = func(s *discordgo.Session, i *discordgo.Interaction, wait bool, data *discordgo.WebhookParams) (*discordgo.Message, error) {
		return s.FollowupMessageCreate(i, wait, data)
	}
)

type commandEmbedsResult struct {
	embeds []*discordgo.MessageEmbed
	err    error
}

func RespondWithEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error {
	return interactionRespond(s, i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: []*discordgo.MessageEmbed{embed}},
	})
}

func RespondWithError(s *discordgo.Session, i *discordgo.InteractionCreate, message string) {
	if err := respondWithEmbedsEphemeral(s, i, []*discordgo.MessageEmbed{TemplateError(message, "Error")}); err != nil {
		slog.Error("Failed to respond to interaction", "error", err)
	}
}

func RunDeferredEmbedCommand(s *discordgo.Session, i *discordgo.InteractionCreate, timeout time.Duration, exec DeferredEmbedExecutor, mapErr DeferredErrorMapper) error {
	if s == nil {
		return fmt.Errorf("discord session is required")
	}
	if i == nil || i.Interaction == nil {
		return fmt.Errorf("interaction is required")
	}
	if exec == nil {
		return fmt.Errorf("deferred executor is required")
	}

	ctx := context.Background()
	cancel := func() {}
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	}
	defer cancel()

	resultCh := make(chan commandEmbedsResult, 1)
	go func() {
		embeds, err := exec(ctx)
		resultCh <- commandEmbedsResult{embeds: embeds, err: err}
	}()

	trigger := deferTriggerDelay()
	if trigger <= 0 {
		select {
		case result := <-resultCh:
			return respondCommandResult(s, i, result, mapErr, false)
		default:
		}
	}

	timer := time.NewTimer(trigger)
	defer timer.Stop()
	select {
	case result := <-resultCh:
		return respondCommandResult(s, i, result, mapErr, false)
	case <-timer.C:
		select {
		case result := <-resultCh:
			return respondCommandResult(s, i, result, mapErr, false)
		default:
		}
		if err := DeferInteraction(s, i); err != nil {
			if hasDiscordErrorCode(err, discordErrCodeAlreadyAck) || hasDiscordErrorCode(err, discordErrCodeUnknown) {
				<-resultCh
				return nil
			}
			return err
		}
		result := <-resultCh
		return respondCommandResult(s, i, result, mapErr, true)
	}
}

func DeferInteraction(s *discordgo.Session, i *discordgo.InteractionCreate) error {
	return interactionRespond(s, i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})
}

func EditDeferredEmbeds(s *discordgo.Session, i *discordgo.InteractionCreate, embeds []*discordgo.MessageEmbed) error {
	_, err := interactionResponseEdit(s, i.Interaction, &discordgo.WebhookEdit{Embeds: &embeds})
	return err
}

func respondWithEmbeds(s *discordgo.Session, i *discordgo.InteractionCreate, embeds []*discordgo.MessageEmbed) error {
	return interactionRespond(s, i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Embeds: embeds},
	})
}

func respondWithEmbedsEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, embeds []*discordgo.MessageEmbed) error {
	return interactionRespond(s, i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: embeds,
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func respondDeferredErrorEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, errEmbed *discordgo.MessageEmbed) error {
	_, err := followupMessageCreate(s, i.Interaction, false, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{errEmbed},
		Flags:  discordgo.MessageFlagsEphemeral,
	})
	if err != nil {
		return EditDeferredEmbeds(s, i, []*discordgo.MessageEmbed{errEmbed})
	}
	if err := interactionResponseDelete(s, i.Interaction); err != nil {
		slog.Warn("Failed to delete deferred interaction response after sending ephemeral error", "error", err)
	}
	return nil
}

func respondCommandResult(s *discordgo.Session, i *discordgo.InteractionCreate, result commandEmbedsResult, mapErr DeferredErrorMapper, deferred bool) error {
	if result.err != nil {
		command := i.ApplicationCommandData().Name
		slog.Error("Deferred command execution failed", "command", command, "error", result.err)

		title := "Oops, something went wrong!"
		message := "Could not connect to Riot servers.\nPlease try again later."
		if mapErr != nil {
			message = mapErr(result.err)
		}
		errEmbed := TemplateError(message, title)
		if deferred {
			return respondDeferredErrorEphemeral(s, i, errEmbed)
		}
		return respondWithEmbedsEphemeral(s, i, []*discordgo.MessageEmbed{errEmbed})
	}

	if deferred {
		return EditDeferredEmbeds(s, i, result.embeds)
	}
	return respondWithEmbeds(s, i, result.embeds)
}

func deferTriggerDelay() time.Duration {
	return max(interactionAckWindow-deferSafetyMargin, 0)
}

func hasDiscordErrorCode(err error, code int) bool {
	restErr, ok := errors.AsType[*discordgo.RESTError](err)
	if !ok || restErr == nil || restErr.Message == nil {
		return false
	}
	return restErr.Message.Code == code
}

func ApplyDefaultFooter(embed *discordgo.MessageEmbed) {
	if embed == nil {
		return
	}
	embed.Footer = &discordgo.MessageEmbedFooter{
		Text:    "League API bot",
		IconURL: cdn.ProfileIconURL(5119),
	}
	embed.Timestamp = time.Now().UTC().Format(time.RFC3339)
}

func TemplateError(message, title string) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    title,
			IconURL: cdn.ProfileIconURL(6922),
		},
		Description: message,
		Color:       0xFF0000,
		URL:         "https://github.com/bingbr/League-API-bot",
	}
	ApplyDefaultFooter(embed)
	return embed
}
