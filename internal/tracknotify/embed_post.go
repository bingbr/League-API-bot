package tracknotify

import (
	"context"
	"fmt"
	"strings"

	"github.com/bingbr/League-API-bot/internal/discord"
	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/riot/cdn"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
	"github.com/bwmarrin/discordgo"
)

func (s *Service) buildPostEmbed(ctx context.Context, notification postgres.TrackMatchNotification, match riot.MatchDetail, queueName string) (*discordgo.MessageEmbed, error) {
	player := findMatchPlayer(match.Info.Players, notification.PlayerPUUID, notification.PlayerRiotID)
	if player == nil {
		return nil, fmt.Errorf("player not found")
	}
	playerProfileIconURL := ""
	if player.ProfileIconID > 0 {
		playerProfileIconURL = cdn.ProfileIconURL(player.ProfileIconID)
	}
	playerTitle := strings.TrimSpace(notification.PlayerRiotID)
	if playerTitle == "" {
		playerTitle = matchPlayerRiotID(*player)
	}

	queueName = strings.TrimSpace(queueName)
	if queueName == "" {
		return nil, errQueueNameUnavailable
	}

	championIDs := collectPostChampionIDs(*player, match.Info.Teams)
	champions := loadOrEmptyMap(func() (map[int]postgres.ChampionDisplay, error) {
		return s.database.ChampionDisplayByIDs(ctx, championIDs)
	})

	spells := loadOrEmptyMap(func() (map[int]postgres.SummonerSpellDisplay, error) {
		return s.database.SummonerSpellDisplayByIDs(ctx, []int{player.Summoner1ID, player.Summoner2ID})
	})

	primaryStyle, secondaryStyle := perkStyles(player.Perks.Styles)
	treeIDs := []int{}
	if primaryStyle != nil {
		treeIDs = append(treeIDs, primaryStyle.Style)
	}
	if secondaryStyle != nil {
		treeIDs = append(treeIDs, secondaryStyle.Style)
	}
	runeTrees := loadOrEmptyMap(func() (map[int]postgres.RuneTreeDisplay, error) {
		return s.database.RuneTreeDisplayByIDs(ctx, treeIDs)
	})

	runeIDs := collectPerkIDs(primaryStyle, secondaryStyle)
	runes := loadOrEmptyMap(func() (map[int]postgres.RuneDisplay, error) {
		return s.database.RuneDisplayByIDs(ctx, runeIDs)
	})

	itemIDs := collectPostItems(*player)
	items := loadOrEmptyMap(func() (map[int]postgres.ItemDisplay, error) {
		return s.database.ItemDisplayByIDs(ctx, itemIDs)
	})

	winWord, color, authorIcon := "Lost", 0xCC0000, cdn.ProfileIconURL(3367)
	if player.Win {
		winWord, color, authorIcon = "Won", 0x008000, cdn.ProfileIconURL(4069)
	}

	fields := []*discordgo.MessageEmbedField{
		{Name: "Champion", Value: championToken(player.ChampionID, champions), Inline: true},
		{Name: "KDA", Value: formatKDA(*player), Inline: true},
		{Name: "Summoners", Value: summonerSpellTokens(*player, spells), Inline: false},
	}
	if primaryStyle != nil {
		fields = append(fields, runeStyleField(primaryStyle, runeTrees, runes))
	}
	if secondaryStyle != nil {
		fields = append(fields, runeStyleField(secondaryStyle, runeTrees, runes))
	}
	fields = append(fields, &discordgo.MessageEmbedField{
		Name:   "Build",
		Value:  buildItemLines(itemIDs, items),
		Inline: false,
	})

	blueBans, redBans := bansLinePost(match.Info.Teams, 100, champions), bansLinePost(match.Info.Teams, 200, champions)
	if blueBans != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "🔵 Bans", Value: blueBans, Inline: true})
	}
	if redBans != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "🔴 Bans", Value: redBans, Inline: true})
	}

	embed := &discordgo.MessageEmbed{
		Author: &discordgo.MessageEmbedAuthor{
			Name:    "Post Game",
			IconURL: authorIcon,
		},
		Color: color,
		Title: playerTitle,
		Description: fmt.Sprintf(
			"%s a %s game.\n\n**Match Duration**: %s.",
			winWord, queueName, formatDuration(matchDurationSeconds(match.Info)),
		),
		Fields: fields,
	}
	if playerProfileIconURL != "" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: playerProfileIconURL,
		}
	}
	discord.ApplyDefaultFooter(embed)
	return embed, nil
}

func championToken(championID int, champions map[int]postgres.ChampionDisplay) string {
	champion := champions[championID]
	name := strings.TrimSpace(champion.Name)
	if name == "" {
		name = fmt.Sprintf("Champion %d", championID)
	}
	if icon := strings.TrimSpace(champion.DiscordIcon); icon != "" {
		return fmt.Sprintf("%s %s", icon, name)
	}
	return name
}

func championIconOnly(championID int, champions map[int]postgres.ChampionDisplay) string {
	champion := champions[championID]
	return strings.TrimSpace(champion.DiscordIcon)
}

func findMatchPlayer(players []riot.MatchPlayer, playerPUUID, playerRiotID string) *riot.MatchPlayer {
	playerPUUID = strings.TrimSpace(playerPUUID)
	playerRiotID = strings.TrimSpace(playerRiotID)

	for idx := range players {
		if strings.TrimSpace(players[idx].PUUID) == playerPUUID {
			return &players[idx]
		}
	}

	for idx := range players {
		if strings.EqualFold(matchPlayerRiotID(players[idx]), playerRiotID) {
			return &players[idx]
		}
	}
	return nil
}

func matchPlayerRiotID(player riot.MatchPlayer) string {
	gameName := strings.TrimSpace(player.RiotIDGameName)
	tagLine := strings.TrimSpace(player.RiotIDTagline)
	if gameName != "" && tagLine != "" {
		return riot.FormatRiotID(gameName, tagLine)
	}
	if name := strings.TrimSpace(player.SummonerName); name != "" {
		return name
	}
	return strings.TrimSpace(player.PUUID)
}

func collectPostChampionIDs(player riot.MatchPlayer, teams []riot.MatchTeam) []int {
	seen := make(map[int]struct{})
	out := make([]int, 0, 1+len(teams))
	out = appendUniquePositiveID(out, seen, player.ChampionID)
	for _, team := range teams {
		for _, ban := range team.Bans {
			out = appendUniquePositiveID(out, seen, ban.ChampionID)
		}
	}
	return out
}

func collectPostItems(player riot.MatchPlayer) []int {
	out := make([]int, 0, 6)
	for _, id := range []int{player.Item0, player.Item1, player.Item2, player.Item3, player.Item4, player.Item5} {
		if id > 0 {
			out = append(out, id)
		}
	}
	return out
}

func summonerSpellTokens(player riot.MatchPlayer, spells map[int]postgres.SummonerSpellDisplay) string {
	ids := []int{player.Summoner1ID, player.Summoner2ID}
	tokens := make([]string, 0, 2)
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		spell := spells[id]
		if icon := strings.TrimSpace(spell.DiscordIcon); icon != "" {
			tokens = append(tokens, icon)
			continue
		}
		name := strings.TrimSpace(spell.Name)
		if name == "" {
			name = fmt.Sprintf("Spell %d", id)
		}
		tokens = append(tokens, name)
	}
	if len(tokens) == 0 {
		return "-"
	}
	return strings.Join(tokens, " ")
}

func formatKDA(player riot.MatchPlayer) string {
	ratio := float64(player.Kills + player.Assists)
	if player.Deaths > 0 {
		ratio /= float64(player.Deaths)
	}
	return fmt.Sprintf("%d/%d/%d %.2f:1", player.Kills, player.Deaths, player.Assists, ratio)
}

func perkStyles(styles []riot.MatchPerkStyle) (primary *riot.MatchPerkStyle, secondary *riot.MatchPerkStyle) {
	for idx := range styles {
		description := strings.ToLower(strings.TrimSpace(styles[idx].Description))
		if strings.Contains(description, "primary") {
			primary = &styles[idx]
		} else if strings.Contains(description, "sub") {
			secondary = &styles[idx]
		}
	}
	if primary == nil && len(styles) > 0 {
		primary = &styles[0]
	}
	if secondary == nil && len(styles) > 1 {
		secondary = &styles[1]
	}
	return primary, secondary
}

func collectPerkIDs(styles ...*riot.MatchPerkStyle) []int {
	seen := map[int]struct{}{}
	out := []int{}
	for _, style := range styles {
		if style == nil {
			continue
		}
		for _, selection := range style.Selections {
			if selection.Perk <= 0 {
				continue
			}
			if _, ok := seen[selection.Perk]; ok {
				continue
			}
			seen[selection.Perk] = struct{}{}
			out = append(out, selection.Perk)
		}
	}
	return out
}

func runeStyleField(style *riot.MatchPerkStyle, trees map[int]postgres.RuneTreeDisplay, runes map[int]postgres.RuneDisplay) *discordgo.MessageEmbedField {
	if style == nil {
		return nil
	}

	tree := trees[style.Style]
	treeName := strings.TrimSpace(tree.Name)
	if treeName == "" {
		treeName = "Runes"
	}
	fieldName := treeName
	if icon := strings.TrimSpace(tree.DiscordIcon); icon != "" {
		fieldName = fmt.Sprintf("%s %s", icon, treeName)
	}

	lines := make([]string, 0, len(style.Selections))
	for _, selection := range style.Selections {
		runeDisplay := runes[selection.Perk]
		name := strings.TrimSpace(runeDisplay.Name)
		if name == "" {
			name = fmt.Sprintf("Rune %d", selection.Perk)
		}
		if icon := strings.TrimSpace(runeDisplay.DiscordIcon); icon != "" {
			lines = append(lines, fmt.Sprintf("%s %s", icon, name))
		} else {
			lines = append(lines, name)
		}
	}
	if len(lines) == 0 {
		lines = []string{"-"}
	}

	return &discordgo.MessageEmbedField{
		Name:   fieldName,
		Value:  strings.Join(lines, "\n"),
		Inline: true,
	}
}

func buildItemLines(itemIDs []int, items map[int]postgres.ItemDisplay) string {
	if len(itemIDs) == 0 {
		return "-"
	}

	names := make([]string, 0, len(itemIDs))
	for _, id := range itemIDs {
		item := items[id]
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("Item %d", id)
		}
		names = append(names, name)
	}

	rows := make([]string, 0, (len(names)+2)/3)
	for idx := 0; idx < len(names); idx += 3 {
		rows = append(rows, strings.Join(names[idx:min(idx+3, len(names))], " | "))
	}
	return strings.Join(rows, "\n")
}

func bansLinePost(teams []riot.MatchTeam, teamID int, champions map[int]postgres.ChampionDisplay) string {
	for _, team := range teams {
		if team.TeamID != teamID {
			continue
		}
		seen := make(map[int]struct{}, len(team.Bans))
		championIDs := make([]int, 0, len(team.Bans))
		for _, ban := range team.Bans {
			championIDs = appendUniquePositiveID(championIDs, seen, ban.ChampionID)
		}
		return banIconsLine(championIDs, champions)
	}
	return ""
}

func matchDurationSeconds(info riot.MatchInfo) int64 {
	if info.GameDuration > 0 {
		return info.GameDuration
	}
	if info.GameStartTimestamp > 0 && info.GameEndTimestamp > info.GameStartTimestamp {
		return (info.GameEndTimestamp - info.GameStartTimestamp) / 1000
	}
	return 0
}

func formatDuration(seconds int64) string {
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	rem := seconds % 60
	return fmt.Sprintf("%dm%ds", minutes, rem)
}
