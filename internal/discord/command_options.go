package discord

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/bingbr/League-API-bot/data"
	"github.com/bwmarrin/discordgo"
)

var regions = loadRegions()

type Region struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

func loadRegions() []Region {
	var loaded []Region
	if err := json.Unmarshal(data.RegionsJSON, &loaded); err != nil {
		panic(fmt.Sprintf("load region choices: %v", err))
	}
	if len(loaded) == 0 {
		panic("load region choices: no regions configured")
	}
	for i := range loaded {
		loaded[i].Name = strings.TrimSpace(loaded[i].Name)
		loaded[i].Value = strings.ToLower(strings.TrimSpace(loaded[i].Value))
		if loaded[i].Name == "" || loaded[i].Value == "" {
			panic(fmt.Sprintf("load region choices: invalid region at index %d", i))
		}
	}
	return loaded
}

func Regions() []Region {
	return slices.Clone(regions)
}

func regionOptionChoices() []*discordgo.ApplicationCommandOptionChoice {
	regions := Regions()
	choices := make([]*discordgo.ApplicationCommandOptionChoice, 0, len(regions))
	for _, region := range regions {
		choice := &discordgo.ApplicationCommandOptionChoice{
			Name:  region.Name,
			Value: region.Value,
		}
		choices = append(choices, choice)
	}
	return choices
}

var RegionOption = &discordgo.ApplicationCommandOption{
	Type:        discordgo.ApplicationCommandOptionString,
	Name:        "region",
	Description: "Select account region.",
	Choices:     regionOptionChoices(),
	Required:    true,
}

var NickInputOption = &discordgo.ApplicationCommandOption{
	Type:         discordgo.ApplicationCommandOptionString,
	Name:         "nick",
	Description:  "Insert account in format nickname#tagline.",
	Autocomplete: false,
	Required:     true,
	MinLength:    new(7),
	MaxLength:    22,
}

func AccountTargetOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		RegionOption,
		NickInputOption,
	}
}
