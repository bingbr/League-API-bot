package discord

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
)

var (
	tokenDiscord = "YOUR_TOKEN"
	guild        = ""
)

func Connect() {
	session, err := discordgo.New("Bot " + tokenDiscord)
	if err != nil {
		panic(err)
	}
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
