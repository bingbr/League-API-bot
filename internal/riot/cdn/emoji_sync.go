package cdn

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/bingbr/League-API-bot/data"
	"github.com/bwmarrin/discordgo"
	"golang.org/x/sync/errgroup"
)

const (
	emojiLocale           = "en_US"
	maxEmojiImageBytes    = 256 * 1024
	runeImageBaseURL      = "https://ddragon.leagueoflegends.com/cdn/img"
	localRankSourcePrefix = "local-rank://"
	emojiSyncConcurrency  = 20 // Limit concurrent Discord interactions
	emojiMaxRetries       = 2
)

type EmojiEntry struct {
	AssetKey            string
	Kind                string
	AssetID             *int
	Tier                string
	EmojiName           string
	EmojiID             string
	DiscordIcon         string
	SourceURL           string
	ContentHash         string
	SourceETag          string
	SourceLastModified  string
	SourceContentLength int64
}

type DiscordIconDB interface {
	UpsertDiscordIcons(ctx context.Context, icons DiscordIcons, fetchedAt time.Time) error
	EmojiEntries(ctx context.Context) (map[string]EmojiEntry, error)
	UpsertEmojiEntries(ctx context.Context, entries []EmojiEntry) error
	DeleteEmojiEntries(ctx context.Context, assetKeys []string) error
	GetRiotCDNSyncVersion(ctx context.Context, syncKey string) (string, bool, error)
	UpsertRiotCDNSyncVersion(ctx context.Context, syncKey, version string) error
}

type emojiKind int

const (
	emojiKindChampion emojiKind = iota + 1
	emojiKindSummonerSpell
	emojiKindRuneTree
	emojiKindRune
	emojiKindRankTier
)

func (k emojiKind) String() string {
	switch k {
	case emojiKindChampion:
		return "champion"
	case emojiKindSummonerSpell:
		return "summoner-spell"
	case emojiKindRuneTree:
		return "rune-tree"
	case emojiKindRune:
		return "rune"
	case emojiKindRankTier:
		return "rank"
	default:
		return "unknown"
	}
}

type emojiAsset struct {
	Kind emojiKind
	ID   int
	Tier string
	Name string
	URL  string
}

func (a emojiAsset) manifestKey() string {
	if a.Kind == emojiKindRankTier {
		return fmt.Sprintf("%s:%s", a.Kind, strings.ToLower(a.Tier))
	}
	return fmt.Sprintf("%s:%d", a.Kind, a.ID)
}

func StartApplicationEmojiSync(ctx context.Context, session *discordgo.Session, version string, syncNonRank bool, db DiscordIconDB, logger *slog.Logger) (<-chan error, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if session == nil {
		return nil, fmt.Errorf("discord session is required")
	}
	appID, err := resolveApplicationID(session)
	if err != nil {
		return nil, fmt.Errorf("fetch discord app identity: %w", err)
	}

	// Fetch existing state
	existingEmojis, err := session.ApplicationEmojis(appID)
	if err != nil {
		return nil, fmt.Errorf("list application emojis: %w", err)
	}
	idx := newEmojiIndex(existingEmojis)
	client := NewClient()
	fetchedAt := time.Now().UTC()
	L.Update(version, fetchedAt)

	// Sync Ranks
	rankAssets := buildRankEmojiAssets()
	if err := runSyncBatch(ctx, client, session, appID, rankAssets, idx, db, logger, fetchedAt); err != nil {
		return nil, fmt.Errorf("sync ranks: %w", err)
	}

	if !syncNonRank {
		return nil, nil
	}

	// Background sync for the rest
	done := make(chan error, 1)
	go func() {
		logger.Info("=== Background emoji sync started ===", "version", version)
		defer close(done)
		done <- syncRemainingKinds(ctx, client, session, appID, version, db, logger, fetchedAt, idx)
	}()
	return done, nil
}

func resolveApplicationID(session *discordgo.Session) (string, error) {
	if session.State != nil && session.State.User != nil && session.State.User.ID != "" {
		return session.State.User.ID, nil
	}
	user, err := session.User("@me")
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

func syncRemainingKinds(ctx context.Context, client *Client, s *discordgo.Session, appID, version string, db DiscordIconDB, log *slog.Logger, fetchedAt time.Time, idx *emojiIndex) error {
	assets, err := buildNonRankEmojiAssets(ctx, client, version)
	if err != nil {
		return err
	}

	assetsByKind := make(map[emojiKind][]emojiAsset)
	for _, a := range assets {
		assetsByKind[a.Kind] = append(assetsByKind[a.Kind], a)
	}

	for _, kind := range []emojiKind{emojiKindChampion, emojiKindSummonerSpell, emojiKindRuneTree, emojiKindRune} {
		if list := assetsByKind[kind]; len(list) > 0 {
			if err := runSyncBatch(ctx, client, s, appID, list, idx, db, log, fetchedAt); err != nil {
				return err
			}
		}
	}
	log.Info("=== Background emoji sync ended ===")
	return nil
}

func runSyncBatch(ctx context.Context, client *Client, s *discordgo.Session, appID string, assets []emojiAsset, idx *emojiIndex, db DiscordIconDB, log *slog.Logger, fetchedAt time.Time) error {
	if len(assets) == 0 {
		return nil
	}
	kind := assets[0].Kind
	log.Debug("syncing emojis", "kind", kind, "count", len(assets))

	manifest, err := db.EmojiEntries(ctx)
	if err != nil {
		return fmt.Errorf("load manifest: %w", err)
	}

	icons := newDiscordIcons()

	validKeys := make(map[string]struct{}, len(assets))
	for _, a := range assets {
		validKeys[a.manifestKey()] = struct{}{}
	}

	if err := deleteStaleEmojis(ctx, s, appID, kind, validKeys, manifest, idx, db, log); err != nil {
		return err
	}

	var (
		mu                    sync.Mutex
		manifestUpdates       []EmojiEntry
		created, kept, failed int
	)
	recordFailure := func(asset emojiAsset, err error) {
		mu.Lock()
		failed++
		mu.Unlock()
		log.Warn("Failed to sync emoji", "asset", asset.Name, "err", err)
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(emojiSyncConcurrency)
	for _, asset := range assets {
		entry := manifest[asset.manifestKey()]
		g.Go(func() error {
			result, err := syncEmojiAsset(gctx, client.httpClient, s, appID, asset, entry, idx)
			if err != nil {
				recordFailure(asset, err)
				return nil
			}

			mu.Lock()
			if result.created {
				created++
			}
			if result.kept {
				kept++
			}
			updateIcons(&icons, asset, result.icon)
			manifestUpdates = append(manifestUpdates, result.manifestEntry)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	if len(manifestUpdates) > 0 {
		if err := db.UpsertEmojiEntries(ctx, manifestUpdates); err != nil {
			return err
		}
	}
	if err := db.UpsertDiscordIcons(ctx, icons, fetchedAt); err != nil {
		return err
	}

	log.Debug("synced emojis", "kind", kind, "created", created, "kept", kept, "failed", failed)
	return nil
}

func newDiscordIcons() DiscordIcons {
	return DiscordIcons{
		Champions:      make(map[int]string),
		SummonerSpells: make(map[int]string),
		RuneTrees:      make(map[int]string),
		Runes:          make(map[int]string),
		Ranks:          make(map[string]string),
	}
}

func deleteStaleEmojis(ctx context.Context, s *discordgo.Session, appID string, kind emojiKind, validKeys map[string]struct{}, manifest map[string]EmojiEntry, idx *emojiIndex, db DiscordIconDB, log *slog.Logger) error {
	var staleKeys []string
	kindName := kind.String()
	for key, entry := range manifest {
		if entry.Kind != kindName {
			continue
		}
		if _, ok := validKeys[key]; ok {
			continue
		}
		if emoji := idx.find(entry.EmojiID, entry.EmojiName); emoji != nil {
			if err := deleteEmojiWithRetry(ctx, s, appID, emoji.ID); err != nil {
				log.Warn("Failed to delete stale emoji", "name", emoji.Name, "assetKey", key, "error", err)
				continue
			}
			idx.delete(emoji.ID)
			log.Info("Deleted stale emoji", "name", emoji.Name)
		}
		staleKeys = append(staleKeys, key)
	}
	if len(staleKeys) > 0 {
		if err := db.DeleteEmojiEntries(ctx, staleKeys); err != nil {
			return fmt.Errorf("delete stale emoji entries: %w", err)
		}
	}
	return nil
}

type syncEmojiAssetResult struct {
	manifestEntry EmojiEntry
	icon          string
	created       bool
	kept          bool
}

func syncEmojiAsset(ctx context.Context, client *http.Client, s *discordgo.Session, appID string, asset emojiAsset, entry EmojiEntry, idx *emojiIndex) (syncEmojiAssetResult, error) {
	existing := idx.find(entry.EmojiID, entry.EmojiName)
	if existing == nil {
		existing = idx.findByName(asset.Name)
	}

	imgData, err := fetchEmojiImage(ctx, client, asset.URL, entry)
	if err != nil {
		return syncEmojiAssetResult{}, err
	}

	if imgData.NotModified {
		canKeepExisting := existing != nil && !existing.Animated &&
			strings.EqualFold(existing.Name, asset.Name) &&
			strings.TrimSpace(entry.ContentHash) != ""
		if canKeepExisting {
			icon := formatDiscordIcon(existing)
			return syncEmojiAssetResult{
				manifestEntry: buildManifestEntry(asset, existing, icon, imgData, entry.ContentHash),
				icon:          icon,
				kept:          true,
			}, nil
		}

		// Existing emoji state doesn't match current expectations; fetch body and recreate.
		imgData, err = fetchEmojiImage(ctx, client, asset.URL, EmojiEntry{})
		if err != nil {
			return syncEmojiAssetResult{}, err
		}
	}

	unchanged := existing != nil &&
		!existing.Animated &&
		strings.EqualFold(existing.Name, asset.Name) &&
		entry.ContentHash == imgData.ContentHash

	finalEmoji := existing
	result := syncEmojiAssetResult{kept: unchanged}
	if !unchanged {
		if existing != nil {
			_ = deleteEmojiWithRetry(ctx, s, appID, existing.ID)
			idx.delete(existing.ID)
		}

		finalEmoji, err = createEmoji(ctx, s, appID, asset.Name, imgData.DataURI)
		if err != nil {
			return syncEmojiAssetResult{}, err
		}
		idx.upsert(finalEmoji)
		result.created = true
	}

	icon := formatDiscordIcon(finalEmoji)
	result.icon = icon
	result.manifestEntry = buildManifestEntry(asset, finalEmoji, icon, imgData, imgData.ContentHash)
	return result, nil
}

func buildManifestEntry(asset emojiAsset, emoji *discordgo.Emoji, icon string, img fetchedImage, hash string) EmojiEntry {
	entry := EmojiEntry{
		AssetKey:            asset.manifestKey(),
		Kind:                asset.Kind.String(),
		Tier:                strings.ToLower(asset.Tier),
		EmojiName:           emoji.Name,
		EmojiID:             emoji.ID,
		DiscordIcon:         icon,
		SourceURL:           asset.URL,
		ContentHash:         strings.TrimSpace(hash),
		SourceETag:          strings.TrimSpace(img.SourceETag),
		SourceLastModified:  strings.TrimSpace(img.SourceLastModified),
		SourceContentLength: img.SourceContentLength,
	}
	if asset.ID > 0 {
		entry.AssetID = new(asset.ID)
	}
	return entry
}

// --- Image Handling ---
type fetchedImage struct {
	DataURI, ContentHash, SourceETag, SourceLastModified string
	NotModified                                          bool
	SourceContentLength                                  int64
}

func fetchEmojiImage(ctx context.Context, client *http.Client, url string, previous EmojiEntry) (img fetchedImage, err error) {
	if strings.HasPrefix(url, localRankSourcePrefix) {
		return fetchLocalRankImage(url, previous)
	}

	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fetchedImage{}, err
	}
	if etag := strings.TrimSpace(previous.SourceETag); etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified := strings.TrimSpace(previous.SourceLastModified); lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fetchedImage{}, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()

	if resp.StatusCode == http.StatusNotModified {
		img = fetchedImage{
			ContentHash:         strings.TrimSpace(previous.ContentHash),
			NotModified:         true,
			SourceETag:          firstNonEmpty(resp.Header.Get("ETag"), previous.SourceETag),
			SourceLastModified:  firstNonEmpty(resp.Header.Get("Last-Modified"), previous.SourceLastModified),
			SourceContentLength: previous.SourceContentLength,
		}
		if contentLength := responseContentLength(resp); contentLength > 0 {
			img.SourceContentLength = contentLength
		}
		return img, nil
	}

	if resp.StatusCode != http.StatusOK {
		return fetchedImage{}, fmt.Errorf("status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxEmojiImageBytes+1))
	if err != nil {
		return fetchedImage{}, err
	}

	contentType := resp.Header.Get("Content-Type")
	if len(data) > maxEmojiImageBytes {
		return fetchedImage{}, fmt.Errorf("image too large: %d", len(data))
	}
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	hash := sha256.Sum256(data)
	b64 := base64.StdEncoding.EncodeToString(data)
	img = fetchedImage{
		DataURI:             fmt.Sprintf("data:%s;base64,%s", contentType, b64),
		ContentHash:         hex.EncodeToString(hash[:]),
		SourceETag:          strings.TrimSpace(resp.Header.Get("ETag")),
		SourceLastModified:  strings.TrimSpace(resp.Header.Get("Last-Modified")),
		SourceContentLength: responseContentLength(resp),
	}
	if img.SourceContentLength <= 0 {
		img.SourceContentLength = int64(len(data))
	}
	return img, nil
}

func responseContentLength(resp *http.Response) int64 {
	if resp == nil {
		return 0
	}
	if resp.ContentLength > 0 {
		return resp.ContentLength
	}
	raw := strings.TrimSpace(resp.Header.Get("Content-Length"))
	if raw == "" {
		return 0
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func fetchLocalRankImage(url string, previous EmojiEntry) (fetchedImage, error) {
	filename, err := localRankFilename(url)
	if err != nil {
		return fetchedImage{}, err
	}

	dataBytes, err := data.RankIconsFS.ReadFile("ranks/" + filename)
	if err != nil {
		return fetchedImage{}, fmt.Errorf("load rank icon %q: %w", filename, err)
	}
	if len(dataBytes) > maxEmojiImageBytes {
		return fetchedImage{}, fmt.Errorf("image too large: %d", len(dataBytes))
	}

	hash := sha256.Sum256(dataBytes)
	contentHash := hex.EncodeToString(hash[:])
	img := fetchedImage{
		ContentHash:         contentHash,
		SourceContentLength: int64(len(dataBytes)),
	}
	if strings.TrimSpace(previous.ContentHash) == contentHash {
		img.NotModified = true
		return img, nil
	}

	contentType := http.DetectContentType(dataBytes)
	img.DataURI = fmt.Sprintf("data:%s;base64,%s", contentType, base64.StdEncoding.EncodeToString(dataBytes))
	return img, nil
}

func localRankFilename(url string) (string, error) {
	filename := strings.TrimSpace(strings.TrimPrefix(url, localRankSourcePrefix))
	if filename == "" || strings.Contains(filename, "/") || strings.Contains(filename, "\\") {
		return "", fmt.Errorf("invalid local rank url: %q", url)
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".png") {
		return "", fmt.Errorf("invalid local rank file extension: %q", filename)
	}
	return filename, nil
}

// --- Discord Helpers ---
var deleteEmojiWithRetry = deleteEmoji

func createEmoji(ctx context.Context, s *discordgo.Session, appID, name, data string) (*discordgo.Emoji, error) {
	var emoji *discordgo.Emoji
	err := retry(ctx, func() error {
		var e error
		emoji, e = s.ApplicationEmojiCreate(appID, &discordgo.EmojiParams{Name: name, Image: data})
		return e
	})
	return emoji, err
}

func deleteEmoji(ctx context.Context, s *discordgo.Session, appID, emojiID string) error {
	return retry(ctx, func() error {
		err := s.ApplicationEmojiDelete(appID, emojiID)
		if err != nil && strings.Contains(err.Error(), "404") {
			return nil
		}
		return err
	})
}

func retry(ctx context.Context, fn func() error) error {
	var err error
	for i := range emojiMaxRetries {
		if err = fn(); err == nil {
			return nil
		}
		// Simple exponential backoff for rate limits
		timer := time.NewTimer(time.Duration(1<<i) * 300 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return err
}

type emojiIndex struct {
	byID   map[string]*discordgo.Emoji
	byName map[string]*discordgo.Emoji
	mu     sync.RWMutex
}

func newEmojiIndex(list []*discordgo.Emoji) *emojiIndex {
	idx := &emojiIndex{
		byID:   make(map[string]*discordgo.Emoji),
		byName: make(map[string]*discordgo.Emoji),
	}
	for _, e := range list {
		idx.upsert(e)
	}
	return idx
}

func (i *emojiIndex) upsert(e *discordgo.Emoji) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.byID[e.ID] = e
	i.byName[strings.ToLower(e.Name)] = e
}

func (i *emojiIndex) delete(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if e, ok := i.byID[id]; ok {
		delete(i.byName, strings.ToLower(e.Name))
		delete(i.byID, id)
	}
}

func (i *emojiIndex) find(id, name string) *discordgo.Emoji {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if e, ok := i.byID[id]; ok {
		return e
	}
	return i.byName[strings.ToLower(name)]
}

func (i *emojiIndex) findByName(name string) *discordgo.Emoji {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.byName[strings.ToLower(name)]
}

func buildRankEmojiAssets() []emojiAsset {
	tiers := []struct{ T, N, F string }{
		{"unranked", "Unranked", "unranked.png"}, {"iron", "Iron", "iron.png"}, {"bronze", "Bronze", "bronze.png"},
		{"silver", "Silver", "silver.png"}, {"gold", "Gold", "gold.png"}, {"platinum", "Platinum", "platinum.png"},
		{"emerald", "Emerald", "emerald.png"}, {"diamond", "Diamond", "diamond.png"}, {"master", "Master", "master.png"},
		{"grandmaster", "Grandmaster", "grandmaster.png"}, {"challenger", "Challenger", "challenger.png"},
	}
	out := make([]emojiAsset, 0, len(tiers))
	for _, t := range tiers {
		out = append(out, emojiAsset{
			Kind: emojiKindRankTier, Tier: t.T, Name: t.N,
			URL: localRankSourcePrefix + t.F,
		})
	}
	return out
}

func buildNonRankEmojiAssets(ctx context.Context, client *Client, ver string) ([]emojiAsset, error) {
	bundle, err := fetchCoreAssets(ctx, client, ver, emojiLocale, false)
	if err != nil {
		return nil, err
	}

	assets := make([]emojiAsset, 0, 300)
	for _, c := range bundle.Champions.Data {
		id, _ := strconv.Atoi(c.Key)
		assets = append(assets, emojiAsset{emojiKindChampion, id, "", c.ID, fmt.Sprintf("%s/%s/img/champion/%s", BaseURL, ver, c.Image.Full)})
	}
	for _, s := range bundle.SummonerSpells.Data {
		id, _ := strconv.Atoi(s.Key)
		assets = append(assets, emojiAsset{emojiKindSummonerSpell, id, "", s.ID, fmt.Sprintf("%s/%s/img/spell/%s", BaseURL, ver, s.Image.Full)})
	}
	for _, t := range bundle.Runes {
		assets = append(assets, emojiAsset{emojiKindRuneTree, t.ID, "", t.Key, fmt.Sprintf("%s/%s", runeImageBaseURL, t.Icon)})
		for _, s := range t.Slots {
			for _, r := range s.Runes {
				assets = append(assets, emojiAsset{emojiKindRune, r.ID, "", r.Key, fmt.Sprintf("%s/%s", runeImageBaseURL, r.Icon)})
			}
		}
	}
	return assets, nil
}

func formatDiscordIcon(e *discordgo.Emoji) string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("<:%s:%s>", e.Name, e.ID)
}

func updateIcons(icons *DiscordIcons, a emojiAsset, icon string) {
	switch a.Kind {
	case emojiKindChampion:
		icons.Champions[a.ID] = icon
	case emojiKindSummonerSpell:
		icons.SummonerSpells[a.ID] = icon
	case emojiKindRuneTree:
		icons.RuneTrees[a.ID] = icon
	case emojiKindRune:
		icons.Runes[a.ID] = icon
	case emojiKindRankTier:
		icons.Ranks[strings.ToLower(a.Tier)] = icon
	}
}
