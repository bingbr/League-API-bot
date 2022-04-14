package league

import (
	"fmt"
	"strings"
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

func betterFormat(num float32) string {
	s := fmt.Sprintf("%.4f", num)
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}

func (rank AccountRanking) RankOutput(i int, q string) string {
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
