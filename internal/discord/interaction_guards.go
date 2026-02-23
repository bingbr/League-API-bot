package discord

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

const (
	CommandNotConfiguredMessage = "The command is not configured."
	CommandGuildOnlyMessage     = "This command is only available with the bot added to the server."
)

func RequireCommandConfigured(s *discordgo.Session, i *discordgo.InteractionCreate, configured bool) bool {
	if configured {
		return true
	}
	RespondWithError(s, i, CommandNotConfiguredMessage)
	return false
}

func RequireGuildCommand(s *discordgo.Session, i *discordgo.InteractionCreate) (string, bool) {
	guildID := strings.TrimSpace(i.GuildID)
	if guildID == "" {
		RespondWithError(s, i, CommandGuildOnlyMessage)
		return "", false
	}
	return guildID, true
}
