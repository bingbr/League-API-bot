package discord

import (
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	allCommands = []*discordgo.ApplicationCommand{
		{
			Name:        "free",
			Description: "It's free real estate!",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "champion",
					Description: "Show free champion rotation for this week. For unranked modes only.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
		{
			Name:        "track",
			Description: "Track an account.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "add",
					Description: "Add an account to track.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:         "region",
							Description:  "Select account region.",
							Type:         discordgo.ApplicationCommandOptionString,
							Autocomplete: true,
							Required:     true,
						},
						{
							Name:         "nick",
							Description:  "Insert nickname.",
							Type:         discordgo.ApplicationCommandOptionString,
							Autocomplete: false,
							Required:     true,
						},
					},
				},
				{
					Name:        "remove",
					Description: "Stop tracking an account.",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Options: []*discordgo.ApplicationCommandOption{
						{
							Name:         "region",
							Description:  "Select the account region.",
							Type:         discordgo.ApplicationCommandOptionString,
							Autocomplete: true,
							Required:     true,
						},
						{
							Name:         "nick",
							Description:  "Insert the nickname.",
							Type:         discordgo.ApplicationCommandOptionString,
							Autocomplete: false,
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
							Description:  "Highlight the #channel that will be in use by the bot.",
							Type:         discordgo.ApplicationCommandOptionString,
							Autocomplete: false,
							Required:     true,
						},
					},
				},
			},
		},
		{
			Name:        "summoner",
			Description: "View information about a League of Legends account.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "region",
					Description:  "Select account region.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
				{
					Name:         "nick",
					Description:  "Insert nickname to proceed.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: false,
					Required:     true,
				},
			},
		},
		{
			Name:        "mastery",
			Description: "Show champions mastery from a League of Legends account.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "region",
					Description:  "Select account region.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
				{
					Name:         "nick",
					Description:  "Insert nickname to proceed.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: false,
					Required:     true,
				},
				{
					Name:         "mastery",
					Description:  "Select champion mastery.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
			},
		},
		{
			Name:        "leaderboard",
			Description: "Display a leaderboard with the ranking of the accounts tracked on this server.",
			Type:        discordgo.ChatApplicationCommand,
		},
	}

	regionsOption   = []*discordgo.ApplicationCommandOptionChoice{}
	masteriesOption = []*discordgo.ApplicationCommandOptionChoice{}

	lang = language.Und
)

// Slash command autocomplete
func loadAutoComplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	switch len(data.Options) {
	case 1:
		switch {
		case data.Options[0].Options[0].Focused:
			var response []*discordgo.ApplicationCommandOptionChoice
			for _, choice := range regionsOption {
				if strings.Contains(cases.Lower(lang).String(choice.Name), cases.Lower(lang).String(data.Options[0].Options[0].StringValue())) {
					response = append(response, choice)
				}
			}
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: response}})
			if err != nil {
				log.Printf("Load autocomplete for track region error: %s", err)
			}
		}
	case 2:
		switch {
		case data.Options[0].Focused:
			var response []*discordgo.ApplicationCommandOptionChoice
			for _, choice := range regionsOption {
				if strings.Contains(cases.Lower(lang).String(choice.Name), cases.Lower(lang).String(data.Options[0].StringValue())) {
					response = append(response, choice)
				}
			}
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: response}})
			if err != nil {
				log.Printf("Load autocomplete for region error: %s", err)
			}
		}
	case 3:
		switch {
		case data.Options[2].Focused:
			var response []*discordgo.ApplicationCommandOptionChoice
			for _, choice := range masteriesOption {
				if strings.Contains(cases.Lower(lang).String(choice.Name), cases.Lower(lang).String(data.Options[2].StringValue())) {
					response = append(response, choice)
				}
			}
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{Type: discordgo.InteractionApplicationCommandAutocompleteResult, Data: &discordgo.InteractionResponseData{Choices: response}})
			if err != nil {
				log.Printf("Load autocomplete mastery error: %s", err)
			}
		}
	}
}
