package riot

import (
	"fmt"
	"strings"
)

const UnrankedTier = "unranked"

func QueueEntry(entries []LeagueEntry, queueType string) *LeagueEntry {
	for idx := range entries {
		if entries[idx].QueueType == queueType {
			return &entries[idx]
		}
	}
	return nil
}

func RankTiersToLookup(entries []LeagueEntry) []string {
	return collectRankTiers(func(emit func(string)) {
		for _, entry := range entries {
			emit(entry.Tier)
		}
	})
}

func RankTiersToLookupByPUUID(entries map[string]*LeagueEntry) []string {
	return collectRankTiers(func(emit func(string)) {
		for _, entry := range entries {
			if entry == nil {
				continue
			}
			emit(entry.Tier)
		}
	})
}

func collectRankTiers(visit func(emit func(string))) []string {
	seen := map[string]struct{}{UnrankedTier: {}}
	out := []string{UnrankedTier}
	visit(func(rawTier string) {
		tier := NormalizeRankTier(rawTier)
		if tier == "" {
			return
		}
		if _, ok := seen[tier]; ok {
			return
		}
		seen[tier] = struct{}{}
		out = append(out, tier)
	})
	return out
}

func HideDivisionForTier(tier string) bool {
	tier = NormalizeRankTier(tier)
	return tier == "master" || tier == "grandmaster" || tier == "challenger"
}

func TierTitle(tier, unrankedLabel string) string {
	tier = NormalizeRankTier(tier)
	if tier == "" {
		if strings.TrimSpace(unrankedLabel) != "" {
			return strings.TrimSpace(unrankedLabel)
		}
		return "Unranked"
	}
	return strings.ToUpper(tier[:1]) + tier[1:]
}

func RankedLine(entry LeagueEntry, unrankedLabel string) string {
	tier := NormalizeRankTier(entry.Tier)
	if tier == "" {
		return TierTitle("", unrankedLabel)
	}

	title := TierTitle(tier, unrankedLabel)
	rank := strings.TrimSpace(strings.ToUpper(entry.Rank))
	if HideDivisionForTier(tier) {
		rank = ""
	}
	if rank == "" {
		return fmt.Sprintf("%s %dLP", title, entry.LeaguePoints)
	}
	return fmt.Sprintf("%s %s %dLP", title, rank, entry.LeaguePoints)
}

func RankedLineWithIcon(entry *LeagueEntry, rankIcons map[string]string, unrankedLabel string) string {
	if strings.TrimSpace(unrankedLabel) == "" {
		unrankedLabel = "Unranked"
	}
	if entry == nil {
		return WithRankIcon(rankIcons, UnrankedTier, unrankedLabel)
	}
	tier := NormalizeRankTier(entry.Tier)
	if tier == "" {
		tier = UnrankedTier
	}
	return WithRankIcon(rankIcons, tier, RankedLine(*entry, unrankedLabel))
}

func RecordLine(entry LeagueEntry, winSuffix, lossSuffix string) string {
	winSuffix = strings.TrimSpace(winSuffix)
	lossSuffix = strings.TrimSpace(lossSuffix)
	if winSuffix == "" {
		winSuffix = "W"
	}
	if lossSuffix == "" {
		lossSuffix = "L"
	}

	total := entry.Wins + entry.Losses
	if total <= 0 {
		return fmt.Sprintf("0%% %d%s %d%s", entry.Wins, winSuffix, entry.Losses, lossSuffix)
	}
	winRate := (entry.Wins*100 + total/2) / total
	return fmt.Sprintf("%d%% %d%s %d%s", winRate, entry.Wins, winSuffix, entry.Losses, lossSuffix)
}

func RecordLineOr(entry *LeagueEntry, winSuffix, lossSuffix, emptyValue string) string {
	if entry == nil {
		emptyValue = strings.TrimSpace(emptyValue)
		if emptyValue == "" {
			emptyValue = "-"
		}
		return emptyValue
	}
	return RecordLine(*entry, winSuffix, lossSuffix)
}

func WithRankIcon(rankIcons map[string]string, tier, value string) string {
	icon := strings.TrimSpace(rankIcons[NormalizeRankTier(tier)])
	if icon == "" {
		return value
	}
	return fmt.Sprintf("%s %s", icon, value)
}
