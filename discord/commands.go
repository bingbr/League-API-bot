package discord

import (
	"fmt"
	"log"
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
	}

	commandHand = map[string]func(session *discordgo.Session, interaction *discordgo.InteractionCreate){
		"search": func(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
			data := interaction.ApplicationCommandData()
			lRegion = data.Options[0].StringValue()

			if len(data.Options) == 1 {
				log.Print("Command failed, index out of range.")
			} else {
				lAccount = data.Options[1].StringValue()
				switch interaction.Type {
				case discordgo.InteractionApplicationCommand:
					if len(lAccount) < 2 || len(lAccount) > 22 || len(lRegion) < 1 || len(lRegion) > 5 {
						go msgInvalidAcc(session, interaction)
					} else {
						go msgAddContent(session, interaction)
					}
				case discordgo.InteractionApplicationCommandAutocomplete:
					go loadRegion(session, interaction)
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
				panic(err)
			}
		},
	}

	oldInteraction    *discordgo.InteractionCreate
	lRegion, lAccount string
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
	err := session.InteractionResponseDelete(session.State.User.ID, interaction.Interaction)
	if err != nil {
		panic(err)
	}
}

func msgInvalidAcc(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Your account %q isn`t valid.", lAccount),
			Flags:   1 << 6,
		},
	})
	if err != nil {
		panic(err)
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
		panic(err)
	}
}

func msgAddContent(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	user := interaction.Member.User.Username + "#" + interaction.Member.User.Discriminator + " id:" + interaction.Member.User.ID
	oldInteraction = interaction
	nickname := strings.Join(strings.Split(strings.ToLower(lAccount), " "), "+")
	go league.AccInfo(lRegion, "", user, nickname)
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("You tried to search for **%q**, is that right?", lAccount),
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
		log.Printf("Error: %s", err)
	}
}

func loadRegion(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	data := interaction.ApplicationCommandData()
	var choice []*discordgo.ApplicationCommandOptionChoice
	switch {
	case data.Options[0].Focused:
		choice = []*discordgo.ApplicationCommandOptionChoice{
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
	}
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{
			Choices: choice,
		},
	})
	if err != nil {
		panic(err)
	}
}

func showAccountInfo(session *discordgo.Session, interaction *discordgo.InteractionCreate) {
	err := session.InteractionRespond(interaction.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: league.AboutAcc(),
		},
	})
	if err != nil {
		msgError(session, interaction)
		log.Printf("Error: %s", err)
	}
	league.CleanAll()
}
