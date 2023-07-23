package league

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

const userAgent = "League-API-bot/1.2"

// Load the required League of Legends data to use the League API.
func LoadData() {
	var (
		ranks []Rank
		queue []Queue
	)
	fetchCustom("https://ddragon.leagueoflegends.com/realms/na.json", &L)
	LoadLocal(".data/json/rank.json", &ranks)
	insertRank(ranks)
	loadData("champion")
	loadData("summoner")
	loadDataItem()
	loadDataRunes()
	fetchCustom("https://static.developer.riotgames.com/docs/lol/queues.json", &queue)
	insertQueues(queue)
	LoadFreeList()
}

func fetchCustom(url string, data interface{}) {
	res, err := http.Get(url)
	if err != nil {
		log.Fatalln(err)
	}
	err = json.NewDecoder(res.Body).Decode(&data)
	if err != nil {
		log.Fatalln(err)
	}
}

// Send a GET to the Riot API.
func fetchData(region, path, id string, contents interface{}) {
	req, err := http.NewRequest("GET", "https://"+region+".api.riotgames.com/"+path+id, nil)
	if err != nil {
		log.Printf("Error GET: %s", err)
	}
	req.Header = map[string][]string{
		"User-Agent":   {userAgent},
		"X-Riot-Token": {os.Getenv("RIOT_TOKEN")},
	}
	client := &http.Client{
		Timeout: time.Second * 180,
		Transport: &http.Transport{
			IdleConnTimeout:       time.Second * 180,
			ResponseHeaderTimeout: time.Second * 180,
		},
	}
	res, err := client.Do(req)
	if err != nil {
		log.Printf("Error Do: %s", err)
		return
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		switch res.StatusCode {
		case http.StatusForbidden:
			log.Println("Your Riot token has expired.")
		case http.StatusUnauthorized:
			log.Println("Your Riot token is invalid.")
		case http.StatusNotFound:
			// log.Println("404 - request not found.")
		case http.StatusTooManyRequests:
			log.Println("API rate limit exceeded.")
		case http.StatusInternalServerError:
			log.Println("Riot Games API has failed.")
		case http.StatusServiceUnavailable:
			log.Println("Riot Games API is down.")
		default:
			log.Printf("API /%s%s resulted in an error %d.", path, id, res.StatusCode)
		}
	} else {
		err = json.NewDecoder(res.Body).Decode(&contents)
		if err != nil {
			log.Printf("Error Decode: %s", err)
		}
	}
}

// Load game data from CDN.
func loadCdnData(data string, i interface{}) {
	res, _ := http.Get(L.Cdn + "/" + L.Version + "/data/en_US/" + data + ".json")
	err := json.NewDecoder(res.Body).Decode(&i)
	if err != nil {
		log.Printf("Load from %s/%s/data/en_US/%s failed: %s", L.Cdn, L.Version, data, err)
	}
}

// Load from local.
func LoadLocal(path string, i interface{}) {
	data, _ := os.ReadFile(path)
	err := json.Unmarshal(data, &i)
	if err != nil {
		log.Printf("Load from %s failed: %s", path, err)
	}
}
