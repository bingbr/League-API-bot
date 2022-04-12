# League of Legends API Bot
A Discord bot that uses League of Legends API. Display info about your account, such as: Level, Ranked stats (Elo, Winrate, Win, Losses)

## How to use
1. Clone this repository: `git clone https://github.com/bingbr/League-API-Bot.git`
1. Put your [Riot Games API Token](https://developer.riotgames.com/) in [tokenRiot](/league/init.go)
1. Put your [Discord Bot Token](https://discord.com/developers/applications) in [tokenDiscord](/discord/init.go)
1. Open repository in Terminal/CMD.
1. Run `go run main.go`

### Made with:
* [GO](https://go.dev/)
* [DiscordGo](https://github.com/bwmarrin/discordgo)
* [FastHTTP](https://github.com/valyala/fasthttp)