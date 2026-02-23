package cdn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
)

func TestBuildRankEmojiAssets(t *testing.T) {
	assets := buildRankEmojiAssets()
	expectedTiers := map[string]bool{
		"unranked": true, "iron": true, "bronze": true, "silver": true, "gold": true,
		"platinum": true, "emerald": true, "diamond": true, "master": true,
		"grandmaster": true, "challenger": true,
	}
	if len(assets) != len(expectedTiers) {
		t.Fatalf("len(assets) = %d, want %d", len(assets), len(expectedTiers))
	}

	seenNames := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		if asset.Kind != emojiKindRankTier {
			t.Fatalf("kind = %v, want %v", asset.Kind, emojiKindRankTier)
		}
		if !expectedTiers[asset.Tier] {
			t.Fatalf("unexpected tier %q", asset.Tier)
		}
		if !strings.HasPrefix(asset.URL, localRankSourcePrefix) {
			t.Fatalf("url = %q, want prefix %q", asset.URL, localRankSourcePrefix)
		}
		if asset.Name == "" {
			t.Fatalf("name should not be empty for tier %q", asset.Tier)
		}
		lower := strings.ToLower(asset.Name)
		if _, ok := seenNames[lower]; ok {
			t.Fatalf("duplicate name %q", asset.Name)
		}
		seenNames[lower] = struct{}{}
	}
}

func TestFetchEmojiImageLoadsLocalRankPNG(t *testing.T) {
	t.Parallel()

	img, err := fetchEmojiImage(context.Background(), nil, localRankSourcePrefix+"emerald.png", EmojiEntry{})
	if err != nil {
		t.Fatalf("fetchEmojiImage() error = %v", err)
	}

	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(img.DataURI, prefix) {
		t.Fatalf("data uri = %q, want prefix %q", img.DataURI, prefix)
	}

	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(img.DataURI, prefix))
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if len(decoded) < 8 {
		t.Fatalf("decoded payload too short: %d", len(decoded))
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.Equal(decoded[:8], pngSignature) {
		t.Fatalf("decoded payload is not png")
	}

	wantHash := sha256.Sum256(decoded)
	if img.ContentHash != hex.EncodeToString(wantHash[:]) {
		t.Fatalf("content hash = %q, want %q", img.ContentHash, hex.EncodeToString(wantHash[:]))
	}
	if img.SourceContentLength <= 0 {
		t.Fatalf("source content length = %d, want > 0", img.SourceContentLength)
	}
}

func TestFetchEmojiImageLocalRankUsesContentHashForNotModified(t *testing.T) {
	t.Parallel()

	first, err := fetchEmojiImage(context.Background(), nil, localRankSourcePrefix+"diamond.png", EmojiEntry{})
	if err != nil {
		t.Fatalf("fetchEmojiImage() error = %v", err)
	}

	second, err := fetchEmojiImage(context.Background(), nil, localRankSourcePrefix+"diamond.png", EmojiEntry{
		ContentHash: first.ContentHash,
	})
	if err != nil {
		t.Fatalf("fetchEmojiImage() second call error = %v", err)
	}
	if !second.NotModified {
		t.Fatalf("expected local rank image to be not modified when hash matches")
	}
	if second.DataURI != "" {
		t.Fatalf("data uri = %q, want empty when not modified", second.DataURI)
	}
	if second.ContentHash != first.ContentHash {
		t.Fatalf("content hash = %q, want %q", second.ContentHash, first.ContentHash)
	}
}

func TestFetchEmojiImageReturnsNotModifiedWithCachedMetadata(t *testing.T) {
	t.Parallel()

	const etag = `"abc123"`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		t.Fatalf("missing expected If-None-Match header")
	}))
	defer server.Close()

	prev := EmojiEntry{
		ContentHash:         "cached-hash",
		SourceETag:          etag,
		SourceLastModified:  "Wed, 21 Oct 2015 07:28:00 GMT",
		SourceContentLength: 42,
	}
	img, err := fetchEmojiImage(context.Background(), server.Client(), server.URL+"/aatrox.png", prev)
	if err != nil {
		t.Fatalf("fetchEmojiImage() error = %v", err)
	}
	if !img.NotModified {
		t.Fatalf("expected not modified response")
	}
	if img.ContentHash != prev.ContentHash {
		t.Fatalf("content hash = %q, want %q", img.ContentHash, prev.ContentHash)
	}
	if img.SourceETag != etag {
		t.Fatalf("etag = %q, want %q", img.SourceETag, etag)
	}
}

func TestFetchEmojiImageRejectsTooLarge(t *testing.T) {
	t.Parallel()

	large := bytes.Repeat([]byte("x"), maxEmojiImageBytes+1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(large)
	}))
	defer server.Close()

	_, err := fetchEmojiImage(context.Background(), server.Client(), server.URL+"/big.png", EmojiEntry{})
	if err == nil {
		t.Fatalf("expected error for oversized image")
	}
	if !strings.Contains(err.Error(), "image too large") {
		t.Fatalf("error = %q, want contains %q", err.Error(), "image too large")
	}
}

func TestEmojiAssetManifestKey(t *testing.T) {
	champion := emojiAsset{Kind: emojiKindChampion, ID: 266}
	if got := champion.manifestKey(); got != "champion:266" {
		t.Fatalf("champion manifest key = %q", got)
	}

	rank := emojiAsset{Kind: emojiKindRankTier, Tier: "Gold"}
	if got := rank.manifestKey(); got != "rank:gold" {
		t.Fatalf("rank manifest key = %q", got)
	}
}

func TestFormatDiscordIcon(t *testing.T) {
	if got := formatDiscordIcon(nil); got != "" {
		t.Fatalf("formatDiscordIcon(nil) = %q, want empty string", got)
	}

	static := &discordgo.Emoji{ID: "123", Name: "Aatrox"}
	if got := formatDiscordIcon(static); got != "<:Aatrox:123>" {
		t.Fatalf("formatDiscordIcon(static) = %q", got)
	}

	animated := &discordgo.Emoji{ID: "456", Name: "Poro", Animated: true}
	if got := formatDiscordIcon(animated); got != "<:Poro:456>" {
		t.Fatalf("formatDiscordIcon(animated) = %q", got)
	}
}

func TestSyncEmojiAssetUnchanged(t *testing.T) {
	t.Parallel()

	body := []byte("not-a-real-png-but-still-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	asset := emojiAsset{
		Kind: emojiKindChampion,
		ID:   266,
		Name: "aatrox",
		URL:  server.URL + "/aatrox.png",
	}
	hash := sha256.Sum256(body)
	entry := EmojiEntry{
		SourceURL:   asset.URL,
		ContentHash: hex.EncodeToString(hash[:]),
		EmojiID:     "e1",
		EmojiName:   "Aatrox",
	}
	idx := newEmojiIndex([]*discordgo.Emoji{{ID: "e1", Name: "Aatrox"}})

	result, err := syncEmojiAsset(context.Background(), server.Client(), nil, "app-id", asset, entry, idx)
	if err != nil {
		t.Fatalf("syncEmojiAsset() error = %v", err)
	}
	if !result.kept {
		t.Fatalf("expected unchanged asset to be kept")
	}
	if result.created {
		t.Fatalf("expected unchanged asset not to be created")
	}
	if result.icon != "<:Aatrox:e1>" {
		t.Fatalf("icon = %q, want %q", result.icon, "<:Aatrox:e1>")
	}
	if result.manifestEntry.AssetKey != "champion:266" {
		t.Fatalf("asset key = %q, want %q", result.manifestEntry.AssetKey, "champion:266")
	}
	if result.manifestEntry.ContentHash != entry.ContentHash {
		t.Fatalf("content hash = %q, want %q", result.manifestEntry.ContentHash, entry.ContentHash)
	}
}

func TestSyncEmojiAssetUnchangedIgnoresSourceURLVersion(t *testing.T) {
	t.Parallel()

	body := []byte("versioned-url-same-content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(body)
	}))
	defer server.Close()

	asset := emojiAsset{
		Kind: emojiKindChampion,
		ID:   266,
		Name: "Aatrox",
		URL:  server.URL + "/15.2.1/img/champion/Aatrox.png",
	}
	hash := sha256.Sum256(body)
	entry := EmojiEntry{
		SourceURL:   server.URL + "/15.1.1/img/champion/Aatrox.png",
		ContentHash: hex.EncodeToString(hash[:]),
		EmojiID:     "e1",
		EmojiName:   "Aatrox",
	}
	idx := newEmojiIndex([]*discordgo.Emoji{{ID: "e1", Name: "Aatrox"}})

	result, err := syncEmojiAsset(context.Background(), server.Client(), nil, "app-id", asset, entry, idx)
	if err != nil {
		t.Fatalf("syncEmojiAsset() error = %v", err)
	}
	if !result.kept || result.created {
		t.Fatalf("expected unchanged asset to be kept without recreate")
	}
}

func TestRunSyncBatchEmptyAssetsNoop(t *testing.T) {
	err := runSyncBatch(
		context.Background(),
		&Client{httpClient: http.DefaultClient},
		nil,
		"app-id",
		nil,
		nil,
		nil,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		time.Now().UTC(),
	)
	if err != nil {
		t.Fatalf("runSyncBatch() error = %v", err)
	}
}

func TestDeleteStaleEmojisSkipsManifestDeletionOnDiscordDeleteError(t *testing.T) {
	originalDelete := deleteEmojiWithRetry
	deleteEmojiWithRetry = func(context.Context, *discordgo.Session, string, string) error {
		return errors.New("discord delete failed")
	}
	defer func() {
		deleteEmojiWithRetry = originalDelete
	}()

	db := &emojiSyncTestDB{}
	manifest := map[string]EmojiEntry{
		"rank:old": {
			AssetKey:  "rank:old",
			Kind:      emojiKindRankTier.String(),
			Tier:      "old",
			EmojiID:   "emoji-1",
			EmojiName: "Old",
		},
	}
	idx := newEmojiIndex([]*discordgo.Emoji{{ID: "emoji-1", Name: "Old"}})

	err := deleteStaleEmojis(
		context.Background(),
		nil,
		"app-id",
		emojiKindRankTier,
		map[string]struct{}{},
		manifest,
		idx,
		db,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("deleteStaleEmojis() error = %v", err)
	}
	if len(db.deletedKeys) != 0 {
		t.Fatalf("DeleteEmojiEntries called with %v, want no call", db.deletedKeys)
	}
	if idx.find("emoji-1", "Old") == nil {
		t.Fatalf("emoji index entry was removed after failed delete")
	}
}

func TestDeleteStaleEmojisDeletesManifestOnSuccess(t *testing.T) {
	originalDelete := deleteEmojiWithRetry
	deleteEmojiWithRetry = func(context.Context, *discordgo.Session, string, string) error {
		return nil
	}
	defer func() {
		deleteEmojiWithRetry = originalDelete
	}()

	db := &emojiSyncTestDB{}
	manifest := map[string]EmojiEntry{
		"rank:old": {
			AssetKey:  "rank:old",
			Kind:      emojiKindRankTier.String(),
			Tier:      "old",
			EmojiID:   "emoji-1",
			EmojiName: "Old",
		},
	}
	idx := newEmojiIndex([]*discordgo.Emoji{{ID: "emoji-1", Name: "Old"}})

	err := deleteStaleEmojis(
		context.Background(),
		nil,
		"app-id",
		emojiKindRankTier,
		map[string]struct{}{},
		manifest,
		idx,
		db,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("deleteStaleEmojis() error = %v", err)
	}
	if len(db.deletedKeys) != 1 || db.deletedKeys[0] != "rank:old" {
		t.Fatalf("DeleteEmojiEntries keys = %v, want [rank:old]", db.deletedKeys)
	}
	if idx.find("emoji-1", "Old") != nil {
		t.Fatalf("emoji index entry still present after successful delete")
	}
}

func TestRunSyncBatchDeleteEmojiEntriesFailureBubbles(t *testing.T) {
	db := &emojiSyncTestDB{
		manifest: map[string]EmojiEntry{
			"rank:stale": {
				AssetKey: "rank:stale",
				Kind:     emojiKindRankTier.String(),
				Tier:     "stale",
			},
		},
		deleteErr: errors.New("db delete failed"),
	}

	err := runSyncBatch(
		context.Background(),
		&Client{httpClient: http.DefaultClient},
		nil,
		"app-id",
		[]emojiAsset{{
			Kind: emojiKindRankTier,
			Tier: "gold",
			Name: "Gold",
			URL:  "https://example.invalid/gold.png",
		}},
		newEmojiIndex(nil),
		db,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		time.Now().UTC(),
	)
	if err == nil {
		t.Fatalf("runSyncBatch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "delete stale emoji entries") {
		t.Fatalf("runSyncBatch() error = %q, want delete stale emoji entries", err.Error())
	}
}

type emojiSyncTestDB struct {
	manifest    map[string]EmojiEntry
	deleteErr   error
	deletedKeys []string
}

func (d *emojiSyncTestDB) UpsertDiscordIcons(context.Context, DiscordIcons, time.Time) error {
	return nil
}

func (d *emojiSyncTestDB) EmojiEntries(context.Context) (map[string]EmojiEntry, error) {
	if d.manifest == nil {
		return map[string]EmojiEntry{}, nil
	}
	out := make(map[string]EmojiEntry, len(d.manifest))
	maps.Copy(out, d.manifest)
	return out, nil
}

func (d *emojiSyncTestDB) UpsertEmojiEntries(context.Context, []EmojiEntry) error {
	return nil
}

func (d *emojiSyncTestDB) DeleteEmojiEntries(_ context.Context, assetKeys []string) error {
	d.deletedKeys = append(d.deletedKeys, assetKeys...)
	return d.deleteErr
}

func (d *emojiSyncTestDB) GetRiotCDNSyncVersion(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (d *emojiSyncTestDB) UpsertRiotCDNSyncVersion(context.Context, string, string) error {
	return nil
}
