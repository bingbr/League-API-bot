package league

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/valyala/fasthttp"
)

var (
	tokenRiot       = "YOUR_TOKEN"
	client          *fasthttp.Client
	acc             Account
	accRanking      AccountRanking
	solo, flex, tft string
	accRegion       string
)

func fetchInfo(region, accID, nickname string) {
	readTimeout, _ := time.ParseDuration("10s")
	writeTimeout, _ := time.ParseDuration("3s")
	maxIdleConnDuration, _ := time.ParseDuration("1h")
	client = &fasthttp.Client{
		ReadTimeout:                   readTimeout,
		WriteTimeout:                  writeTimeout,
		MaxIdleConnDuration:           maxIdleConnDuration,
		NoDefaultUserAgentHeader:      true,
		DisableHeaderNamesNormalizing: true,
		DisablePathNormalizing:        true,
		Dial: (&fasthttp.TCPDialer{
			Concurrency:      4096,
			DNSCacheDuration: time.Hour,
		}).Dial,
	}

	req := fasthttp.AcquireRequest()
	if nickname == "" {
		req.SetRequestURI("https://" + region + ".api.riotgames.com/lol/league/v4/entries/by-summoner/" + accID)
	} else {
		req.SetRequestURI("https://" + region + ".api.riotgames.com/lol/summoner/v4/summoners/by-name/" + nickname)
	}
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("X-Riot-Token", tokenRiot)
	resp := fasthttp.AcquireResponse()
	err := client.Do(req, resp)
	fasthttp.ReleaseRequest(req)
	if err == nil {
		if nickname == "" {
			json.Unmarshal(resp.SwapBody(resp.Body()), &accRanking)
		} else {
			json.Unmarshal(resp.SwapBody(resp.Body()), &acc)
		}
	} else {
		fmt.Fprintf(os.Stderr, "Erro de conexão: %v\n", err)
	}
	fasthttp.ReleaseResponse(resp)
}

func soloQ(i int) {
	winRate := accRanking[i].Wins * 100 / (accRanking[i].Wins + accRanking[i].Losses)
	solo = fmt.Sprintf(
		"\nSoloQ\n**Elo**: %v %v %d LP\n**Winrate**: %v%% **W**: %d **L**: %d",
		accRanking[i].Tier,
		accRanking[i].Rank,
		accRanking[i].LeaguePoints,
		winRate,
		accRanking[i].Wins,
		accRanking[i].Losses,
	)
}

func flexQ(i int) {
	winRate := accRanking[i].Wins * 100 / (accRanking[i].Wins + accRanking[i].Losses)
	flex = fmt.Sprintf(
		"\nFlexQ\n**Elo**: %v %v %d LP\n**Winrate**: %v%% **W**: %d **L**: %d",
		accRanking[i].Tier,
		accRanking[i].Rank,
		accRanking[i].LeaguePoints,
		winRate,
		accRanking[i].Wins,
		accRanking[i].Losses,
	)
}

func tftQ(i int) {
	winRate := accRanking[i].Wins * 100 / (accRanking[i].Wins + accRanking[i].Losses)
	tft = fmt.Sprintf(
		"\nTFT\n**Winrate**: %v%% **W**: %d **L**: %d",
		winRate,
		accRanking[i].Wins,
		accRanking[i].Losses,
	)
}

func aboutRanking() string {
	for i := range accRanking {
		if accRanking[i].QueueType == "RANKED_FLEX_SR" {
			flexQ(i)
		} else if accRanking[i].QueueType == "RANKED_SOLO_5x5" {
			soloQ(i)
		} else {
			tftQ(i)
		}
	}
	return fmt.Sprintf(
		"\n%s \n%s \n%s", solo, flex, tft,
	)
}

func AccInfo(region, id, nickname string) {
	fetchInfo(region, "", nickname)
	accRegion = region
}

func AboutAcc(user string) string {
	fetchInfo(accRegion, acc.ID, "")
	log.Printf("%v pesquisou %q ID:%v", user, acc.Name, acc.ID)
	return fmt.Sprintf("**Nick**: %v    **Level**: %d %s", acc.Name, acc.SummonerLevel, aboutRanking())
}
