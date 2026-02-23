# League API bot

A Discord bot that looks up League of Legends player stats, tracks live and post-game results, and shows the weekly free champion rotation. Built with Go, the Riot Games API, and PostgreSQL.

### Made with
* [GO](https://go.dev/) ([DiscordGo](https://github.com/bwmarrin/discordgo) and [pgx](https://github.com/jackc/pgx/))
* [PostgreSQL](https://www.postgresql.org/)

## Try it now!
### [Add to Discord](https://discord.com/oauth2/authorize?client_id=961732062782562304)

## Commands
| Command | Autocomplete | Description |
|:--------|:------------:|:------------|
| [`/search`](#summoner) | `region` | View information about an account (level, solo/duo and flex rank). |
| [`/free week`](#free-champion) | — | View the current free champion rotation. |
| [`/leaderboard`](#leaderboard) | — | Show tracked players ranked by solo/duo MMR. |
| [`/track config`](#configuration) | `channel` | Set the channel where tracking updates are posted. |
| [`/track add`](#configuration) | `region` | Add an account to track. Posts live-game and post-game info. |
| [`/track remove`](#configuration) | `account` | Stop tracking an account. |

## How to run in the cloud
1. Open [Railway](https://railway.app/) or a similar cloud service
1. Clone this repo into your `New Project`
1. Add [Postgresql](https://docs.railway.app/databases/postgresql) database to your created project.
1. Set your [Riot Games API key](https://developer.riotgames.com/), [Discord bot token](https://discord.com/developers/applications) and `DATABASE_URL` in your League API bot `variables`.

## How to run locally
### Prerequisites
- [Docker](https://www.docker.com/get-started/)
- Docker Compose

1. Create your environment file:
```bash
cp .env.example .env
```
2. Edit `.env` and set your Discord and Riot API key:
```env
DISCORD_TOKEN=your_discord_bot_token
RIOT_API_KEY=RGAPI-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
```
3. Start Postgres and the bot:
```bash
docker compose up --build -d
```

## Screenshots
### Configuration
![Using auto-complete to configure the account tracker](/screenshots/track-config-autocomplete.png)
![Adding an account to be tracked](/screenshots/track-add-autocomplete.png)
![Bot response to commands](/screenshots/track.png)

### Live game and post game
![Live game and post-game statistics](/screenshots/track-live-post-game.png)

### Free champion
![Champion's free week rotation](/screenshots/free-week.png)

### Summoner 
![Account stats](/screenshots/summoner.png)

### Leaderboard
![Server leaderboard](/screenshots/leaderboard.png)

## Legal disclaimer
> League API Bot was created under Riot Games' ["Legal Jibber Jabber"](https://www.riotgames.com/en/legal) policy using assets owned by Riot Games.  Riot Games does not endorse or sponsor this project.