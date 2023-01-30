# League API bot
A Discord bot that uses Riot Games API. 

## Try it now
### [Add this bot to your Discord server](https://discord.com/api/oauth2/authorize?client_id=961732062782562304&permissions=2147745792&scope=bot)

## Commands available
|      Command     | Autocomplete Option | Description |
|:----------------:|:-------------------:|:-----------:|
| `/summoner`      | `region`            | Show information about an account, such as level and ranked stats. |
| `/mastery`       | `region`, `level`   | Show the masteries and others stats of the champions of an account. |
| `/free champion` | :x:                 | Show weekly free champions rotation. |
| `/track config`  | `channel`           | Define the `#discord-channel` where you want to receive live game and post game stats of a tracked account. |
| `/track add`     | `region`            | Add an account to be tracked. |
| `/track remove`  | `region`            | Stop tracking an account. |
| `/leadboard`     | :x:                 | Show ranking of tracked accounts on discord server. |


## How to run on cloud
1. Open [Railway](https://railway.app/) or a similar service
1. Clone this repo in your  `New Project`
1. Add [Postgresql](https://docs.railway.app/databases/postgresql) database to your created enviroment
1. Put your [Riot Games API Token](https://developer.riotgames.com/) and your [Discord Bot Token](https://discord.com/developers/applications) inside of your League-API-bot `Variables`
```
DISCORD_TOKEN=****
RIOT_TOKEN=****
```

## Print
### Summoner 
![Displaying an account stats](/print/summoner.png)

### Mastery
![Showing autocomplete for region and mastery](/print/autocomplete-mastery.png)
![Displaying mastery status of 25 champions from an account](/print/mastery.png)

### Defining channel
![Configuring tracker using autocomplete](/print/config.png)

### Tracked account live game
![Displaying live game stats](/print/livegame.png)

### Tracked account post game
![Displaying post game stats](/print/postgame.webp)

## Legal disclaimer
> League API Bot was created under Riot Games' ["Legal Jibber Jabber"](https://www.riotgames.com/en/legal) policy using assets owned by Riot Games.  Riot Games does not endorse or sponsor this project.


### Made with
* [GO](https://go.dev/) ([DiscordGo](https://github.com/bwmarrin/discordgo), [GoDotEnv](https://github.com/joho/godotenv) and [pgx](https://github.com/jackc/pgx/))
* [PostgreSQL](https://www.postgresql.org/)
