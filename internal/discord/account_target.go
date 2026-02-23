package discord

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bwmarrin/discordgo"
)

const (
	minNickLen, maxNickLen = 7, 22
)

func ParseAccountTargetOptions(i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) (region, nick, tag, validationErr string) {
	region = OptionValueByName(options, "region")
	if region == "" {
		return "", "", "", fmt.Sprintf("The %s is required.", "region")
	}
	if riot.NormalizePlatformRegion(region) == "" {
		return "", "", "", "The region entered is invalid."
	}

	nick = OptionValueByName(options, "nick")
	if nick == "" {
		return "", "", "", fmt.Sprintf("The %s is required.", "nick")
	}
	if !isValidLen(nick, minNickLen, maxNickLen) {
		return "", "", "", fmt.Sprintf("The nick must be between %d and %d characters", minNickLen, maxNickLen)
	}
	nick, tag, err := riot.SplitRiotID(nick)
	if err != nil {
		return "", "", "", "The nick is invalid. Use format nickname#tagline."
	}
	return region, nick, tag, ""
}

func MapAccountNotFoundHint(i *discordgo.InteractionCreate, err error, nick, tag string) (string, bool) {
	if !riot.IsAccountByRiotIDNotFound(err) {
		return "", false
	}
	account := riot.FormatRiotID(nick, tag)
	return fmt.Sprintf("Your account nick is invalid or you entered it incorrectly.\nNo results found for player `%s`.", account), true
}

func OptionValueByName(options []*discordgo.ApplicationCommandInteractionDataOption, name string) string {
	for _, option := range options {
		if option.Name == name {
			if value, ok := option.Value.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func FocusedOptionValue(options []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, option := range options {
		if option.Focused {
			if value, ok := option.Value.(string); ok {
				return strings.TrimSpace(value)
			}
			return ""
		}
	}
	return ""
}

func isValidLen(s string, min, max int) bool {
	l := utf8.RuneCountInString(s)
	return l >= min && l <= max
}
