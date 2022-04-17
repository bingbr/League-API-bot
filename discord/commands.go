package discord

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/bingbr/League-API-bot/league"
	"github.com/bwmarrin/discordgo"
)

var (
	allCommands = []*discordgo.ApplicationCommand{
		{
			Name:        "search",
			Description: "Show information about your League of Legends account.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "region",
					Description:  "Select region of your account.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
				{
					Name:         "nick",
					Description:  "Insert your nickname to proceed.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: false,
					Required:     true,
				},
			},
		},
		{
			Name:        "masteries",
			Description: "Show champions in your account with the chosen mastery.",
			Type:        discordgo.ChatApplicationCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:         "region",
					Description:  "Select region of your account.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
				{
					Name:         "nick",
					Description:  "Insert your nickname to proceed.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: false,
					Required:     true,
				},
				{
					Name:         "mastery",
					Description:  "Insert your desired mastery to show the champions.",
					Type:         discordgo.ApplicationCommandOptionString,
					Autocomplete: true,
					Required:     true,
				},
			},
		},
	}

	commandHand = map[string]func(session *discordgo.Session, interaction *discordgo.InteractionCreate){
		"search": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			data := interaction.ApplicationCommandData()
			if len(data.Options) == 1 {
				log.Print("Search index out of range.")
			} else {
				switch interaction.Type {
				case discordgo.InteractionApplicationCommand:
					region := data.Options[0].StringValue()
					account := data.Options[1].StringValue()
					if len(account) < 2 || len(account) > 22 || len(region) < 1 || len(region) > 5 {
						go msgInvalidAcc(account, session, interaction)
					} else {
						go msgAddContent(region, account, session, interaction)
					}
				case discordgo.InteractionApplicationCommandAutocomplete:
					go loadAutoComplete(session, interaction)
				}
			}
		},
		"masteries": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			data := interaction.ApplicationCommandData()
			if len(data.Options) == 1 {
				log.Print("Mastery index out of range.")
			} else {
				switch interaction.Type {
				case discordgo.InteractionApplicationCommand:
					region := data.Options[0].StringValue()
					account := data.Options[1].StringValue()
					mastery := data.Options[2].StringValue()
					if len(account) < 2 || len(account) > 22 || len(region) < 1 || len(region) > 5 {
						go msgInvalidAcc(account, session, interaction)
					} else {
						masteryCheck, _ := strconv.Atoi(mastery)
						if masteryCheck > 0 && masteryCheck < 8 {
							go msgMasterContent(region, account, int(masteryCheck), session, interaction)
						} else {
							go msgInvalidAcc(account, session, interaction)
						}
					}
				case discordgo.InteractionApplicationCommandAutocomplete:
					loadAutoComplete(session, interaction)
				}
			}
		},
	}

	componentHand = map[string]func(session *discordgo.Session, interaction *discordgo.InteractionCreate){
		"yes-confirm": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			go removeInteraction(session, oldInteraction)
			go showAccountInfo(session, interaction)
		},
		"no-confirm": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			go removeInteraction(session, oldInteraction)
			err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Ops, bye.",
					Flags:   1 << 6,
				},
			})
			if err != nil {
				log.Printf("No-Confirm Error: %s", err)
			}
		},
		"yes-mastery": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			go removeInteraction(session, oldInteraction)
			go showMasteryInfo(session, interaction)
		},
		"no-mastery": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			go removeInteraction(session, oldInteraction)
			err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "Ops, bye.",
					Flags:   1 << 6,
				},
			})
			if err != nil {
				log.Printf("No-Mastery Error: %s", err)
			}
		},
	}

	regionsOpt = []*discordgo.ApplicationCommandOptionChoice{
		{
			Name:  "Brazil",
			Value: "br1",
		},
		{
			Name:  "Europe North East",
			Value: "eun1",
		},
		{
			Name:  "Europe West",
			Value: "euw1",
		},
		{
			Name:  "Japan",
			Value: "jp1",
		},
		{
			Name:  "Korea",
			Value: "kr",
		},
		{
			Name:  "Latin America North",
			Value: "la1",
		},
		{
			Name:  "Latin America South",
			Value: "la2",
		},
		{
			Name:  "North America",
			Value: "na1",
		},
		{
			Name:  "Oceania",
			Value: "oc1",
		},
		{
			Name:  "Turkey",
			Value: "tr1",
		},
		{
			Name:  "Russia",
			Value: "ru",
		},
		{
			Name:  "PBE",
			Value: "pbe1",
		},
	}

	masteriesOpt = []*discordgo.ApplicationCommandOptionChoice{
		{
			Name:  "Mastery 1",
			Value: "1",
		},
		{
			Name:  "Mastery 2",
			Value: "2",
		},
		{
			Name:  "Mastery 3",
			Value: "3",
		},
		{
			Name:  "Mastery 4",
			Value: "4",
		},
		{
			Name:  "Mastery 5",
			Value: "5",
		},
		{
			Name:  "Mastery 6",
			Value: "6",
		},
		{
			Name:  "Max Level Mastery",
			Value: "7",
		},
	}

	oldInteraction *discordgo.InteractionCreate
)

func CreateCommands(session *discordgo.Session) {
	session.AddHandler(func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
		switch interaction.Type {
		case discordgo.InteractionMessageComponent:
			if h, ok := componentHand[interaction.MessageComponentData().CustomID]; ok {
				h(session, interaction)
			}
		default:
			if h, ok := commandHand[interaction.ApplicationCommandData().Name]; ok {
				h(session, interaction)
			}
		}
	})
	_, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, guild, allCommands)
	if err != nil {
		log.Fatalf("Não foi possível registrar os comandos: %v", err)
	}
}

func RemoveCommands(session *discordgo.Session, guild string) {
	// TODO: Fix not removing global commands
	registeredCommands, err := session.ApplicationCommands(session.State.User.ID, guild)
	if err != nil {
		log.Fatalf("Não foi possível remover os comandos: %v", err)
	}
	for _, cmd := range registeredCommands {
		err := session.ApplicationCommandDelete(session.State.User.ID, guild, cmd.ID)
		if err != nil {
			log.Panicf("Não foi possível remover %q: %v", cmd.Name, err)
		}
	}
}

func removeInteraction(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	err := session.InteractionResponseDelete(interaction.Interaction)
	if err != nil {
		log.Printf("Remove Interaction Error: %s", err)
	}
}

func msgInvalidAcc(account string, session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Your account %q isn't valid.", account),
			Flags:   1 << 6,
		},
	})
	if err != nil {
		log.Printf("Message Invalid Account Error: %s", err)
	}
}

func msgError(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Something went wrong.",
			Flags:   1 << 6,
		},
	})
	if err != nil {
		log.Printf("Message Error had an Error: %s", err)
	}
}

func msgAddContent(region, nick string, session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	user := interaction.Member.User.Username + "#" + interaction.Member.User.Discriminator + " id:" + interaction.Member.User.ID
	oldInteraction = interaction
	nickname := strings.Join(strings.Split(strings.ToLower(nick), " "), "+")
	go league.LoadAccInfo(region, user, nickname, 0)
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("You tried to search for **%q**, is that right?", nick),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Emoji: discordgo.ComponentEmoji{
								Name: "👍",
							},
							Label:    "Yes",
							Style:    discordgo.SuccessButton,
							CustomID: "yes-confirm",
						},
						discordgo.Button{
							Emoji: discordgo.ComponentEmoji{
								Name: "👎",
							},
							Label:    "No",
							Style:    discordgo.DangerButton,
							CustomID: "no-confirm",
						},
					},
				},
			},
		},
	})
	if err != nil {
		msgError(session, interaction)
		log.Printf("Message Add Content Error: %s", err)
	}
}

func msgMasterContent(region, nick string, mastery int, session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	user := interaction.Member.User.Username + "#" + interaction.Member.User.Discriminator + " id:" + interaction.Member.User.ID
	oldInteraction = interaction
	nickname := strings.Join(strings.Split(strings.ToLower(nick), " "), "+")
	var master string
	go league.LoadAccInfo(region, user, nickname, mastery)
	if mastery == 7 {
		master = "mastered"
	} else {
		master = fmt.Sprintf("mastery %d", mastery)
	}
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("You tried to search for %s champions in account **%q**, is that right?", master, nick),
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Emoji: discordgo.ComponentEmoji{
								Name: "👍",
							},
							Label:    "Yes",
							Style:    discordgo.SuccessButton,
							CustomID: "yes-mastery",
						},
						discordgo.Button{
							Emoji: discordgo.ComponentEmoji{
								Name: "👎",
							},
							Label:    "No",
							Style:    discordgo.DangerButton,
							CustomID: "no-mastery",
						},
					},
				},
			},
		},
	})
	if err != nil {
		msgError(session, interaction)
		log.Printf("Message Mastery Content Error: %s", err)
	}
}

func loadAutoComplete(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	data := interaction.ApplicationCommandData()
	switch len(data.Options) {
	case 2:
		switch {
		case data.Options[0].Focused:
			err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionApplicationCommandAutocompleteResult,
				Data: &discordgo.InteractionResponseData{
					Choices: regionsOpt,
				},
			})
			if err != nil {
				log.Printf("Load Autocomplete Region Error: %s", err)
			}
		}
	case 3:
		switch {
		case data.Options[2].Focused:
			err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionApplicationCommandAutocompleteResult,
				Data: &discordgo.InteractionResponseData{
					Choices: masteriesOpt,
				},
			})
			if err != nil {
				log.Printf("Load Autocomplete Mastery Error: %s", err)
			}
		}
	}
}

func showAccountInfo(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	_, response := league.Response("acc")
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response,
		},
	})
	if err != nil {
		msgError(session, interaction)
		log.Printf("Message Account Info Error: %s", err)
	}
}

func showMasteryInfo(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	oldInteraction = interaction
	length, response := league.Response("mst")
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: response,
		},
	})
	if err != nil {
		msgError(session, interaction)
		log.Printf("Message Mastery Info Error: %s", err)
	}
	for i := 1; i < length; i++ {
		masteryContinuation(i, session, interaction)
	}
}

func masteryContinuation(index int, session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	_, err := session.FollowupMessageCreate(interaction.Interaction, true, &discordgo.WebhookParams{
		Content: league.FollowupResponse(index),
	})
	if err != nil {
		log.Printf("Message Continuation Error: %s", err)
	}
}
