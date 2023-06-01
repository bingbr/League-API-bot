# League API bot
A Discord bot that utilizes the Riot Games API to bring the world of League of Legends into your server. With this bot, you can easily access information about your favorite players and their matches, including statistics, rankings and even the weekly free champions. You can also set up a notification so you never miss a player's game.

## Try it now!
### [Add this bot to your Discord server](https://discord.com/api/oauth2/authorize?client_id=961732062782562304&permissions=2147745792&scope=bot)

## Commands available
|      Command     | Autocomplete Option | Description |
|:----------------:|:-------------------:|:-----------:|
| [`/summoner`](#summoner)      | `region`            | View information about an account, such as level and rank statistics. |
| [`/mastery`](#mastery)       | `region`, `level`   | View the status of up to 25 champions from a single account at the mastery level of your choice. |
| [`/free champion`](#free-champion-rotation) | :x:                 | Show the latest weekly free champions rotation. |
| [`/track config`](#configuration)  | `channel`           | Choose the #Discord-channel for live & post-game stats from a tracked account. |
| [`/track add`](#configuration)     | `region`            | Track an account. The bot will let you know when the account start a match and when the match ends, including the result. |
| [`/track remove`](#configuration)  | `region`            | Stop tracking an account. |
| [`/leadboard`](#leaderboard)     | :x:                 | View the ranking of tracked accounts on the Discord server. |


## How to run in the cloud
1. Open [Railway](https://railway.app/) or a similar cloud service
1. Clone this repo into your `New Project`
1. Add [Postgresql](https://docs.railway.app/databases/postgresql) database to your created environment
1. Place your [Riot Games API token](https://developer.riotgames.com/) and your [Discord bot token](https://discord.com/developers/applications) in your League API bot `variables`.
```
DISCORD_TOKEN=****
RIOT_TOKEN=****
```

## Print
### Configuration
![Print using auto-complete to configure the account tracker](/print/track-config-autocomplete.webp)
![Print of adding an account to be track](/print/track-add-autocomplete.webp)
![Print with bot response to commands](/print/track.webp)

### Live game and post game
![Print of live game and post-game statistics](/print/track-live-post-game.webp)

### Free champion rotation
![Print showing the champion's free week rotation](/print/free-week.webp)

### Summoner 
![Print showing an account stats](/print/summoner.webp)

### Mastery
![Print showing the mastery status of champions from one account](/print/mastery.webp)

### Leaderboard
![Print showing the leaderboard of a Discord server](/print/leaderboard.webp)


## Legal disclaimer
> League API Bot was created under Riot Games' ["Legal Jibber Jabber"](https://www.riotgames.com/en/legal) policy using assets owned by Riot Games.  Riot Games does not endorse or sponsor this project.


### Made with
* [GO](https://go.dev/) ([DiscordGo](https://github.com/bwmarrin/discordgo), [GoDotEnv](https://github.com/joho/godotenv) and [pgx](https://github.com/jackc/pgx/))
* [PostgreSQL](https://www.postgresql.org/)
