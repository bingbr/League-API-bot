package discord

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bingbr/League-API-bot/league"

	"github.com/bwmarrin/discordgo"
)

var (
	tokenDiscord = "YOUR_TOKEN"
	guild        = "" // Can use GuildID of your Discord server.
)

func Connect() {
	session, err := discordgo.New("Bot " + tokenDiscord)
	if err != nil {
		panic(err)
	}

	league.LoadLeagueChampions()

	session.Identify.Intents = discordgo.IntentsGuildMessages
	err = session.Open()
	if err != nil {
		panic(err)
	}

	CreateCommands(session)
	log.Println("Bot Online.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	session.Close()
	RemoveCommands(session, guild)
	log.Println("Bot Offline.")
}
