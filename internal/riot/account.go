package riot

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const defaultRiotUserAgent = "League-API-bot/2.0"

var validPlatformRegions = map[string]struct{}{
	"br1": {}, "eun1": {}, "euw1": {}, "jp1": {}, "kr": {},
	"la1": {}, "la2": {}, "na1": {}, "oc1": {}, "pbe1": {},
	"ph2": {}, "ru": {}, "sg2": {}, "th2": {}, "tr1": {},
	"tw2": {}, "vn2": {},
}

type RiotAccount struct {
	PUUID    string `json:"puuid"`
	GameName string `json:"gameName"`
	TagLine  string `json:"tagLine"`
}

type SummonerProfile struct {
	PUUID         string `json:"puuid"`
	ProfileIconID int    `json:"profileIconId"`
	RevisionDate  int64  `json:"revisionDate"`
	SummonerLevel int64  `json:"summonerLevel"`
}

type LeagueEntry struct {
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Rank         string `json:"rank"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
}

type ChampionRotation struct {
	FreeChampionIDs              []int `json:"freeChampionIds"`
	FreeChampionIDsForNewPlayers []int `json:"freeChampionIdsForNewPlayers"`
	MaxNewPlayerLevel            int   `json:"maxNewPlayerLevel"`
}

type HTTPStatusError struct {
	URL        string
	StatusCode int
	Body       string
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return "riot request failed"
	}
	return fmt.Sprintf("request %s failed: status %d body %q", e.URL, e.StatusCode, e.Body)
}

func IsAccountByRiotIDNotFound(err error) bool {
	statusErr, ok := errors.AsType[*HTTPStatusError](err)
	return ok && statusErr.StatusCode == http.StatusNotFound && strings.Contains(statusErr.URL, "/riot/account/v1/accounts/by-riot-id/")
}

func NormalizePlatformRegion(region string) string {
	region = strings.ToLower(strings.TrimSpace(region))
	if _, ok := validPlatformRegions[region]; ok {
		return region
	}
	return ""
}

func PlatformContinent(region string) string {
	switch NormalizePlatformRegion(region) {
	case "br1", "la1", "la2", "na1", "oc1", "pbe1":
		return "americas"
	case "jp1", "kr":
		return "asia"
	case "euw1", "eun1", "ru", "tr1":
		return "europe"
	case "ph2", "sg2", "th2", "tw2", "vn2":
		return "sea"
	default:
		return ""
	}
}

func FetchAccountByRiotID(ctx context.Context, platformRegion, gameName, tagLine, apiKey string) (RiotAccount, error) {
	gameName = strings.TrimSpace(gameName)
	tagLine = strings.TrimPrefix(strings.TrimSpace(tagLine), "#")
	if gameName == "" || tagLine == "" {
		return RiotAccount{}, fmt.Errorf("game name and tag line are required")
	}
	continent := PlatformContinent(NormalizePlatformRegion(platformRegion))
	if continent == "" {
		return RiotAccount{}, fmt.Errorf("unsupported platform region %q", platformRegion)
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/riot/account/v1/accounts/by-riot-id/%s/%s",
		continent, url.PathEscape(gameName), url.PathEscape(tagLine))
	var account RiotAccount
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &account); err != nil {
		return RiotAccount{}, fmt.Errorf("fetch account by riot id: %w", err)
	}
	return account, nil
}

func FetchSummonerByPUUID(ctx context.Context, platformRegion, puuid, apiKey string) (SummonerProfile, error) {
	region, err := requirePlatformRegion(platformRegion)
	if err != nil {
		return SummonerProfile{}, err
	}
	puuid, err = requireNonEmpty("puuid", puuid)
	if err != nil {
		return SummonerProfile{}, err
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/lol/summoner/v4/summoners/by-puuid/%s", region, url.PathEscape(puuid))
	var profile SummonerProfile
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &profile); err != nil {
		return SummonerProfile{}, fmt.Errorf("fetch summoner by puuid: %w", err)
	}
	return profile, nil
}

func FetchLeagueEntriesByPUUID(ctx context.Context, platformRegion, puuid, apiKey string) ([]LeagueEntry, error) {
	region, err := requirePlatformRegion(platformRegion)
	if err != nil {
		return nil, err
	}
	puuid, err = requireNonEmpty("puuid", puuid)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/by-puuid/%s", region, url.PathEscape(puuid))
	var entries []LeagueEntry
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &entries); err != nil {
		return nil, fmt.Errorf("fetch league entries by puuid: %w", err)
	}
	return entries, nil
}

func FetchChampionRotation(ctx context.Context, platformRegion, apiKey string) (ChampionRotation, error) {
	region, err := requirePlatformRegion(platformRegion)
	if err != nil {
		return ChampionRotation{}, err
	}

	endpoint := fmt.Sprintf("https://%s.api.riotgames.com/lol/platform/v3/champion-rotations", region)
	var rotation ChampionRotation
	if err := doRiotJSONWithRetry(ctx, endpoint, apiKey, &rotation); err != nil {
		return ChampionRotation{}, fmt.Errorf("fetch champion rotations: %w", err)
	}
	return rotation, nil
}

func requirePlatformRegion(platformRegion string) (string, error) {
	if region := NormalizePlatformRegion(platformRegion); region != "" {
		return region, nil
	}
	return "", fmt.Errorf("platform region is required")
}

func requireNonEmpty(name, value string) (string, error) {
	if value = strings.TrimSpace(value); value != "" {
		return value, nil
	}
	return "", fmt.Errorf("%s is required", name)
}

func NormalizeRankTier(tier string) string {
	return strings.ToLower(strings.TrimSpace(tier))
}

var (
	ErrRiotIDRequired = errors.New("riot id is required")
	ErrInvalidRiotID  = errors.New("riot id must be in format nickname#tagline")
)

func SplitRiotID(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", ErrRiotIDRequired
	}
	idx := strings.LastIndex(raw, "#")
	if idx <= 0 || idx >= len(raw)-1 {
		return "", "", ErrInvalidRiotID
	}
	gameName := strings.TrimSpace(raw[:idx])
	tagLine := strings.TrimSpace(raw[idx+1:])
	if gameName == "" || tagLine == "" {
		return "", "", ErrInvalidRiotID
	}
	return gameName, tagLine, nil
}

func FormatRiotID(gameName, tagLine string) string {
	return fmt.Sprintf("%s#%s", strings.TrimSpace(gameName), strings.TrimPrefix(strings.TrimSpace(tagLine), "#"))
}
