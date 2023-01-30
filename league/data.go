package league

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	L    RiotAPI
	lang = language.Und
)

func appendString(response ...string) (desc []string) {
	return append(desc, response...)
}

func appendInt(response ...int) (desc []int) {
	return append(desc, response...)
}

func ChampionsAvailable() ([]string, []string) {
	ffa, fft := loadFreeChampion("ffa"), loadFreeChampion("fft")
	title, champions := []string{" ", "Champions that are available to play without \nneeding to purchase them with RP or Essence."}, []string{ffa, fft}
	return appendString(title...), appendString(champions...)
}

func LoadFreeList() {
	clearTable("fft")
	clearTable("ffa")
	var free FreeChampion
	fetchData("na1", "lol/platform/v3/champion-rotations", "", &free)
	insertChampionFree(free.FreeChampionIdForNewPlayer, "fft")
	insertChampionFree(free.FreeChampionId, "ffa")
}

func CleanCache() {
	removeAccount()
}

func loadDataItem() {
	var data Data
	loadCdnData("item", &data)
	data.insertItens()
}

func loadData(txt string) {
	var data Data
	var dt []Item
	loadCdnData(txt, &data)
	LoadLocal(".data/json/"+txt+".json", &dt)
	data.insertData(txt, dt)
}

func loadDataRunes() {
	var cdn []Rune
	var local []Rune
	LoadLocal(".data/json/runes.json", &local)
	loadCdnData("runesReforged", &cdn)
	insertRunes(cdn, local)
}

func IsNew(region, nickname string) (resp bool) {
	acc := loadAccountByNick(region, nickname)
	if acc.ID == "" {
		resp = true
	}
	return resp
}

func IsTracked(region, nickname, channel string) (resp bool) {
	switch IsNew(region, nickname) {
	case false:
		acc := loadAccountByNick(region, nickname)
		saved_acc := loadTrackedAccount(channel, acc.ID)
		if saved_acc.ID != "" {
			resp = true
		}
	}
	return resp
}

func SearchAccount(region, nickname, class string) (resp bool) {
	if class == "live" {
		var acc Account
		fetchData(region, "lol/summoner/v4/summoners/", nickname, &acc)
		resp = acc.add(region, nickname, class)
	} else {
		var acc Account
		fetchData(region, "lol/summoner/v4/summoners/by-name/", nickname, &acc)
		resp = acc.add(region, nickname, class)
	}
	return resp
}

func (acc Account) add(region, nickname, class string) (resp bool) {
	if acc.ID != "" {
		switch region {
		case "br1", "la1", "la2", "na1", "oc1", "pbe1":
			acc.push(region, "americas", strings.Join(strings.Split(cases.Lower(lang).String(acc.Name), " "), "+"))
		case "jp1", "kr":
			acc.push(region, "asia", strings.Join(strings.Split(cases.Lower(lang).String(acc.Name), " "), "+"))
		case "euw1", "eun1", "ru", "tr1":
			acc.push(region, "europe", strings.Join(strings.Split(cases.Lower(lang).String(acc.Name), " "), "+"))
		case "ph2", "sg2", "th2", "tw2", "vn2":
			acc.push(region, "sea", strings.Join(strings.Split(cases.Lower(lang).String(acc.Name), " "), "+"))
		}
		switch class {
		case "info":
			acc.fetchRank(region)
			go acc.fetchMastery(region)
		case "mastery":
			go acc.fetchRank(region)
			acc.fetchMastery(region)
		case "track":
			go acc.fetchRank(region)
			go acc.fetchMastery(region)
		case "live":
			acc.fetchRank(region)
			acc.fetchMastery(region)
		}
		resp = true
	}
	return resp
}

func TrackAttach(region, compacted_account, channel string) string {
	account := loadAccountByNick(region, compacted_account)
	insertTrack(channel, account.ID)
	return account.Name
}

func TrackRemove(region, compacted_account, channel string) string {
	account := loadAccountByNick(region, compacted_account)
	removeTrack(channel, account.ID)
	return account.Name
}

func TrackConfig(guild, channel string) {
	insertDiscord(guild, channel)
}

func (acc Account) about() []string {
	response := []string{acc.Name, fmt.Sprintf("**Modified**: <t:%d:R> **Level**: %d", (acc.RevisionDate / 1000), acc.SummonerLevel), fmt.Sprintf("%s/%s/img/profileicon/%v.png", L.Cdn, L.N.Profileicon, acc.ProfileIconID)}
	return appendString(response...)
}

func ShowAccountInfo(region, nickname string) ([]string, string, string) {
	acc := loadAccountByNick(region, nickname)
	solo, flex := acc.rank()
	return acc.about(), solo, flex
}

func (acc Account) rank() (solo string, flex string) {
	s, f := loadAccountRank("rank_solo", acc.ID), loadAccountRank("rank_flex", acc.ID)
	if s.SubTier != "" || f.SubTier != "" {
		if f.SubTier != "" {
			flex = f.info()
		}
		if s.SubTier != "" {
			solo = s.info()
		}
	}
	return solo, flex
}

func (r AccountRank) mmr() (res int) {
	switch r.Tier {
	case "IRON":
		res = 0 + r.subTierMmr() + r.LeaguePoints
	case "BRONZE":
		res = 400 + r.subTierMmr() + r.LeaguePoints
	case "SILVER":
		res = 800 + r.subTierMmr() + r.LeaguePoints
	case "GOLD":
		res = 1200 + r.subTierMmr() + r.LeaguePoints
	case "PLATINUM":
		res = 1600 + r.subTierMmr() + r.LeaguePoints
	case "DIAMOND":
		res = 2000 + r.subTierMmr() + r.LeaguePoints
	case "MASTER", "GRANDMASTER", "CHALLENGER":
		res = 2400 + r.subTierMmr() + r.LeaguePoints
	}
	return res
}

func (r AccountRank) subTierMmr() (res int) {
	switch r.SubTier {
	case "IV":
		res = 0
	case "III":
		res = 100
	case "II":
		res = 200
	case "I":
		res = 300
	}
	return res
}

func (acc Account) fetchRank(region string) {
	var rank []AccountRank
	fetchData(region, "lol/league/v4/entries/by-summoner/", acc.ID, &rank)
	if len(rank) != 0 {
		for i := range rank {
			switch rank[i].QueueType {
			case "RANKED_FLEX_SR":
				if len(rank) == 1 {
					rank[i].push("rank_flex")
					r := AccountRank{Tier: "UNRANKED", SummonerID: acc.ID}
					r.push("rank_solo")
				} else {
					rank[i].push("rank_flex")
				}
			case "RANKED_SOLO_5x5":
				rank[i].push("rank_solo")
			}
		}
	} else {
		r := AccountRank{Tier: "UNRANKED", SummonerID: acc.ID}
		r.push("rank_solo")
	}
}

func (r AccountRank) info() (response string) {
	switch r.HotStreak {
	case true:
		if r.MiniSeries.Target >= 1 {
			response = fmt.Sprintf("%s\n%.0f%% %dW %dL\n*Winning Streak*\n**BO5**:  %dW %dL", r.fixedTier(), r.wr(), r.Wins, r.Losses, r.MiniSeries.Wins, r.MiniSeries.Losses)
		} else {
			response = fmt.Sprintf("%s\n%.0f%% %dW %dL\n*Winning Streak*", r.fixedTier(), r.wr(), r.Wins, r.Losses)
		}
	default:
		if r.MiniSeries.Target >= 1 {
			response = fmt.Sprintf("%s\n%.0f%% %dW %dL\n**BO5**:  %dW %dL", r.fixedTier(), r.wr(), r.Wins, r.Losses, r.MiniSeries.Wins, r.MiniSeries.Losses)
		} else {
			response = fmt.Sprintf("%s\n%.0f%% %dW %dL", r.fixedTier(), r.wr(), r.Wins, r.Losses)
		}
	}
	return response
}

func (r AccountRank) wr() float64 {
	return float64(r.Wins) * 100 / float64(r.Wins+r.Losses)
}

func (r AccountRank) fixedTier() (res string) {
	switch r.Tier {
	case "MASTER", "GRANDMASTER", "CHALLENGER":
		res = fmt.Sprintf("%s %s %dLP", r.Icon, cases.Title(lang).String(r.Tier), r.LeaguePoints)
	case "UNRANKED":
		res = "---"
	default:
		res = fmt.Sprintf("%s %s %s %dLP", r.Icon, cases.Title(lang).String(r.Tier), r.SubTier, r.LeaguePoints)
	}
	return res
}

func (r AccountRank) fixedStats() (res string) {
	if r.Wins == 0 && r.Losses == 0 {
		res = "---"
	} else {
		res = fmt.Sprintf("%.0f%% %dW %dL", r.wr(), r.Wins, r.Losses)
	}
	return res
}

func (acc AccountRank) fixedName() (res string) {
	if len([]rune(acc.Name)) > 13 {
		if strings.ToUpper(acc.Name) == acc.Name {
			res = acc.Name[:13]
		} else {
			res = acc.Name[:14]
		}
	} else {
		res = acc.Name
	}
	return res
}

func ShowLeadboard(channel string) (des, res []string, valid bool) {
	var nick, tier, stat []string
	accs := loadLeadboardRank(channel)
	if len(accs) >= 1 {
		for _, acc := range accs {
			nick, tier, stat = append(nick, acc.fixedName()), append(tier, acc.fixedTier()), append(stat, acc.fixedStats())
		}
		des, res, valid = []string{" ", "List of best solo/duo players on this discord server.", ""}, []string{strings.Join(nick, "\n"), strings.Join(tier, "\n"), strings.Join(stat, "\n")}, true
	}
	return appendString(des...), appendString(res...), valid
}

func (acc Account) fetchMastery(region string) {
	var mastery []Mastery
	fetchData(region, "lol/champion-mastery/v4/champion-masteries/by-summoner/", acc.ID, &mastery)
	insertMastery(acc.ID, mastery)
}

func filterMastery(all []Mastery, level int) (filtered []Mastery, total []Mastery) {
	for _, mastery := range all {
		if mastery.ChampionLevel == level {
			total = append(total, mastery)
		}
	}
	if len(total) != 0 {
		if len(total) <= 25 {
			filtered = append(filtered, total...)
		} else {
			for i := 0; i < 25; i++ {
				filtered = append(filtered, total[i])
			}
		}
	}
	return filtered, total
}

func ShowAccountMastery(region, compressed_nick string, level int) ([]string, []string, int) {
	acc := loadAccountByNick(region, compressed_nick)
	total, filtered := filterMastery(loadMastery(acc.ID), level)
	return aboutMastery(acc, total, filtered)
}

func aboutMastery(acc Account, masteries, filtered []Mastery) ([]string, []string, int) {
	var name, points, last []string
	for _, c := range masteries {
		champion := loadChampionByID(c.ChampionID)
		name, points, last = append(name, fmt.Sprintf("%s %s", champion.Icon, champion.FullName)), append(points, fmt.Sprintf("%d", c.ChampionPoints)), append(last, fmt.Sprintf("<t:%d:R>", (c.LastPlayTime/1000)))
	}
	return appendString(acc.Name, fmt.Sprintf("List of champions with desired mastery.\nShowing %v of %v.", len(masteries), len(filtered)), fmt.Sprintf("%s/%s/img/profileicon/%v.png", L.Cdn, L.N.Profileicon, acc.ProfileIconID)), appendString(strings.Join(name, "\n"), strings.Join(points, "\n"), strings.Join(last, "\n")), len(name)
}

func IsGameLive() (liv, pos bool, su []string) {
	accs := loadAllTrackedAccounts()
	for _, acc := range accs {
		var api LiveGame
		fetchData(acc.Region, "lol/spectator/v4/active-games/by-summoner/", acc.ID, &api)
		switch api.GameID {
		case 0:
			_, db := loadLiveGame(acc.Channel, acc.ID)
			if !(db.GameID == 0) {
				removeLiveGame(db.GameID, db.Channel)
				pos, su = true, []string{db.Channel, db.PlatformID, acc.ID, acc.Continent, fmt.Sprintf("%s_%d", db.PlatformID, db.GameID)}
			}
		default:
			_, db := loadLiveGame(acc.Channel, acc.ID)
			if !(db.GameID == api.GameID) {
				api.insert(acc.Channel, acc.ID)
				liv, su = true, []string{acc.Channel, acc.Region, acc.ID, acc.Continent}
			}
		}
	}
	return liv, pos, appendString(su...)
}

func (lg LiveGame) data(t int) (resp []string) {
	var team, tier, rank, b []string
	for _, p := range lg.Participants {
		if p.TeamID == t {
			team, tier, rank = append(team, fmt.Sprintf("%s %s", loadChampionByID(p.ChampionID).Icon, loadAccountAndRankByID(p.SummonerID).fixedName())), append(tier, loadAccountAndRankByID(p.SummonerID).fixedTier()), append(rank, loadAccountAndRankByID(p.SummonerID).fixedStats())
		}
	}
	resp = []string{strings.Join(team, "\n"), strings.Join(tier, "\n"), strings.Join(rank, "\n")}
	resp = appendString(resp...)
	if len(lg.BannedChampions) > 1 {
		for _, ban := range lg.BannedChampions {
			if ban.TeamID == t {
				b = append(b, loadChampionByID(ban.ChampionID).Icon)
			}
		}
		resp = append(resp, strings.Join(b, " "))
	}
	return resp
}

func banFormated(bans []LiveGameBan) (b []string) {
	for _, ban := range bans {
		b = append(b, loadChampionByID(ban.ChampionID).Icon)
	}
	return b
}

func ShowLiveGame(region, id string) (des, red, blu []string) {
	var live LiveGame
	fetchData(region, "lol/spectator/v4/active-games/by-summoner/", id, &live)
	for _, player := range live.Participants {
		SearchAccount(region, player.SummonerID, "live")
	}
	blu, red = live.data(100), live.data(200)
	a, q := loadAccountByID(id), loadQueueByID(live.GameQueueConfigID)
	des = []string{a.Name, fmt.Sprintf("Is playing %s on %s.", q.formated(), q.Map), fmt.Sprintf("%s/%s/img/profileicon/%v.png", L.Cdn, L.N.Profileicon, a.ProfileIconID)}
	return appendString(des...), appendString(red...), appendString(blu...)
}

func build(player Player) string {
	return fmt.Sprintf("%s - %s - %s\n%s - %s - %s\n", loadItemByID(player.Item0), loadItemByID(player.Item1), loadItemByID(player.Item2), loadItemByID(player.Item3), loadItemByID(player.Item4), loadItemByID(player.Item5))
}

func (player Player) runes(i int) string {
	r := loadRuneByID(i)
	return fmt.Sprintf("%s %s", r.Icon, r.Name)
}

func runesFormated(player Player) (p, p_formated, s, s_formated string) {
	p_formated, s_formated = fmt.Sprintf("%s\n%s\n%s", player.runes(player.Perks.Styles[0].Selections[0].Perk), player.runes(player.Perks.Styles[0].Selections[1].Perk), player.runes(player.Perks.Styles[0].Selections[2].Perk)), fmt.Sprintf("%s\n%s", player.runes(player.Perks.Styles[1].Selections[0].Perk), player.runes(player.Perks.Styles[1].Selections[1].Perk))
	return player.runes(player.Perks.Styles[0].Style), p_formated, player.runes(player.Perks.Styles[1].Style), s_formated
}

func (q Queue) formated() string {
	return strings.TrimSuffix(strings.TrimPrefix(q.Description, "5v5 "), " games")
}

func (player Player) data() (data []string, aes []int) {
	var champ, kda, sum, p, p1, s, s1, bui string
	champ, sum, bui = loadChampionByID(int(player.ChampionId)).Icon+" "+player.ChampionName, fmt.Sprintf("%s %s", loadSummonerByID(player.D).Icon, loadSummonerByID(player.F).Icon), build(player)
	p, p1, s, s1 = runesFormated(player)
	if player.Death == 0 {
		kda = fmt.Sprintf("%d/%d/%d\nPerfect KDA", player.Kill, player.Death, player.Assist)
	} else {
		kda = fmt.Sprintf("%d/%d/%d\n%.2f:1", player.Kill, player.Death, player.Assist, ((float64(player.Kill) + float64(player.Assist)) / float64(player.Death)))
	}
	switch player.Win {
	case true:
		aes = []int{0x008000, 4069}
	default:
		aes = []int{0xCC0000, 3367}
	}
	data = []string{champ, kda, sum, p, p1, s, s1, bui}
	return appendString(data...), appendInt(aes...)
}

func ShowPostGame(continent, region, match, id string) (des, data, ban []string, aes []int) {
	var game Match
	var acc Account
	var v, blue, red string
	fetchData(continent, "lol/match/v5/matches/", match, &game)
	for _, player := range game.Info.Participants {
		if player.Summoner == id {
			SearchAccount(region, player.Summoner, "live")
			acc = loadAccountByID(player.Summoner)
			data, aes = player.data()
			switch player.Win {
			case true:
				v = "Won"
			case false:
				v = "Lost"
			}
		}
	}
	for _, team := range game.Info.Teams {
		if len(team.Bans) > 1 {
			if team.TeamID == 100 {
				blue = strings.Join(banFormated(team.Bans), " ")
			} else {
				red = strings.Join(banFormated(team.Bans), " ")
			}
		}
	}

	ban = []string{blue, red}
	duration, _ := time.ParseDuration(fmt.Sprintf("%ds", game.Info.Duration))
	des = []string{acc.Name, fmt.Sprintf("%s a %s game.\n\n**Match Duration**: %s", v, loadQueueByID(game.Info.QueueID).formated(), duration.String()), fmt.Sprintf("%s/%s/img/profileicon/%v.png", L.Cdn, L.N.Profileicon, acc.ProfileIconID)}
	return appendString(des...), appendString(data...), appendString(ban...), appendInt(aes...)
}
