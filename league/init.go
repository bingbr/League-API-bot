package league

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/valyala/fasthttp"
)

var (
	tokenRiot          = "YOUR_TOKEN"
	client             *fasthttp.Client
	acc                Account
	accRank, accTFT    AccountRanking
	so, fl, tf, re, er string
)

func fetchInfo(region, path, summoner string, run *sync.WaitGroup) {
	defer run.Done()
	readTimeout, _ := time.ParseDuration("10s")
	writeTimeout, _ := time.ParseDuration("10s")
	maxIdleConnDuration, _ := time.ParseDuration("10s")
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

	switch path {
	case "lol/summoner/v4/summoners/":
		req.SetRequestURI("https://" + region + ".api.riotgames.com/" + path + "by-name/" + summoner)
	default:
		req.SetRequestURI("https://" + region + ".api.riotgames.com/" + path + "by-summoner/" + summoner)
	}

	switch er {
	case "Fail", "Wait":
		fasthttp.ReleaseRequest(req)
	default:
		req.Header.Add("Content-Type", "application/json")
		req.Header.Add("X-Riot-Token", tokenRiot)
		resp := fasthttp.AcquireResponse()
		err := client.Do(req, resp)
		fasthttp.ReleaseRequest(req)
		if err == nil {
			switch path {
			case "lol/summoner/v4/summoners/":
				switch resp.Header.StatusCode() {
				case 404:
					er = "Fail"
					fasthttp.ReleaseResponse(resp)
				case 429:
					er = "Wait"
					fasthttp.ReleaseResponse(resp)
				default:
					json.Unmarshal(resp.Body(), &acc)
					fasthttp.ReleaseResponse(resp)
				}
			case "tft/league/v1/entries/":
				json.Unmarshal(resp.Body(), &accTFT)
				fasthttp.ReleaseResponse(resp)
				run.Done()
			default:
				json.Unmarshal(resp.Body(), &accRank)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Erro de conexão: %v\n", err)
		}
		fasthttp.ReleaseResponse(resp)
	}
}

func aboutRanking(rank AccountRanking) {
	for i := range rank {
		switch rank[i].QueueType {
		case "RANKED_FLEX_SR":
			fl = accRank.RankOutput(i, "Flex")
		case "RANKED_SOLO_5x5":
			so = accRank.RankOutput(i, "Solo")
		case "RANKED_TFT":
			tf = accTFT.RankOutput(i, "TFT")
		}
	}
}

func AccInfo(region, user, nick string) {
	var run sync.WaitGroup
	go fetchInfo(region, "lol/summoner/v4/summoners/", nick, &run)
	re = region
	run.Add(1)
	run.Wait()
	if er == "Fail" || er == "Wait" {
		log.Printf("%v pesquisou um nick inválido.", user)
	} else {
		log.Printf("%v pesquisou %q ID:%v.", user, acc.Name, acc.ID)
	}
}

func CleanAll() {
	so = ""
	fl = ""
	tf = ""
	er = ""
}

func AboutAcc() string {
	var run sync.WaitGroup
	switch er {
	case "Fail":
		return "Account not found."
	case "Wait":
		return "Too many requests, please try again in 2 minutes."
	default:
		go fetchInfo(re, "lol/league/v4/entries/", acc.ID, &run)
		go fetchInfo(re, "tft/league/v1/entries/", acc.ID, &run)
		run.Add(3)
		run.Wait()
		aboutRanking(accRank)
		aboutRanking(accTFT)
		return fmt.Sprintf("**Nick**: %v    **Level**: %d\n**Last Modified**: %s %v",
			acc.Name, acc.SummonerLevel, time.UnixMilli(acc.RevisionDate).Format("02/01/2006 15:04:05"), fmt.Sprintf("\n%s \n%s \n%s", so, fl, tf))
	}
}
