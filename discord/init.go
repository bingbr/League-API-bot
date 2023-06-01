package discord

import (
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bingbr/League-API-bot/league"
	"github.com/bwmarrin/discordgo"
	// "github.com/joho/godotenv"
)

const guild = ""

func Connect() {
	// err := godotenv.Load(".env") // Required in order to run on localhost.
	// if err != nil {
	// 	panic(err)
	// }
	session, err := discordgo.New("Bot " + os.Getenv("DISCORD_TOKEN"))
	if err != nil {
		panic(err)
	}

	league.OpenSession()
	league.LoadData()

	session.Identify.Intents = discordgo.IntentsGuildMessages
	err = session.Open()
	if err != nil {
		panic(err)
	}
	go initAC()
	createCommands(session)
	log.Println("Bot Online.")
	go trackLogic(session)
	go updateLogic()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	session.Close()
	removeCommands(session, guild)
	log.Println("Bot Offline.")
}
