package league

import (
	"encoding/json"
	"strconv"
)

type RiotAPI struct {
	N struct {
		Item        string `json:"item"`
		Rune        string `json:"rune"`
		Mastery     string `json:"mastery"`
		Summoner    string `json:"summoner"`
		Champion    string `json:"champion"`
		Profileicon string `json:"profileicon"`
		Map         string `json:"map"`
		Language    string `json:"language"`
		Sticker     string `json:"sticker"`
	} `json:"n"`
	Version string `json:"v"`
	Cdn     string `json:"cdn"`
}

type Data struct {
	DataItem map[string]Item `json:"data"`
}

type Item struct {
	Icon     string    `json:"ico"`
	Name     string    `json:"id"`
	ID       StringInt `json:"key"`
	FullName string    `json:"name"`
	Image    struct {
		Full string `json:"full"`
	} `json:"image"`
}

type Rune struct {
	Icon  string `json:"ico"`
	Name  string `json:"name"`
	ID    int    `json:"id"`
	Slots []struct {
		Runes []Rune `json:"runes"`
	} `json:"slots"`
}

type StringInt int

func (st *StringInt) UnmarshalJSON(b []byte) error {
	var item interface{}
	if err := json.Unmarshal(b, &item); err != nil {
		return err
	}
	switch v := item.(type) {
	case int:
		*st = StringInt(v)
	case float64:
		*st = StringInt(int(v))
	case string:
		i, err := strconv.Atoi(v)
		if err != nil {
			return err
		}
		*st = StringInt(i)
	}
	return nil
}

type FreeChampion struct {
	FreeChampionId             []int `json:"freeChampionIds"`
	FreeChampionIdForNewPlayer []int `json:"freeChampionIdsForNewPlayers"`
	MaxNewPlayerLevel          int   `json:"maxNewPlayerLevel"`
}

type Account struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	Puuid         string `json:"puuid"`
	Name          string `json:"name"`
	ProfileIconID int    `json:"profileIconId"`
	RevisionDate  int64  `json:"revisionDate"`
	SummonerLevel int    `json:"summonerLevel"`
	Continent     string
	Region        string
	Channel       string
	TrackID       string
}

type Rank struct {
	Icon string `json:"ico"`
	Tier string `json:"tier"`
}

type AccountRank struct {
	LeagueID     string `json:"leagueId"`
	QueueType    string `json:"queueType"`
	Icon         string
	Tier         string `json:"tier"`
	SubTier      string `json:"rank"`
	Name         string
	SummonerID   string `json:"summonerId"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	HotStreak    bool   `json:"hotStreak"`
	Team         int
	MiniSeries   struct {
		Target   int    `json:"target"`
		Wins     int    `json:"wins"`
		Losses   int    `json:"losses"`
		Progress string `json:"progress"`
	} `json:"miniSeries,omitempty"`
}

type Mastery struct {
	ChampionID     int    `json:"championId"`
	ChampionLevel  int    `json:"championLevel"`
	ChampionPoints int    `json:"championPoints"`
	LastPlayTime   int64  `json:"lastPlayTime"`
	SummonerID     string `json:"summonerId"`
}

type Queue struct {
	QueueID     int    `json:"queueId"`
	Map         string `json:"map"`
	Description string `json:"description"`
}

type LiveGame struct {
	GameID            int              `json:"gameId"`
	GameQueueConfigID int              `json:"gameQueueConfigId"`
	PlatformID        string           `json:"platformId"`
	Participants      []LiveGamePlayer `json:"participants"`
	BannedChampions   []LiveGameBan    `json:"bannedChampions"`
	ChannelID         string
	GuildID           string
	MessageID         string
	ID                string
	On                bool
}

type LiveGamePlayer struct {
	TeamID     int    `json:"teamId"`
	ChampionID int    `json:"championId"`
	SummonerID string `json:"summonerId"`
}

type LiveGameBan struct {
	ChampionID int `json:"championId"`
	TeamID     int `json:"teamId"`
}

type Match struct {
	Metadata struct {
		MatchID string `json:"matchId"`
	} `json:"metadata"`
	Info struct {
		Duration     int64    `json:"gameDuration"`
		Participants []Player `json:"participants"`
		QueueID      int      `json:"queueId"`
		Teams        []struct {
			Bans   []LiveGameBan `json:"bans"`
			TeamID int           `json:"teamId"`
		} `json:"teams"`
	} `json:"info"`
}

type Player struct {
	SummonerID   string    `json:"summonerId"`
	SummonerName string    `json:"summonerName"`
	ChampionId   StringInt `json:"championId"`
	ChampionName string    `json:"championName"`
	Kill         int       `json:"kills"`
	Death        int       `json:"deaths"`
	Assist       int       `json:"assists"`
	Item0        int       `json:"item0"`
	Item1        int       `json:"item1"`
	Item2        int       `json:"item2"`
	Item3        int       `json:"item3"`
	Item4        int       `json:"item4"`
	Item5        int       `json:"item5"`
	Item6        int       `json:"item6"`
	D            int       `json:"summoner1Id"`
	F            int       `json:"summoner2Id"`
	Win          bool      `json:"win"`
	Perks        struct {
		Styles []PostRunes `json:"styles"`
	} `json:"perks"`
}

type PostRunes struct {
	Style      int `json:"style"`
	Selections []struct {
		Perk int `json:"perk"`
	} `json:"selections"`
}

type Server struct {
	GuildID   string
	ChannelID string
}
