package league

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	db  *pgxpool.Pool
	ctx context.Context
)

func OpenSession() {
	log.Println("Connecting to PostgreSQL")
	data, err := pgxpool.New(context.Background(), fmt.Sprintf("postgresql://%s:%s@%s:%s/%s", os.Getenv("POSTGRES_USER"), os.Getenv("POSTGRES_PASSWORD"), os.Getenv("POSTGRES_HOSTNAME"), os.Getenv("POSTGRES_PORT"), os.Getenv("POSTGRES_DB")))
	if err != nil {
		log.Printf("Database connection error: %s", err)
	}
	db = data
	ctx = context.Background()
	Tables()
}

func CloseSession() {
	defer db.Close()
}

func Tables() {
	// Discord server
	createTable("discord", "discord", "guild text not null primary key, channel text unique")

	// Champion
	createTable("champion", "champion", "nome text not null, id int not null primary key, full_name text not null, ico text")

	// Item
	createTable("item", "item", "nome text not null, id int not null primary key")

	// Runes
	createTable("runes", "perk", "nome text not null, id int not null primary key, ico text")

	// Summoners
	createTable("summoners", "summoner", "nome text not null, id int not null primary key, full_name text not null, ico text")

	// Champion free until level 10
	createTable("champion free until 10", "fft", "id int not null primary key references champion(id)")

	// Champion free for all
	createTable("champion free for all", "ffa", "id int not null primary key references champion(id)")

	// Queues
	createTable("queues", "queues", "id int not null primary key, map text, descr text")

	// Riot account
	createTable("league account", "account", "id varchar(63) not null unique, accid varchar(56) not null unique, puuid varchar(78) not null primary key, nick varchar(17) not null, profile_icon int not null, revision_date bigint not null, summoner_level int, region varchar(4) not null, continent varchar(8) not null, compressed_nick varchar(17) not null, updated timestamp default now()")

	// Track player
	_, err := db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\"")
	if err != nil {
		log.Printf("Failed to install UUID: %s", err)
	}
	createTable("players tracked", "track_account", "id UUID not null primary key default uuid_generate_v4(), channel text not null references discord(channel) on update cascade, summoner varchar(63) not null references account(id)")

	// Ranked tier
	createTable("ranked tier", "ranks", "ico text, tier varchar(12) not null primary key")

	// Rank solo
	createTable("rank solo", "rank_solo", "league_id varchar(37), tier varchar(12) not null references ranks(tier) default 'UNRANKED', sub_tier text, summoner varchar(63) not null primary key references account(id), lp int, wins int, losses int, hot_streak boolean, s_target int, s_wins int, s_losses int, mmr int")

	// Rank flex
	createTable("rank flex", "rank_flex", "league_id varchar(37) not null, tier varchar(12) not null references ranks(tier) default 'UNRANKED', sub_tier text not null, summoner varchar(63) not null primary key references account(id), lp int not null default 0, wins int not null default 0, losses int not null default 0, hot_streak boolean, s_target int, s_wins int, s_losses int, mmr int default 0")

	// Champion mastery
	createTable("mastery", "mastery", "summoner varchar(63) not null primary key, champion jsonb")

	// Live game
	createTable("live game", "livegame", "id UUID not null primary key default uuid_generate_v4(), channel text not null, messageid text, gameid bigint not null, platformid text not null, summoner varchar(63) not null, posted boolean default false, updated timestamp default now()")
}

/*
*	Database management commands.
 */

// Template for creating tables.
func createTable(desc, name, content string) {
	_, err := db.Exec(ctx, "create table if not exists "+name+" ("+content+")")
	if err != nil {
		log.Printf("%s table creation error: %s", desc, err)
	}
}

// Delete the selected table.
func clearTable(table string) {
	_, err := db.Exec(ctx, "truncate table "+table+" cascade")
	if err != nil {
		log.Printf("Cleaning %s error: %s", table, err)
	}
}

/*
*	Commands for inserting information into the database.
 */

// Push the Discord server info to the database.
func insertDiscord(g, c string) {
	_, err := db.Exec(ctx, "insert into discord (guild, channel) values ($1, $2) on conflict (guild) do update set channel = excluded.channel", g, c)
	if err != nil {
		log.Printf("Failed to insert a Discord server info: %s", err)
	}
}

// Push champion information to the database.
func (cdnData Data) insertData(name string, cdata []Item) {
	for _, cdn := range cdnData.DataItem {
		for _, data := range cdata {
			if cdn.ID == data.ID {
				_, err := db.Exec(ctx, "insert into "+name+" (nome, id, full_name, ico) values ($1, $2, $3, $4) on conflict (id) do update set nome = excluded.nome, full_name = excluded.full_name, ico = excluded.ico", cdn.Name, cdn.ID, cdn.FullName, data.Icon)
				if err != nil {
					log.Printf("Failed to insert %ss: %s", name, err)
				}
			}
		}
	}
}

// Push item information to the database.
func (cdnData Data) insertItens() {
	for _, item := range cdnData.DataItem {
		_, err := db.Exec(ctx, "insert into item (id, nome) values ($1, $2) on conflict (id) do update set nome = excluded.nome", strings.TrimSuffix(item.Image.Full, ".png"), strings.TrimPrefix(strings.TrimSuffix(item.FullName, "</rarityLegendary><br><subtitleLeft><silver>500 Silver Serpents</silver></subtitleLeft>"), "<rarityLegendary>"))
		if err != nil {
			log.Printf("Failed to insert itens: %s", err)
		}
	}
}

// Push the rune information to the database.
func insertRunes(cdn []Rune, local []Rune) {
	for _, perkStyle := range cdn {
		for _, slots := range perkStyle.Slots {
			for _, perk := range slots.Runes {
				for _, runes := range local {
					if perkStyle.ID == runes.ID {
						_, err := db.Exec(ctx, "insert into perk (id, nome, ico) values ($1, $2, $3) on conflict (id) do update set nome = excluded.nome, ico = excluded.ico", perkStyle.ID, perkStyle.Name, runes.Icon)
						if err != nil {
							log.Printf("Failed to insert perkStyle runes: %s", err)
						}
					}
					if perk.ID == runes.ID {
						_, err := db.Exec(ctx, "insert into perk (id, nome, ico) values ($1, $2, $3) on conflict (id) do update set nome = excluded.nome, ico = excluded.ico", perk.ID, perk.Name, runes.Icon)
						if err != nil {
							log.Printf("Failed to insert perk runes: %s", err)
						}
					}
				}
			}
		}
	}
}

// Push the free champion to the database.
func insertChampionFree(free []int, f string) {
	for _, champion := range free {
		_, err := db.Exec(ctx, "insert into "+f+" (id) values ($1) on conflict do nothing", champion)
		if err != nil {
			log.Printf("Failed to insert champion rotation: %s", err)
		}
	}
}

// Push the tracked account information to the database.
func insertTrack(channel, account string) {
	_, err := db.Exec(ctx, "insert into track_account (channel, summoner) values ($1, $2) on conflict do nothing", channel, account)
	if err != nil {
		log.Printf("Failed to insert new account to be track: %s", err)
	}
}

// Push the account to the database.
func (acc Account) push(region, continent, compressed string) {
	_, err := db.Exec(ctx, "insert into account (id, accid, puuid, nick, profile_icon, revision_date, summoner_level, region, continent, compressed_nick, updated) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now()) on conflict (puuid) do update set id = excluded.id, accid = excluded.accid, nick = excluded.nick, profile_icon = excluded.profile_icon, revision_date = excluded.revision_date, summoner_level = excluded.summoner_level, region = excluded.region, continent = excluded.continent, compressed_nick = excluded.compressed_nick, updated = excluded.updated", acc.ID, acc.AccountID, acc.Puuid, acc.Name, acc.ProfileIconID, acc.RevisionDate, acc.SummonerLevel, region, continent, compressed)
	if err != nil {
		log.Printf("Failed to insert account: %s", err)
	}
}

// Push the ranked tier information to the database.
func insertRank(ranks []Rank) {
	for _, rank := range ranks {
		_, err := db.Exec(ctx, "insert into ranks (ico, tier) values ($1, $2) on conflict do nothing", rank.Icon, rank.Tier)
		if err != nil {
			log.Printf("Failed to insert ranks: %s", err)
		}
	}
}

// Push the account ranking information to the database.
func (r AccountRank) push(queue string) {
	_, err := db.Exec(ctx, "insert into "+queue+" (league_id, tier, sub_tier, summoner, lp, wins, losses, hot_streak, s_target, s_wins, s_losses, mmr) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) on conflict (summoner) do update set league_id = excluded.league_id, tier = excluded.tier, sub_tier = excluded.sub_tier, lp = excluded.lp, wins = excluded.wins, losses = excluded.losses, hot_streak = excluded.hot_streak, s_wins = excluded.s_wins, s_losses = excluded.s_losses, mmr = excluded.mmr", r.LeagueID, r.Tier, r.SubTier, r.SummonerID, r.LeaguePoints, r.Wins, r.Losses, r.HotStreak, r.MiniSeries.Target, r.MiniSeries.Wins, r.MiniSeries.Losses, r.mmr())
	if err != nil {
		log.Printf("Failed to insert account rank at %s: %s", queue, err)
	}
}

// Push the mastery of the champion's account into the database.
func insertMastery(summoner string, data interface{}) {
	_, err := db.Exec(ctx, "insert into mastery (summoner, champion) values ($1, $2) on conflict (summoner) do update set champion = excluded.champion", summoner, data)
	if err != nil {
		log.Printf("Failed to insert the account masteries: %s", err)
	}
}

// Push queue information to the database.
func insertQueues(q []Queue) {
	for _, queue := range q {
		_, err := db.Exec(ctx, "insert into queues (id, map, descr) values ($1, $2, $3) on conflict (id) do update set map = excluded.map, descr = excluded.descr", queue.QueueID, queue.Map, queue.Description)
		if err != nil {
			log.Printf("Failed to insert queues: %s", err)
		}
	}
}

// Push the account's live game information to the database.
func (lg LiveGame) insert(channel, summonerId string) {
	_, err := db.Exec(ctx, "insert into livegame (channel, gameid, platformid, summoner) values ($1, $2, $3, $4)", channel, lg.GameID, lg.PlatformID, summonerId)
	if err != nil {
		log.Printf("Failed to insert live game: %s", err)
	}
}

// Send updated live game information to the database.
func (lg LiveGame) Update(summonerId, messageId string) {
	_, err := db.Exec(ctx, "insert into livegame (id, channel, messageid, gameid, platformid, summoner, posted) values ($1, $2, $3, $4, $5, $6, $7) on conflict (id) do update set channel = excluded.channel, messageid = excluded.messageid, gameid = excluded.gameid, platformid = excluded.platformid, summoner = excluded.summoner, posted = excluded.posted", lg.ID, lg.ChannelID, messageId, lg.GameID, lg.PlatformID, summonerId, true)
	if err != nil {
		log.Printf("Failed to update live game: %s", err)
	}
}

/*
*	Commands for retrieving information from the database.
 */

// Check to see if a Discord server is stored in the database.
func ServerIsRegistred(guild string) (proceeds bool, channel string) {
	var saved_guild string
	_ = db.QueryRow(ctx, "select guild, channel from discord where guild = $1", guild).Scan(&saved_guild, &channel)
	if saved_guild == guild {
		proceeds = true
	}
	return proceeds, channel
}

// Retrieve champions weekly available to be play for free.
func loadFreeChampion(f string) (champion string) {
	resp, err := db.Query(ctx, "select c.ico, c.full_name from "+f+" as f join champion as c on c.id = f.id order by full_name")
	if err != nil {
		log.Printf("Failure to query champions for free: %s", err)
	}
	defer resp.Close()

	var in []string
	for resp.Next() {
		var icon, name string
		err = resp.Scan(&icon, &name)
		in = append(in, icon+" "+name)
		if err != nil {
			log.Printf("Failed to scan champion free: %s", err)
		}
	}
	champion = strings.Join(in, "\n")
	return champion
}

// Load account tracked from database.
func loadTrackedAccount(channel, summoner string) (info Account) {
	_ = db.QueryRow(ctx, "select id, channel, summoner from track_account where channel = $1 and summoner = $2", channel, summoner).Scan(&info.TrackID, &info.Channel, &info.ID)
	return info
}

// Load live game data from database.
func loadLiveGame(channel, summoner string) (lg LiveGame) {
	_ = db.QueryRow(ctx, "select channel, gameid, platformid, posted from livegame as live where channel = $1 and summoner = $2", channel, summoner).Scan(&lg.ChannelID, &lg.GameID, &lg.PlatformID, &lg.On)
	return lg
}

// Load all live game data from the database.
func LoadAllLiveGame(typo string, posted bool) (lg []LiveGame, accs []Account) {
	var live LiveGame
	var acc Account
	switch typo {
	case "live":
		resp, err := db.Query(ctx, "select live.id::text, live.channel, live.gameid, live.platformid, acc.id, acc.continent from livegame as live join account as acc on live.summoner = acc.id where live.posted = $1", posted)
		if err != nil {
			log.Printf("Failed to query for all live game data: %s", err)
		}
		defer resp.Close()
		for resp.Next() {
			err = resp.Scan(&live.ID, &live.ChannelID, &live.GameID, &live.PlatformID, &acc.ID, &acc.Continent)
			if err != nil {
				log.Printf("Failed to scan live game data: %s", err)
			}
			lg = append(lg, live)
			accs = append(accs, acc)
		}
	default:
		resp, err := db.Query(ctx, "select live.id::text, live.channel, live.messageid, live.gameid, live.platformid, acc.id, acc.continent from livegame as live join account as acc on live.summoner = acc.id where live.posted = $1", posted)
		if err != nil {
			log.Printf("Querying all live game data: %s", err)
		}
		defer resp.Close()
		for resp.Next() {
			err = resp.Scan(&live.ID, &live.ChannelID, &live.MessageID, &live.GameID, &live.PlatformID, &acc.ID, &acc.Continent)
			if err != nil {
				log.Printf("Failed to scan all live game data: %s", err)
			}
			lg = append(lg, live)
			accs = append(accs, acc)
		}
	}
	return lg, accs
}

// Load all tracked accounts.
func loadAllTrackedAccounts() (all []Account) {
	var account Account
	resp, err := db.Query(ctx, "select acc.region, acc.continent, channel, summoner, acc.nick from track_account as t join account as acc on acc.id = t.summoner")
	if err != nil {
		log.Printf("Failed to query all tracked accounts: %s", err)
	}
	defer resp.Close()
	for resp.Next() {
		err = resp.Scan(&account.Region, &account.Continent, &account.Channel, &account.ID, &account.Name)
		if err != nil {
			log.Printf("Failed to scan tracked account: %s", err)
		}
		all = append(all, account)
	}
	return all
}

// Load all servers from the database.
func LoadAllServerDB() (servers []Server) {
	var server Server
	resp, err := db.Query(ctx, "select guild, channel from discord")
	if err != nil {
		log.Printf("Failed to query all Discord servers: %s", err)
	}
	defer resp.Close()
	for resp.Next() {
		err = resp.Scan(&server.GuildID, &server.ChannelID)
		if err != nil {
			log.Printf("Failed to scan Discord server: %s", err)
		}
		servers = append(servers, server)
	}
	return servers
}

// Load champion from database using ID.
func loadChampionByID(id int) (champs Item) {
	_ = db.QueryRow(ctx, "select id, full_name, ico from champion where id = $1", id).Scan(&champs.ID, &champs.FullName, &champs.Icon)
	return champs
}

// Load item from database using ID.
func loadItemByID(id int) (item string) {
	_ = db.QueryRow(ctx, "select nome from item where id = $1", id).Scan(&item)
	return item
}

// Load summoners from database using ID.
func loadSummonerByID(id int) (sum Item) {
	err := db.QueryRow(ctx, "select id, full_name, ico from summoner where id = $1", id).Scan(&sum.ID, &sum.FullName, &sum.Icon)
	if err != nil {
		log.Printf("Failed to query summoner by id: %s", err)
	}
	return sum
}

// Load runes from database using ID.
func loadRuneByID(id int) (perk Rune) {
	err := db.QueryRow(ctx, "select id, nome, ico from perk where id = $1", id).Scan(&perk.ID, &perk.Name, &perk.Icon)
	if err != nil {
		log.Printf("Failed to query runes by id: %s", err)
	}
	return perk
}

// Load queue from database using ID.
func loadQueueByID(id int) (queue Queue) {
	err := db.QueryRow(ctx, "select id, map, descr from queues where id = $1", id).Scan(&queue.QueueID, &queue.Map, &queue.Description)
	if err != nil {
		log.Printf("Failed to query queue by id: %s", err)
	}
	return queue
}

// Load account from database using nick.
func loadAccountByNick(server, nick string) (acc Account) {
	_ = db.QueryRow(ctx, "select nick, id, revision_date, summoner_level, profile_icon from account where compressed_nick = $1 and region = $2", nick, server).Scan(&acc.Name, &acc.ID, &acc.RevisionDate, &acc.SummonerLevel, &acc.ProfileIconID)
	return acc
}

// Load account from database using ID.
func loadAccountByID(id string) (acc Account) {
	_ = db.QueryRow(ctx, "select nick, id, revision_date, summoner_level, profile_icon from account where id = $1 ", id).Scan(&acc.Name, &acc.ID, &acc.RevisionDate, &acc.SummonerLevel, &acc.ProfileIconID)
	return acc
}

// Load account data and rank from database using ID.
func loadAccountAndRankByID(id string) (acc AccountRank) {
	err := db.QueryRow(ctx, "select acc.id, acc.nick, ranks.ico, r.tier, r.sub_tier, r.lp, r.wins, r.losses from rank_solo as r join account as acc on acc.id = r.summoner join ranks on ranks.tier = r.tier where summoner = $1", id).Scan(&acc.SummonerID, &acc.Name, &acc.Icon, &acc.Tier, &acc.SubTier, &acc.LeaguePoints, &acc.Wins, &acc.Losses)
	if err != nil {
		log.Printf("Failed to retrieve account and rank by ID: %s", err)
	}
	return acc
}

// Load the rank of an account from the database.
func loadAccountRank(rank_queue, acc_id string) (r AccountRank) {
	_ = db.QueryRow(ctx, "select ranks.ico, r.tier, r.sub_tier, r.lp, r.wins, r.losses, r.hot_streak, r.s_target, r.s_wins, r.s_losses from "+rank_queue+" as r join account as acc on acc.id = r.summoner join ranks on ranks.tier = r.tier where summoner = $1", acc_id).Scan(&r.Icon, &r.Tier, &r.SubTier, &r.LeaguePoints, &r.Wins, &r.Losses, &r.HotStreak, &r.MiniSeries.Target, &r.MiniSeries.Wins, &r.MiniSeries.Losses)
	return r
}

// Load the ranked leaderboard of a Discord server.
func loadLeaderboardRank(channel string) (all []AccountRank) {
	var r AccountRank
	resp, err := db.Query(ctx, "select acc.nick, ranks.ico, r.tier, r.sub_tier, r.lp, r.wins, r.losses from track_account as t join account as acc on acc.id = t.summoner join rank_solo as r on r.summoner = t.summoner join ranks on ranks.tier = r.tier where channel = $1 order by r.mmr desc limit 20", channel)
	if err != nil {
		log.Printf("Failed to retrieve account rank: %s", err)
	}
	defer resp.Close()

	for resp.Next() {
		err = resp.Scan(&r.Name, &r.Icon, &r.Tier, &r.SubTier, &r.LeaguePoints, &r.Wins, &r.Losses)
		if err != nil {
			log.Printf("Failed to scan the account rank: %s", err)
		}
		all = append(all, r)
	}
	return all
}

// Load the mastery of the champion's account from the database.
func loadMastery(summoner string) (m []Mastery) {
	resp, err := db.Query(ctx, "select champion from mastery where summoner = $1", summoner)
	if err != nil {
		log.Printf("Failed to query mastery: %s", err)
	}
	defer resp.Close()

	for resp.Next() {
		err = resp.Scan(&m)
		if err != nil {
			log.Printf("Failed to scan mastery: %s", err)
		}
	}
	return m
}

/*
*	Commands used to remove information from the database.
 */

// Remove the tracked account from the database.
func removeTrackedAccount(channel, account string) {
	_, err := db.Exec(ctx, "delete from track_account where channel = $1 and summoner = $2", channel, account)
	if err != nil {
		log.Printf("Failed to remove account from tracked table: %s", err)
	}
}

// Remove a Discord server from the database.
func DisableTrackOnServer(channel string) {
	_, err := db.Exec(ctx, "delete from track_account where channel = $1", channel)
	if err != nil {
		log.Printf("Failed to remove channel from tracked table: %s", err)
	}
	_, err = db.Exec(ctx, "delete from discord where channel = $1", channel)
	if err != nil {
		log.Printf("Failed to remove Discord server from database: %s", err)
	}
	_, err = db.Exec(ctx, "delete from livegame where channel = $1", channel)
	if err != nil {
		log.Printf("Failed to remove tracked live game from database: %s", err)
	}
}

// Remove a live game from the database.
func (lg LiveGame) Remove(summoner string) {
	_, err := db.Exec(ctx, "delete from livegame where gameid = $1 and channel = $2 and summoner = $3", lg.GameID, lg.ChannelID, summoner)
	if err != nil {
		log.Printf("Failed to remove live game data: %s", err)
	}
}

// Clear the live game table.
func (lg LiveGame) clear() {
	_, err := db.Exec(ctx, "delete from livegame where updated < now() - interval '1 HOUR'")
	if err != nil {
		log.Printf("Failed to clean the live table: %s", err)
	}
}

func removeAccount() {
	var ids []string
	var id string
	resp, err := db.Query(ctx, "select id from account where updated < now() - interval '30 DAYS'")
	if err != nil {
		log.Printf("Querying unused accounts: %s", err)
	}
	defer resp.Close()
	for resp.Next() {
		err = resp.Scan(&id)
		if err != nil {
			log.Printf("Failed to scan unused accounts: %s", err)
		}
		ids = append(ids, id)
	}
	for i := range ids {
		_, err := db.Exec(ctx, "delete from rank_flex where summoner = $1", ids[i])
		if err != nil {
			log.Printf("Failed to remove unused account flex rank: %s", err)
		}
		_, err = db.Exec(ctx, "delete from rank_solo where summoner = $1", ids[i])
		if err != nil {
			log.Printf("Failed to remove unused account solo rank: %s", err)
		}
		_, err = db.Exec(ctx, "delete from mastery where summoner = $1", ids[i])
		if err != nil {
			log.Printf("Failed to remove unused account champion mastery: %s", err)
		}
		_, err = db.Exec(ctx, "delete from track_account where summoner = $1", ids[i])
		if err != nil {
			log.Printf("Failed to remove account from tracked table: %s", err)
		}
		_, err = db.Exec(ctx, "delete from account where id = $1", ids[i])
		if err != nil {
			log.Printf("Failed to remove unused account: %s", err)
		}
	}
}
