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
	tokenRiot       = "YOUR_TOKEN"
	client          *fasthttp.Client
	acc             Account
	accRank, accTFT AccountRanking
	champList       ChampionList
	accChamp        AccountChampionStats
	location, er    string
	master          int
)

func httpConfig() {
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
}

func LoadLeagueChampions() {
	httpConfig()
	req := fasthttp.AcquireRequest()
	req.SetRequestURI("https://ddragon.leagueoflegends.com/cdn/12.7.1/data/en_US/champion.json")
	resp := fasthttp.AcquireResponse()
	err := fasthttp.Do(req, resp)
	fasthttp.ReleaseRequest(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Erro de conexão: %v\n", err)
	}
	if resp.StatusCode() != fasthttp.StatusOK {
		log.Printf("Não foi possível baixar os dados, HTTP Status: %d", fasthttp.StatusOK)
	}
	json.Unmarshal(resp.Body(), &champList)
	fasthttp.ReleaseResponse(resp)
}

func fetchInfo(region, path, summoner string, run *sync.WaitGroup) {
	defer run.Done()
	httpConfig()
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
			case "lol/champion-mastery/v4/champion-masteries/":
				json.Unmarshal(resp.Body(), &accChamp)
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

func aboutRanking(rank AccountRanking) (string, string, string) {
	var so, fl, tf string
	for i := range rank {
		switch rank[i].QueueType {
		case "RANKED_FLEX_SR":
			fl = accRank.rankOutput(i, "Flex")
		case "RANKED_SOLO_5x5":
			so = accRank.rankOutput(i, "Solo")
		case "RANKED_TFT":
			tf = accTFT.rankOutput(i, "TFT")
		}
	}
	return so, fl, tf
}

func LoadAccInfo(region, user, nick string, mastery int) {
	var run sync.WaitGroup
	go fetchInfo(region, "lol/summoner/v4/summoners/", nick, &run)
	location = region
	run.Add(1)
	run.Wait()
	master = mastery
	if er == "Fail" || er == "Wait" {
		log.Printf("%v pesquisou um nick inválido.", user)
	} else {
		log.Printf("%v pesquisou %q ID:%v.", user, acc.Name, acc.ID)
	}
}

func FollowupResponse(index int) string {
	return champList.championsMastery(master, accChamp)[index]
}

func Response(t string) (int, string) {
	var run sync.WaitGroup
	switch er {
	case "Fail":
		return 0, "Account not found."
	case "Wait":
		return 0, "Too many requests, please try again in 2 minutes."
	default:
		switch t {
		case "acc":
			go fetchInfo(location, "lol/league/v4/entries/", acc.ID, &run)
			go fetchInfo(location, "tft/league/v1/entries/", acc.ID, &run)
			run.Add(2)
			run.Wait()
			so, fl, _ := aboutRanking(accRank)
			_, _, tf := aboutRanking(accTFT)
			er = ""
			return 0, fmt.Sprintf("**Nick**: %v    **Level**: %d\n**Last Modified**: %s %v",
				acc.Name, acc.SummonerLevel, time.UnixMilli(acc.RevisionDate).Format("02/01/2006 15:04:05"), fmt.Sprintf("\n%s \n%s \n%s", so, fl, tf))
		case "mst":
			go fetchInfo(location, "lol/champion-mastery/v4/champion-masteries/", acc.ID, &run)
			run.Add(2)
			run.Wait()

			legend := "_[Champion Name]_ _[Last Played Date]_ _[Chest Available (🎁 = true | X = false)]_\n"
			arrayList := champList.championsMastery(master, accChamp)
			switch master {
			case 7:
				if len(arrayList) > 1 {
					response := fmt.Sprintf("**%s Mastered**\n%s\n%s\n...", acc.Name, legend, arrayList[0])
					er = ""
					return len(arrayList), response
				} else {
					if len(arrayList) == 0 {
						return 0, "Your account doesn't has mastered champions."
					} else {
						response := fmt.Sprintf("**%s Mastered**\n%s\n%s", acc.Name, legend, arrayList[0])
						er = ""
						return 0, response
					}
				}
			default:
				if len(arrayList) > 1 {
					response := fmt.Sprintf("**%s Mastery %v**\n%s\n%s\n...", acc.Name, master, legend, arrayList[0])
					er = ""
					return len(arrayList), response
				} else {
					if len(arrayList) == 0 {
						return 0, fmt.Sprintf("Your account doesn't has mastery %v champions.", master)
					} else {
						response := fmt.Sprintf("**%s Mastery %v**\n%s\n%s", acc.Name, master, legend, arrayList[0])
						er = ""
						return 0, response
					}
				}
			}
		default:
			return 0, "An error occured."
		}
	}
}
