package tracknotify

import (
	"strings"

	"github.com/bingbr/League-API-bot/internal/storage/postgres"
)

func loadOrEmptyMap[K comparable, V any](load func() (map[K]V, error)) map[K]V {
	loaded, err := load()
	if err != nil || loaded == nil {
		return map[K]V{}
	}
	return loaded
}

func appendUniquePositiveID(out []int, seen map[int]struct{}, id int) []int {
	if id <= 0 {
		return out
	}
	if _, ok := seen[id]; ok {
		return out
	}
	seen[id] = struct{}{}
	return append(out, id)
}

func banIconsLine(championIDs []int, champions map[int]postgres.ChampionDisplay) string {
	tokens := make([]string, 0, len(championIDs))
	for _, championID := range championIDs {
		icon := championIconOnly(championID, champions)
		if icon == "" {
			continue
		}
		tokens = append(tokens, icon)
	}
	if len(tokens) == 0 {
		return ""
	}
	return strings.Join(tokens, " ")
}
