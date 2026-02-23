package commands

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bingbr/League-API-bot/internal/riot"
	"github.com/bingbr/League-API-bot/internal/storage/postgres"
)

func TestSoloQueueMMR(t *testing.T) {
	tests := []struct {
		name  string
		entry riot.LeagueEntry
		want  int
	}{
		{
			name:  "unranked",
			entry: riot.LeagueEntry{},
			want:  leadboardMMRUnranked,
		},
		{
			name: "gold two",
			entry: riot.LeagueEntry{
				Tier:         "GOLD",
				Rank:         "II",
				LeaguePoints: 87,
			},
			want: 1487,
		},
		{
			name: "challenger",
			entry: riot.LeagueEntry{
				Tier:         "CHALLENGER",
				Rank:         "I",
				LeaguePoints: 512,
			},
			want: 3312,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := soloQueueMMR(tc.entry); got != tc.want {
				t.Fatalf("soloQueueMMR() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestSortLeadboardRows(t *testing.T) {
	rows := []leadboardRow{
		{
			account: postgres.TrackedAccount{NickName: "Bravo", TagLine: "NA1"},
			mmr:     2000,
		},
		{
			account: postgres.TrackedAccount{NickName: "Alpha", TagLine: "NA1"},
			mmr:     2000,
		},
		{
			account: postgres.TrackedAccount{NickName: "Top", TagLine: "NA1"},
			mmr:     2800,
		},
	}

	sortLeadboardRows(rows)

	got := []string{rows[0].account.RiotID(), rows[1].account.RiotID(), rows[2].account.RiotID()}
	want := []string{"Top#NA1", "Alpha#NA1", "Bravo#NA1"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sorted riotID[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestBuildLeadboardEmbeds(t *testing.T) {
	rows := make([]leadboardRow, 0, 9)
	for i := range 9 {
		rows = append(rows, leadboardRow{
			account: postgres.TrackedAccount{NickName: fmt.Sprintf("Player%02d", i+1), TagLine: "NA1"},
			mmr:     1000 - i,
		})
	}

	embeds := buildLeadboardEmbeds(rows, map[string]string{})
	if len(embeds) != 1 {
		t.Fatalf("len(embeds) = %d, want 1", len(embeds))
	}
	if got := len(embeds[0].Fields); got != 3 {
		t.Fatalf("len(embeds[0].Fields) = %d, want 3", got)
	}
	if embeds[0].Fields[0].Name != "Nick" || embeds[0].Fields[1].Name != "Rank" || embeds[0].Fields[2].Name != "Win Rate" {
		t.Fatalf("unexpected field names: %q, %q, %q", embeds[0].Fields[0].Name, embeds[0].Fields[1].Name, embeds[0].Fields[2].Name)
	}
	if got := embeds[0].Fields[0].Value; got == "-" {
		t.Fatalf("Nick field should not be empty")
	}
	if strings.Contains(embeds[0].Fields[0].Value, "1. ") {
		t.Fatalf("Nick field should not include numeric prefix: %q", embeds[0].Fields[0].Value)
	}
}
