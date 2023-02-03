package discord

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bingbr/League-API-bot/league"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/text/cases"
)

var (
	commandHand = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		"free": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			switch i.ApplicationCommandData().Options[0].Name {
			case "champion":
				go rotationChamps(s, i)
			}
		},
		"track": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			data := i.ApplicationCommandData()
			switch data.Options[0].Name {
			case "add":
				if len(data.Options) != 1 {
					log.Print("Track add account index out of range.")
				} else {
					switch i.Type {
					case discordgo.InteractionApplicationCommand:
						typed_region, typed_account := data.Options[0].Options[0].StringValue(), data.Options[0].Options[1].StringValue()
						switch isValid(typed_account) {
						case true:
							switch match(regionsOption, typed_region) {
							case true:
								compacted_account := strings.Join(strings.Split(cases.Lower(lang).String(typed_account), " "), "+")
								go trackAdd(i.GuildID, typed_region, compacted_account, s, i)
							default:
								go msgPrivate("Region is invalid.", s, i)
							}
						default:
							go msgPrivate("Your account is invalid.", s, i)
						}
					case discordgo.InteractionApplicationCommandAutocomplete:
						go loadAutoComplete(s, i)
					}
				}
			case "remove":
				if len(data.Options) != 1 {
					log.Print("Track remove account index out of range.")
				} else {
					switch i.Type {
					case discordgo.InteractionApplicationCommand:
						typed_region, typed_account := data.Options[0].Options[0].StringValue(), data.Options[0].Options[1].StringValue()
						switch isValid(typed_account) {
						case true:
							switch match(regionsOption, typed_region) {
							case true:
								compacted_account := strings.Join(strings.Split(cases.Lower(lang).String(typed_account), " "), "+")
								go trackDelete(i.GuildID, typed_region, compacted_account, s, i)
							default:
								go msgPrivate("Region is invalid.", s, i)
							}
						default:
							go msgPrivate("Your account is invalid.", s, i)
						}
					case discordgo.InteractionApplicationCommandAutocomplete:
						go loadAutoComplete(s, i)
					}
				}
			case "config":
				if len([]rune(data.Options[0].Options[0].StringValue())) < 19 || len([]rune(data.Options[0].Options[0].StringValue())) > 23 {
					go msgPrivate("Make sure to select the #channel using Discord auto-select.", s, i)
				} else {
					channel_id := strings.TrimPrefix(strings.TrimSuffix(data.Options[0].Options[0].StringValue(), ">"), "<#")
					switch regexp.MustCompile("^[0-9]+$").MatchString(channel_id) {
					case false:
						go msgPrivate("Make sure to select the #channel using Discord auto-select.", s, i)
					case true:
						go trackConfig(i.GuildID, channel_id, s, i)
					}
				}
			}
		},
		"summoner": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			data := i.ApplicationCommandData()
			if len(data.Options) == 1 {
				log.Print("Summoner index out of range.")
			} else {
				switch i.Type {
				case discordgo.InteractionApplicationCommand:
					typed_region, typed_account := data.Options[0].StringValue(), data.Options[1].StringValue()
					switch isValid(typed_account) {
					case true:
						switch match(regionsOption, typed_region) {
						case true:
							compacted_account := strings.Join(strings.Split(cases.Lower(lang).String(typed_account), " "), "+")
							go aboutAccount(typed_region, compacted_account, s, i)
						default:
							go msgPrivate("Region is invalid.", s, i)
						}
					default:
						go msgPrivate("Your account is invalid.", s, i)
					}
				case discordgo.InteractionApplicationCommandAutocomplete:
					go loadAutoComplete(s, i)
				}
			}
		},
		"mastery": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			data := i.ApplicationCommandData()
			if len(data.Options) == 1 {
				log.Print("Summoner index out of range.")
			} else {
				switch i.Type {
				case discordgo.InteractionApplicationCommand:
					typed_region, typed_account, typed_mastery := data.Options[0].StringValue(), data.Options[1].StringValue(), data.Options[2].StringValue()
					switch isValid(typed_account) {
					case true:
						switch match(regionsOption, typed_region) {
						case true:
							switch match(masteriesOption, typed_mastery) {
							case true:
								compacted_account := strings.Join(strings.Split(cases.Lower(lang).String(typed_account), " "), "+")
								go masteryAccount(typed_region, compacted_account, typed_mastery, s, i)
							default:
								go msgPrivate("Mastery is invalid.", s, i)
							}
						default:
							go msgPrivate("Region is invalid.", s, i)
						}
					default:
						go msgPrivate("Your account is invalid.", s, i)
					}
				case discordgo.InteractionApplicationCommandAutocomplete:
					go loadAutoComplete(s, i)
				}
			}
		},
		"leaderboard": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			go leaderboard(i.GuildID, s, i)
		},
	}

	componentHand = map[string]func(s *discordgo.Session, i *discordgo.InteractionCreate){
		// "yes": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// },
		// "no": func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		// },
	}
)

// Check if an account is valid.
func isValid(account string) (resp bool) {
	if len([]rune(account)) > 2 && len([]rune(account)) < 18 {
		resp = true
	}
	return resp
}

// Check if a typed option is valid.
func match(options []*discordgo.ApplicationCommandOptionChoice, typed string) (resp bool) {
	for _, choice := range options {
		available := choice.Value.(string)
		if strings.Compare(cases.Lower(lang).String(available), cases.Lower(lang).String(typed)) == 0 {
			resp = true
		}
	}
	return resp
}

// Check if a channel is invalid.
func channelInvalid(channelId string, s *discordgo.Session) (resp bool) {
	_, err := s.Channel(channelId)
	if err != nil {
		if strings.Contains(err.Error(), "404 Not Found") {
			resp = true
		}
	}
	return resp
}

// Creates all slash commands.
func createCommands(session *discordgo.Session) {
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
		log.Fatalf("Unable to register commands: %v", err)
	}
}

// Remove all created slash commands.
func removeCommands(session *discordgo.Session, guild string) {
	registeredCommands, err := session.ApplicationCommands(session.State.User.ID, guild)
	if err != nil {
		log.Fatalf("Unable to remove commands: %v", err)
	}
	for _, cmd := range registeredCommands {
		err := session.ApplicationCommandDelete(session.State.User.ID, guild, cmd.ID)
		if err != nil {
			log.Panicf("Unable to remove %q: %v", cmd.Name, err)
		}
	}
}

// Load autocomplete options.
func initAC() {
	league.LoadLocal(".data/json/region.json", &regionsOption)
	league.LoadLocal(".data/json/mastery.json", &masteriesOption)
}

// Template for sending a private message.
func msgPrivate(message string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: message,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Printf("Private Message Error: %s", err)
	}
}

// Footer of embed messages.
func footer() *discordgo.MessageEmbedFooter {
	return &discordgo.MessageEmbedFooter{
		Text:    "League API bot",
		IconURL: league.L.Cdn + "/" + league.L.Version + "/img/profileicon/5119.png",
	}
}

// Template for displaying data from an embedded quote.
func templateInfoQuote(typo string, info []string, color int, name, ico string, fields []*discordgo.MessageEmbedField) []*discordgo.MessageEmbed {
	switch typo {
	case "simple":
		return []*discordgo.MessageEmbed{
			{
				Title: info[0], Description: info[1], Color: color,
				URL:       "https://github.com/bingbr/League-API-bot",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{
					Name:    name,
					IconURL: ico,
				},
				Fields: fields,
				Footer: footer(),
			},
		}
	default:
		return []*discordgo.MessageEmbed{
			{
				Title: info[0], Description: info[1], Color: color,
				URL:       "https://github.com/bingbr/League-API-bot",
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Author: &discordgo.MessageEmbedAuthor{
					Name:    name,
					IconURL: ico,
				},
				Thumbnail: &discordgo.MessageEmbedThumbnail{
					URL: info[2],
				},
				Fields: fields,
				Footer: footer(),
			},
		}
	}
}

// Template for sending an embedded quote response
func templateBasicQuote(name string, data *discordgo.InteractionResponseData, s *discordgo.Session, i *discordgo.InteractionCreate) {
	err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: data,
	})
	if err != nil {
		log.Printf("%v error: %s", name, err)
	}
}

// Template for displaying data from an embed.
func templateInfo(info []string, color int, name, ico string, fields []*discordgo.MessageEmbedField) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title: info[0], Description: info[1], Color: color,
		URL:       "https://github.com/bingbr/League-API-bot",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Author: &discordgo.MessageEmbedAuthor{
			Name:    name,
			IconURL: ico,
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: info[2],
		},
		Fields: fields,
		Footer: footer(),
	}
}

// Template for sending an embed response.
func templateBasic(name, channel string, data *discordgo.MessageEmbed, s *discordgo.Session) (message *discordgo.Message) {
	message, err := s.ChannelMessageSendEmbed(channel, data)
	if err != nil {
		log.Printf("%v error: %s", name, err.Error())
	}
	return message
}

// Template for replying with an embed response.
func templateBasicReply(name, channel string, data *discordgo.MessageEmbed, msg *discordgo.MessageReference, s *discordgo.Session) (message *discordgo.Message) {
	message, err := s.ChannelMessageSendEmbedReply(channel, data, msg)
	if err != nil {
		log.Printf("%v error: %s", name, err)
	}
	return message
}

// Response to available weekly champions.
func rotationChamps(s *discordgo.Session, i *discordgo.InteractionCreate) {
	info, champions := league.ChampionsAvailable()

	templateBasicQuote("Show champions for free", &discordgo.InteractionResponseData{
		Embeds: templateInfoQuote("simple", info, 0x54fafa, "Free Champion Rotation", league.L.Cdn+"/"+league.L.Version+"/img/profileicon/4520.png", []*discordgo.MessageEmbedField{{Name: "Free For All", Value: champions[0], Inline: true}, {Name: "Free Until Level 10", Value: champions[1], Inline: true}}),
	}, s, i)
}

// Response to set a default Discord channel.
func trackConfig(guild, channel string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	registred, c := league.ServerIsRegistred(guild)
	switch registred {
	case false:
		go league.TrackConfig(guild, channel)
		go msgPrivate("The bot has been configured. All tracking information will be sent to <#"+channel+">.", s, i)
	case true:
		if c == channel {
			go msgPrivate("The selected channel '<#"+channel+">' is already the default. No changes have been made.", s, i)
		} else {
			go league.TrackConfig(guild, channel)
			go msgPrivate("<#"+channel+"> will be the new default channel.", s, i)
		}
	}
}

// Response to add an account to be track.
func trackAdd(guild, region, compacted_account string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	registred, channel := league.ServerIsRegistred(guild)
	switch registred {
	case false:
		go msgPrivate("Bot is not configured to work on this discord server. Please use ``/track config`` before trying to add an account.", s, i)
	case true:
		if channel == "" {
			go msgPrivate("You must add a #channel using ``/track config`` before attempting to add an account.", s, i)
		} else {
			_, err := s.Channel(channel)
			if err != nil {
				if strings.Contains(err.Error(), "404 Not Found") {
					go msgPrivate("Oops! It seems that the configured #channel no longer exists. Use ``/track config`` to define a new one.", s, i)
				} else {
					go msgPrivate("Oops! Something went wrong. Please use ``/track config`` to define a new #channel.", s, i)
				}
			} else {
				switch league.IsTracked(region, compacted_account, channel) {
				case false:
					switch league.SearchAccount(region, compacted_account, "track") {
					case false:
						go msgPrivate("Your account is invalid or you entered it incorrectly.", s, i)
					case true:
						msgPrivate(league.TrackAttach(region, compacted_account, channel)+" will be tracked and account updates will be displayed in <#"+channel+">.", s, i)
					}
				case true:
					go msgPrivate("Your account is already tracked.", s, i)
				}
			}

		}
	}
}

// Response to a request to delete a tracked account.
func trackDelete(guild, region, compacted_account string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	registred, channel := league.ServerIsRegistred(guild)
	switch registred {
	case false:
		go msgPrivate("Bot is not configured to work on this discord server. Please use ``/track config`` before trying to use this command.", s, i)
	case true:
		if channel == "" {
			go msgPrivate("You must to add a #Channel using ``/track config`` before trying to use this command.", s, i)
		} else {
			switch league.IsTracked(region, compacted_account, channel) {
			case false:
				go msgPrivate("Your account is invalid or you entered it incorrectly.", s, i)
			case true:
				msgPrivate(league.TrackRemove(region, compacted_account, channel)+" is no longer tracked.", s, i)
			}
		}
	}
}

func fieldRanks(solo, flex string) (rank []*discordgo.MessageEmbedField) {
	if solo == "" && flex == "" {
		rank = []*discordgo.MessageEmbedField{}
	} else if flex == "" {
		rank = []*discordgo.MessageEmbedField{
			{Name: "Ranked Solo", Value: solo, Inline: true},
		}
	} else if solo == "" {
		rank = []*discordgo.MessageEmbedField{
			{Name: "Ranked Flex", Value: flex, Inline: true},
		}
	} else {
		rank = []*discordgo.MessageEmbedField{
			{Name: "Ranked Solo", Value: solo, Inline: true},
			{Name: "Ranked Flex", Value: flex, Inline: true},
		}
	}
	return rank
}

func basicAccountLayout(region, account string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	info, solo, flex := league.ShowAccountInfo(region, account)
	templateBasicQuote("Show account", &discordgo.InteractionResponseData{
		Embeds: templateInfoQuote("", info, 0x2b2b2b, "About Account", league.L.Cdn+"/"+league.L.Version+"/img/profileicon/29.png", fieldRanks(solo, flex)),
	}, s, i)
}

// Response to display an account.
func aboutAccount(region, account string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch league.IsNew(region, account, "") {
	case false:
		basicAccountLayout(region, account, s, i)
	case true:
		switch league.SearchAccount(region, account, "info") {
		case false:
			go msgPrivate("Your account is invalid or you entered it incorrectly.", s, i)
		case true:
			basicAccountLayout(region, account, s, i)
		}
	}
}

func fieldMastery(m_data []string) (m []*discordgo.MessageEmbedField) {
	if len(m_data) < 2 {
		m = []*discordgo.MessageEmbedField{}
	} else {
		m = []*discordgo.MessageEmbedField{
			{Name: "Champions", Value: m_data[0], Inline: true},
			{Name: "Points", Value: m_data[1], Inline: true},
			{Name: "Last Played", Value: m_data[2], Inline: true},
		}
	}
	return m
}

func dataMastery(lvl int, info, mastery []string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	switch lvl {
	case 4, 5, 6:
		templateBasicQuote(fmt.Sprintf("Show mastery %v", lvl), &discordgo.InteractionResponseData{
			Embeds: templateInfoQuote("", info, 0xbfa051, fmt.Sprintf("Mastery %v", lvl), fmt.Sprintf("https://raw.communitydragon.org/latest/game/assets/ux/mastery/mastery_icon_%d.png", lvl), fieldMastery(mastery)),
		}, s, i)
	case 7:
		templateBasicQuote(fmt.Sprintf("Show mastery %v", lvl), &discordgo.InteractionResponseData{
			Embeds: templateInfoQuote("", info, 0xbfa051, "Max Level Mastery", fmt.Sprintf("https://raw.communitydragon.org/latest/game/assets/ux/mastery/mastery_icon_%d.png", lvl), fieldMastery(mastery)),
		}, s, i)
	default:
		templateBasicQuote(fmt.Sprintf("Show mastery %v", lvl), &discordgo.InteractionResponseData{
			Embeds: templateInfoQuote("", info, 0xbfa051, fmt.Sprintf("Mastery %v", lvl), "https://raw.communitydragon.org/latest/game/assets/ux/mastery/mastery_icon_default.png", fieldMastery(mastery)),
		}, s, i)
	}
}

func basicMasteryLayout(region, account string, lvl int, s *discordgo.Session, i *discordgo.InteractionCreate) {
	info, mas, length := league.ShowAccountMastery(region, account, lvl)
	if length != 0 {
		dataMastery(lvl, info, mas, s, i)
	} else {
		msgPrivate(fmt.Sprintf("Account does not have a mastery %d champion.", lvl), s, i)
	}
}

// Response to displaying champion mastery for an account.
func masteryAccount(region, account, lvl_converted string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	lvl, _ := strconv.Atoi(lvl_converted)
	switch league.IsNew(region, account, "") {
	case false:
		basicMasteryLayout(region, account, lvl, s, i)
	case true:
		switch league.SearchAccount(region, account, "mastery") {
		case false:
			go msgPrivate("Your account is invalid or you entered it incorrectly.", s, i)
		case true:
			basicMasteryLayout(region, account, lvl, s, i)
		}
	}
}

// Response to server leaderboard.
func leaderboard(guild string, s *discordgo.Session, i *discordgo.InteractionCreate) {
	registred, channel := league.ServerIsRegistred(guild)
	switch registred {
	case false:
		go msgPrivate("Bot is not configured to work on this discord server. Please use ``/track config`` before trying to use this command.", s, i)
	case true:
		if channel == "" {
			go msgPrivate("You must to add a #Channel using ``/track config`` before trying to use this command.", s, i)
		} else {
			desc, content, valid := league.ShowLeaderboard(channel)
			if valid {
				templateBasicQuote("Show leaderboard", &discordgo.InteractionResponseData{
					Embeds: templateInfoQuote("simple", desc, 0xf4b94b, "Leaderboard", league.L.Cdn+"/"+league.L.Version+"/img/profileicon/5496.png", []*discordgo.MessageEmbedField{{Name: "Nick", Value: content[0], Inline: true}, {Name: "Rank", Value: content[1], Inline: true}, {Name: "Win Rate", Value: content[2], Inline: true}}),
				}, s, i)
			} else {
				go msgPrivate("Discord server must have a tracked account.\nAdd one with ``/track add`` before trying to use this command.", s, i)
			}
		}
	}
}

func fieldLive(red, blue []string) (m []*discordgo.MessageEmbedField) {
	if len(red) == 3 && len(blue) == 3 {
		m = []*discordgo.MessageEmbedField{
			{Name: "🔵 Team", Value: blue[0], Inline: true}, {Name: "Rank", Value: blue[1], Inline: true}, {Name: "Win Rate", Value: blue[2], Inline: true},
			{Name: "🔴 Team", Value: red[0], Inline: true}, {Name: "Rank", Value: red[1], Inline: true}, {Name: "Win Rate", Value: red[2], Inline: true},
		}
	} else if len(red) > 3 && len(blue) > 3 {
		m = []*discordgo.MessageEmbedField{
			{Name: "🔵 Team", Value: blue[0], Inline: true}, {Name: "Rank", Value: blue[1], Inline: true}, {Name: "Win Rate", Value: blue[2], Inline: true},
			{Name: "🔴 Team", Value: red[0], Inline: true}, {Name: "Rank", Value: red[1], Inline: true}, {Name: "Win Rate", Value: red[2], Inline: true},
			{Name: "🔵  Bans", Value: blue[3], Inline: true}, {Name: "🔴 Bans", Value: red[3], Inline: true},
		}
	} else {
		m = []*discordgo.MessageEmbedField{}
	}
	return m
}

func fieldPost(data, ban []string) (m []*discordgo.MessageEmbedField) {
	if len(data) == 8 {
		if len(ban) > 1 {
			if len([]rune(ban[0])) > 2 && len([]rune(ban[1])) > 2 {
				m = []*discordgo.MessageEmbedField{
					{Name: "Champion", Value: data[0], Inline: true}, {Name: "KDA", Value: data[1], Inline: true},
					{Name: "Summoners", Value: data[2], Inline: false},
					{Name: data[3], Value: data[4], Inline: true}, {Name: data[5], Value: data[6], Inline: true},
					{Name: "Build", Value: data[7], Inline: false},
					{Name: "🔵 Bans", Value: ban[0], Inline: true}, {Name: "🔴 Bans", Value: ban[1], Inline: true},
				}
			} else {
				m = []*discordgo.MessageEmbedField{
					{Name: "Champion", Value: data[0], Inline: true}, {Name: "KDA", Value: data[1], Inline: true},
					{Name: "Summoners", Value: data[2], Inline: false},
					{Name: data[3], Value: data[4], Inline: true}, {Name: data[5], Value: data[6], Inline: true},
					{Name: "Build", Value: data[7], Inline: false},
				}
			}
		} else {
			m = []*discordgo.MessageEmbedField{
				{Name: "Champion", Value: data[0], Inline: true}, {Name: "KDA", Value: data[1], Inline: true},
				{Name: "Summoners", Value: data[2], Inline: false},
				{Name: data[3], Value: data[4], Inline: true}, {Name: data[5], Value: data[6], Inline: true},
				{Name: "Build", Value: data[7], Inline: false},
			}
		}
	} else {
		m = []*discordgo.MessageEmbedField{}
	}
	return m
}

// Display live game information.
func liveGame(session *discordgo.Session, run *sync.WaitGroup) {
	defer run.Done()
	lg, acc := league.LoadAllLiveGame("live", false)
	if len(lg) >= 1 {
		for i := range lg {
			desc, red, blue := league.ShowLiveGame(strings.ToLower(lg[i].PlatformID), acc[i].ID)
			if len(red) > 1 && len(blue) > 1 {
				if len([]rune(red[1])) > 3 && len([]rune(blue[1])) > 3 {
					msg := templateBasic("Show Live Game", lg[i].ChannelID,
						templateInfo(desc, 0xf9f9f9, "Live Game", league.L.Cdn+"/"+league.L.Version+"/img/profileicon/5376.png", fieldLive(red, blue)),
						session)
					lg[i].Update(acc[i].ID, msg.ID)
				}
			}
			time.Sleep(1 * time.Second)
		}
	}
}

// Display post game information.
func postGame(session *discordgo.Session, run *sync.WaitGroup) {
	defer run.Done()
	games, accs := league.LoadAllLiveGame("", true)
	if len(games) >= 1 {
		lg, acc := league.HasGameEnded(games, accs)
		for i := range lg {
			if len(lg) >= 1 {
				desc, dt, ban, players, ux := league.ShowPostGame(lg[i].PlatformID, fmt.Sprintf("%s_%d", lg[i].PlatformID, lg[i].GameID), acc[i].ID)
				if !(len(ux) == 0) {
					templateBasicReply("Show Post Game", lg[i].ChannelID, templateInfo(desc, ux[0], "Post Game", fmt.Sprintf("%s/%s/img/profileicon/%v.png", league.L.Cdn, league.L.Version, ux[1]), fieldPost(dt, ban)), &discordgo.MessageReference{MessageID: lg[i].MessageID, ChannelID: lg[i].ChannelID, GuildID: lg[i].GuildID}, session)
					lg[i].Remove(acc[i].ID)
					for _, player := range players {
						league.SearchAccount(lg[i].PlatformID, player, "track")
						time.Sleep(6 * time.Second)
					}
				}
				time.Sleep(1 * time.Second)
			}
		}
	}
}

// Disable account tracking when the used channel is deleted.
func disableTracking(session *discordgo.Session) (resp bool) {
	for _, db := range league.LoadAllServerDB() {
		for _, dguild := range session.State.Guilds {
			if db.GuildID == dguild.ID {
				switch channelInvalid(db.ChannelID, session) {
				case true:
					league.DisableTrackOnServer(db.ChannelID)
					resp = true
				}
			}
		}
	}
	return resp
}

// TODO: Better logic
func trackLogic(session *discordgo.Session) {
	for {
		var run sync.WaitGroup
		if !(disableTracking(session)) {
			go league.IsLiveGame(&run)
			go liveGame(session, &run)
			go postGame(session, &run)
			run.Add(3)
			run.Wait()
			time.Sleep(5 * time.Second)
		}
	}
}

// TODO: Refresh data
func updateLogic() {
	for {
		league.LoadFreeList()
		league.CleanCache()
		time.Sleep(time.Until(<-time.After(24 * time.Hour)))
	}
}
