package riot

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type LiveGame struct {
	GameID            int64            `json:"gameId"`
	MapID             int              `json:"mapId"`
	GameMode          string           `json:"gameMode"`
	GameType          string           `json:"gameType"`
	GameQueueConfigID int              `json:"gameQueueConfigId"`
	Players           []LiveGamePlayer `json:"participants"`
	PlatformID        string           `json:"platformId"`
	BannedChampions   []LiveGameBan    `json:"bannedChampions"`
	GameStartTime     int64            `json:"gameStartTime"`
	GameLength        int64            `json:"gameLength"`
}

type LiveGamePlayer struct {
	PUUID         string `json:"puuid"`
	TeamID        int    `json:"teamId"`
	Spell1ID      int    `json:"spell1Id"`
	Spell2ID      int    `json:"spell2Id"`
	ChampionID    int    `json:"championId"`
	ProfileIconID int    `json:"profileIconId"`
	RiotID        string `json:"riotId"`
	Bot           bool   `json:"bot"`
}

type LiveGameBan struct {
	ChampionID int `json:"championId"`
	TeamID     int `json:"teamId"`
	PickTurn   int `json:"pickTurn"`
}

type MatchDetail struct {
	Metadata MatchMetadata `json:"metadata"`
	Info     MatchInfo     `json:"info"`
}

type MatchMetadata struct {
	MatchID string   `json:"matchId"`
	Players []string `json:"participants"`
}

type MatchInfo struct {
	GameDuration       int64         `json:"gameDuration"`
	GameStartTimestamp int64         `json:"gameStartTimestamp"`
	GameEndTimestamp   int64         `json:"gameEndTimestamp"`
	QueueID            int           `json:"queueId"`
	Players            []MatchPlayer `json:"participants"`
	Teams              []MatchTeam   `json:"teams"`
}

type MatchPlayer struct {
	PUUID          string     `json:"puuid"`
	RiotIDGameName string     `json:"riotIdGameName"`
	RiotIDTagline  string     `json:"riotIdTagline"`
	SummonerName   string     `json:"summonerName"`
	ProfileIconID  int        `json:"profileIcon"`
	TeamID         int        `json:"teamId"`
	Win            bool       `json:"win"`
	ChampionID     int        `json:"championId"`
	Kills          int        `json:"kills"`
	Deaths         int        `json:"deaths"`
	Assists        int        `json:"assists"`
	Summoner1ID    int        `json:"summoner1Id"`
	Summoner2ID    int        `json:"summoner2Id"`
	Item0          int        `json:"item0"`
	Item1          int        `json:"item1"`
	Item2          int        `json:"item2"`
	Item3          int        `json:"item3"`
	Item4          int        `json:"item4"`
	Item5          int        `json:"item5"`
	Item6          int        `json:"item6"`
	Perks          MatchPerks `json:"perks"`
}

type MatchPerks struct {
	Styles []MatchPerkStyle `json:"styles"`
}

type MatchPerkStyle struct {
	Description string               `json:"description"`
	Style       int                  `json:"style"`
	Selections  []MatchPerkSelection `json:"selections"`
}

type MatchPerkSelection struct {
	Perk int `json:"perk"`
}

type MatchTeam struct {
	TeamID int        `json:"teamId"`
	Win    bool       `json:"win"`
	Bans   []MatchBan `json:"bans"`
}

type MatchBan struct {
	ChampionID int `json:"championId"`
	PickTurn   int `json:"pickTurn"`
}

func FetchActiveGameBySummoner(ctx context.Context, platformRegion, puuid, apiKey string) (LiveGame, error) {
	region, err := requirePlatformRegion(platformRegion)
	if err != nil {
		return LiveGame{}, err
	}
	puuid, err = requireNonEmpty("puuid", puuid)
	if err != nil {
		return LiveGame{}, err
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/lol/spectator/v5/active-games/by-summoner/%s",
		strings.ToLower(region), url.PathEscape(puuid))
	var game LiveGame
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &game); err != nil {
		return LiveGame{}, fmt.Errorf("fetch active game by summoner: %w", err)
	}
	return game, nil
}

func FetchMatchByID(ctx context.Context, continent, matchID, apiKey string) (MatchDetail, error) {
	continent, err := requireNonEmpty("continent", continent)
	if err != nil {
		return MatchDetail{}, err
	}
	matchID, err = requireNonEmpty("match id", matchID)
	if err != nil {
		return MatchDetail{}, err
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/lol/match/v5/matches/%s",
		strings.ToLower(continent), url.PathEscape(matchID))
	var match MatchDetail
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &match); err != nil {
		return MatchDetail{}, fmt.Errorf("fetch match by id: %w", err)
	}
	return match, nil
}

func BuildMatchID(platformID string, gameID int64) string {
	platformID = strings.ToUpper(strings.TrimSpace(platformID))
	if platformID == "" {
		return strconv.FormatInt(gameID, 10)
	}
	return fmt.Sprintf("%s_%d", platformID, gameID)
}
