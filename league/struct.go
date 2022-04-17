package league

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Account struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	Puuid         string `json:"puuid"`
	Name          string `json:"name"`
	ProfileIconID int    `json:"profileIconId"`
	RevisionDate  int64  `json:"revisionDate"`
	SummonerLevel int    `json:"summonerLevel"`
}

type AccountRanking []struct {
	LeagueID     string `json:"leagueId"`
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Rank         string `json:"rank"`
	SummonerID   string `json:"summonerId"`
	SummonerName string `json:"summonerName"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	Veteran      bool   `json:"veteran"`
	Inactive     bool   `json:"inactive"`
	FreshBlood   bool   `json:"freshBlood"`
	HotStreak    bool   `json:"hotStreak"`
	MiniSeries   struct {
		Target   int    `json:"target"`
		Wins     int    `json:"wins"`
		Losses   int    `json:"losses"`
		Progress string `json:"progress"`
	} `json:"miniSeries,omitempty"`
}

func (rank AccountRanking) rankOutput(i int, q string) string {
	var r string
	wr := float64(rank[i].Wins) * 100 / float64(rank[i].Wins+rank[i].Losses)
	switch rank[i].HotStreak {
	case true:
		if rank[i].MiniSeries.Target >= 1 {
			r = fmt.Sprintf("\n"+q+"\n**Elo**: %v %v %d LP\n**Winrate**: %.2f%%  %dW  %dL   Winning Streak\n**MD5**   %dW   %dL",
				rank[i].Tier, rank[i].Rank, rank[i].LeaguePoints, wr, rank[i].Wins, rank[i].Losses, rank[i].MiniSeries.Wins, rank[i].MiniSeries.Losses)
		} else {
			r = fmt.Sprintf(
				"\n"+q+"\n**Elo**: %v %v %d LP\n**Winrate**: %.2f%%  %dW  %dL   Winning Streak",
				rank[i].Tier, rank[i].Rank, rank[i].LeaguePoints, wr, rank[i].Wins, rank[i].Losses,
			)
		}
	default:
		if rank[i].MiniSeries.Target >= 1 {
			r = fmt.Sprintf("\n"+q+"\n**Elo**: %v %v %d LP\n**Winrate**: %.2f%%  %dW  %dL\n**MD5**   %dW  %dL",
				rank[i].Tier, rank[i].Rank, rank[i].LeaguePoints, wr, rank[i].Wins, rank[i].Losses, rank[i].MiniSeries.Wins, rank[i].MiniSeries.Losses)
		} else {
			r = fmt.Sprintf("\n"+q+"\n**Elo**: %v %v %d LP\n**Winrate**: %.2f%%  %dW  %dL",
				rank[i].Tier, rank[i].Rank, rank[i].LeaguePoints, wr, rank[i].Wins, rank[i].Losses)
		}
	}
	return r
}

type LiveGame struct {
	GameID            int64  `json:"gameId"`
	MapID             int    `json:"mapId"`
	GameMode          string `json:"gameMode"`
	GameType          string `json:"gameType"`
	GameQueueConfigID int    `json:"gameQueueConfigId"`
	Participants      []struct {
		TeamID                   int           `json:"teamId"`
		Spell1ID                 int           `json:"spell1Id"`
		Spell2ID                 int           `json:"spell2Id"`
		ChampionID               int           `json:"championId"`
		ProfileIconID            int           `json:"profileIconId"`
		SummonerName             string        `json:"summonerName"`
		Bot                      bool          `json:"bot"`
		SummonerID               string        `json:"summonerId"`
		GameCustomizationObjects []interface{} `json:"gameCustomizationObjects"`
		Perks                    struct {
			PerkIds      []int `json:"perkIds"`
			PerkStyle    int   `json:"perkStyle"`
			PerkSubStyle int   `json:"perkSubStyle"`
		} `json:"perks"`
	} `json:"participants"`
	Observers struct {
		EncryptionKey string `json:"encryptionKey"`
	} `json:"observers"`
	PlatformID      string `json:"platformId"`
	BannedChampions []struct {
		ChampionID int `json:"championId"`
		TeamID     int `json:"teamId"`
		PickTurn   int `json:"pickTurn"`
	} `json:"bannedChampions"`
	GameStartTime int64 `json:"gameStartTime"`
	GameLength    int   `json:"gameLength"`
}

type AccountChampionStats []struct {
	ChampionID                   int    `json:"championId"`
	ChampionLevel                int    `json:"championLevel"`
	ChampionPoints               int    `json:"championPoints"`
	LastPlayTime                 int64  `json:"lastPlayTime"`
	ChampionPointsSinceLastLevel int    `json:"championPointsSinceLastLevel"`
	ChampionPointsUntilNextLevel int    `json:"championPointsUntilNextLevel"`
	ChestGranted                 bool   `json:"chestGranted"`
	TokensEarned                 int    `json:"tokensEarned"`
	SummonerID                   string `json:"summonerId"`
}

type ChampionList struct {
	Type    string                  `json:"type"`
	Format  string                  `json:"format"`
	Version string                  `json:"version"`
	Data    map[string]ChampionInfo `json:"data"`
}

type ChampionInfo struct {
	Version string `json:"version"`
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Title   string `json:"title"`
	Blurb   string `json:"blurb"`
	Info    struct {
		Attack     int `json:"attack"`
		Defense    int `json:"defense"`
		Magic      int `json:"magic"`
		Difficulty int `json:"difficulty"`
	} `json:"info"`
	Image struct {
		Full   string `json:"full"`
		Sprite string `json:"sprite"`
		Group  string `json:"group"`
		X      int    `json:"x"`
		Y      int    `json:"y"`
		W      int    `json:"w"`
		H      int    `json:"h"`
	} `json:"image"`
	Tags    []string `json:"tags"`
	Partype string   `json:"partype"`
	Stats   struct {
		Hp                   int     `json:"hp"`
		Hpperlevel           int     `json:"hpperlevel"`
		Mp                   int     `json:"mp"`
		Mpperlevel           int     `json:"mpperlevel"`
		Movespeed            int     `json:"movespeed"`
		Armor                int     `json:"armor"`
		Armorperlevel        float64 `json:"armorperlevel"`
		Spellblock           int     `json:"spellblock"`
		Spellblockperlevel   float64 `json:"spellblockperlevel"`
		Attackrange          int     `json:"attackrange"`
		Hpregen              int     `json:"hpregen"`
		Hpregenperlevel      int     `json:"hpregenperlevel"`
		Mpregen              int     `json:"mpregen"`
		Mpregenperlevel      int     `json:"mpregenperlevel"`
		Crit                 int     `json:"crit"`
		Critperlevel         int     `json:"critperlevel"`
		Attackdamage         int     `json:"attackdamage"`
		Attackdamageperlevel int     `json:"attackdamageperlevel"`
		Attackspeedperlevel  float64 `json:"attackspeedperlevel"`
		Attackspeed          float64 `json:"attackspeed"`
	} `json:"stats"`
}

func (cl ChampionList) championsMastery(lvl int, acs AccountChampionStats) []string {
	var geT, resulT []string
	for championName, championInfo := range cl.Data {
		convChampionID, _ := strconv.Atoi(championInfo.Key)
		for _, info := range acs {
			if convChampionID == info.ChampionID {
				switch info.ChampionLevel {
				case lvl:
					switch info.ChestGranted {
					case false:
						geT = append(geT, fmt.Sprintf("**%s** played __%s__ 🎁", championName, time.UnixMilli(info.LastPlayTime).Format("02/01/2006 15:04:05")))
					default:
						geT = append(geT, fmt.Sprintf("**%s** played __%s__ X", championName, time.UnixMilli(info.LastPlayTime).Format("02/01/2006 15:04:05")))
					}
				}
			}
		}
	}
	sort.Strings(geT)
	var j int
	for i := 0; i < len(geT); i += 42 {
		j += 42
		if j > len(geT) {
			j = len(geT)
		}
		resulT = append(resulT, strings.Join(geT[i:j], "\n"))
	}
	return resulT
}
