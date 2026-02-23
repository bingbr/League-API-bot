package cdn

import "encoding/json"

type Queue struct {
	QueueID            int    `json:"id"`
	Name               string `json:"name"`
	GameSelectCategory string `json:"gameSelectCategory"`
}

type GameMap struct {
	MapID int    `json:"id"`
	Name  string `json:"name"`
}

type ChampionList struct {
	Type    string              `json:"type"`
	Format  string              `json:"format"`
	Version string              `json:"version"`
	Data    map[string]Champion `json:"data"`
}

type Champion struct {
	Version string          `json:"version"`
	ID      string          `json:"id"`
	Key     string          `json:"key"`
	Name    string          `json:"name"`
	Title   string          `json:"title"`
	Blurb   string          `json:"blurb"`
	Info    json.RawMessage `json:"info"`
	Image   Image           `json:"image"`
	Tags    []string        `json:"tags"`
	Partype string          `json:"partype"`
	Stats   json.RawMessage `json:"stats"`
}

type ItemList struct {
	Type    string          `json:"type"`
	Version string          `json:"version"`
	Data    map[string]Item `json:"data"`
	Basic   json.RawMessage `json:"basic"`
}

type Item struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Plaintext   string          `json:"plaintext"`
	Into        []string        `json:"into"`
	Image       json.RawMessage `json:"image"`
	Gold        json.RawMessage `json:"gold"`
	Tags        []string        `json:"tags"`
	Maps        json.RawMessage `json:"maps"`
	Stats       json.RawMessage `json:"stats"`
}

type SummonerSpellList struct {
	Type    string                   `json:"type"`
	Version string                   `json:"version"`
	Data    map[string]SummonerSpell `json:"data"`
}

type SummonerSpell struct {
	ID      string `json:"id"`
	Key     string `json:"key"`
	Name    string `json:"name"`
	Image   Image  `json:"image"`
	Tooltip string `json:"tooltip"`
}

type RuneTree struct {
	ID    int        `json:"id"`
	Key   string     `json:"key"`
	Icon  string     `json:"icon"`
	Name  string     `json:"name"`
	Slots []RuneSlot `json:"slots"`
}

type RuneSlot struct {
	Runes []Rune `json:"runes"`
}

type Rune struct {
	ID        int    `json:"id"`
	Key       string `json:"key"`
	Icon      string `json:"icon"`
	Name      string `json:"name"`
	ShortDesc string `json:"shortDesc"`
	LongDesc  string `json:"longDesc"`
}

type Image struct {
	Full string `json:"full"`
}
