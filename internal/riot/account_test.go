package riot

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestPlatformContinent(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"br1", "americas"}, {"na1", "americas"}, {"la1", "americas"},
		{"kr", "asia"}, {"jp1", "asia"},
		{"euw1", "europe"}, {"eun1", "europe"}, {"tr1", "europe"}, {"me1", "europe"},
		{"sg2", "sea"}, {"vn2", "sea"},
		{"na", ""}, {"xx1", ""},
	}
	for _, tc := range tests {
		if got := PlatformContinent(tc.input); got != tc.want {
			t.Fatalf("PlatformContinent(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizePlatformRegion(t *testing.T) {
	if got := NormalizePlatformRegion("  BR1  "); got != "br1" {
		t.Fatalf("NormalizePlatformRegion() = %q, want %q", got, "br1")
	}
	if got := NormalizePlatformRegion("invalid"); got != "" {
		t.Fatalf("NormalizePlatformRegion() = %q, want empty", got)
	}
}

func TestNormalizeRankTier(t *testing.T) {
	if got := NormalizeRankTier("  Gold "); got != "gold" {
		t.Fatalf("NormalizeRankTier() = %q, want %q", got, "gold")
	}
}

func TestSplitRiotID(t *testing.T) {
	tests := []struct {
		input    string
		wantName string
		wantTag  string
		wantErr  error
	}{
		{"Bekko#Ekko", "Bekko", "Ekko", nil},
		{" ", "", "", ErrRiotIDRequired},
		{"Bekko", "", "", ErrInvalidRiotID},
		{"#Ekko", "", "", ErrInvalidRiotID},
		{"Bekko#", "", "", ErrInvalidRiotID},
	}
	for _, tt := range tests {
		name, tag, err := SplitRiotID(tt.input)
		if !errors.Is(err, tt.wantErr) {
			t.Fatalf("SplitRiotID(%q) error = '%v', want '%v", tt.input, err, tt.wantErr)
		}
		if err == nil && (name != tt.wantName || tag != tt.wantTag) {
			t.Fatalf("SplitRiotID(%q) = (%q, %q), want (%q, %q)", tt.input, name, tag, tt.wantName, tt.wantTag)
		}
	}
}

func TestFormatRiotID(t *testing.T) {
	if got := FormatRiotID(" Bekko ", "#Ekko "); got != "Bekko#Ekko" {
		t.Fatalf("FormatRiotID() = %q, want %q", got, "Bekko#Ekko")
	}
}

func TestIsAccountByRiotIDNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "not found on account by riot id endpoint",
			err: fmt.Errorf("wrapped: %w", &HTTPStatusError{
				URL:        "https://americas.api.riotgames.com/riot/account/v1/accounts/by-riot-id/test/tag",
				StatusCode: http.StatusNotFound,
				Body:       `{"status":{"status_code":404}}`,
			}),
			want: true,
		},
		{
			name: "not found on different endpoint",
			err: &HTTPStatusError{
				URL:        "https://na1.api.riotgames.com/lol/summoner/v4/summoners/by-puuid/test",
				StatusCode: http.StatusNotFound,
				Body:       `{"status":{"status_code":404}}`,
			},
			want: false,
		},
		{
			name: "different status on account endpoint",
			err: &HTTPStatusError{
				URL:        "https://americas.api.riotgames.com/riot/account/v1/accounts/by-riot-id/test/tag",
				StatusCode: http.StatusForbidden,
				Body:       `{"status":{"status_code":403}}`,
			},
			want: false,
		},
		{
			name: "non status error",
			err:  errors.New("boom"),
			want: false,
		},
	}

	for _, tt := range tests {
		if got := IsAccountByRiotIDNotFound(tt.err); got != tt.want {
			t.Fatalf("IsAccountByRiotIDNotFound(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
