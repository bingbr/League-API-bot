package commands

import (
	"testing"
	"time"

	"github.com/bingbr/League-API-bot/internal/storage/postgres"
)

func TestNextRotationExpiryUTC(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{
			name: "tuesday before",
			now:  time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "wednesday morning",
			now:  time.Date(2026, 2, 4, 10, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 4, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "wednesday afternoon",
			now:  time.Date(2026, 2, 4, 13, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "thursday",
			now:  time.Date(2026, 2, 5, 9, 0, 0, 0, time.UTC),
			want: time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nextRotationExpiryUTC(tt.now)
			if !got.Equal(tt.want) {
				t.Fatalf("nextRotationExpiryUTC(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestMergeChampionIDs(t *testing.T) {
	got := mergeChampionIDs([]int{12, 32, 12, 55}, []int{55, 90, 10})
	want := []int{12, 32, 55, 90, 10}

	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for idx := range want {
		if got[idx] != want[idx] {
			t.Fatalf("got[%d] = %d, want %d", idx, got[idx], want[idx])
		}
	}
}

func TestChampionLines(t *testing.T) {
	champions := map[int]postgres.ChampionDisplay{
		1: {ChampionID: 1, Name: "Annie", DiscordIcon: "<:Annie:1>"},
		2: {ChampionID: 2, Name: "Olaf", DiscordIcon: ""},
	}

	lines := championLines([]int{1, 2, 3}, champions)
	if len(lines) != 3 {
		t.Fatalf("len(lines) = %d, want 3", len(lines))
	}
	if lines[0] != "<:Annie:1> Annie" {
		t.Fatalf("lines[0] = %q", lines[0])
	}
	if lines[1] != "Olaf" {
		t.Fatalf("lines[1] = %q", lines[1])
	}
	if lines[2] != "ID 3" {
		t.Fatalf("lines[2] = %q", lines[2])
	}

	empty := championLines([]int{}, map[int]postgres.ChampionDisplay{})
	if len(empty) != 1 {
		t.Fatalf("len(empty) = %d, want 1", len(empty))
	}
	if empty[0] != "No champions available at the moment." {
		t.Fatalf("empty[0] = %q", empty[0])
	}
}
