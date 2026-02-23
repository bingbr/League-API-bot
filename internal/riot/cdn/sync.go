package cdn

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	defaultTimeout = 20 * time.Second

	BaseURL         = "https://ddragon.leagueoflegends.com/cdn"
	DefaultVersion  = "latest"
	DefaultInterval = 24 * time.Hour

	versionFetchTimeout = 10 * time.Second

	communityDragonQueuesURL = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/queues.json"
	communityDragonMapsURL   = "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/maps.json"
	defaultDataLanguage      = "en_US"
)

type Client struct {
	httpClient *http.Client
}

func NewClient() *Client {
	return &Client{httpClient: &http.Client{Timeout: defaultTimeout}}
}

func (c *Client) FetchVersions(ctx context.Context) ([]string, error) {
	v, err := fetch[[]string](ctx, c.httpClient, "https://ddragon.leagueoflegends.com/api/versions.json")
	if err == nil && len(v) == 0 {
		return nil, fmt.Errorf("no versions found")
	}
	return v, err
}

func (c *Client) FetchQueues(ctx context.Context) ([]Queue, error) {
	return fetch[[]Queue](ctx, c.httpClient, communityDragonQueuesURL)
}

func (c *Client) FetchMaps(ctx context.Context) ([]GameMap, error) {
	return fetch[[]GameMap](ctx, c.httpClient, communityDragonMapsURL)
}

func (c *Client) FetchChampions(ctx context.Context, cdn, ver, loc string) (ChampionList, error) {
	return fetch[ChampionList](ctx, c.httpClient, ddragonURL(cdn, ver, loc, "champion.json"))
}

func (c *Client) FetchItems(ctx context.Context, cdn, ver, loc string) (ItemList, error) {
	return fetch[ItemList](ctx, c.httpClient, ddragonURL(cdn, ver, loc, "item.json"))
}

func (c *Client) FetchSummonerSpells(ctx context.Context, cdn, ver, loc string) (SummonerSpellList, error) {
	return fetch[SummonerSpellList](ctx, c.httpClient, ddragonURL(cdn, ver, loc, "summoner.json"))
}

func (c *Client) FetchRunes(ctx context.Context, cdn, ver, loc string) ([]RuneTree, error) {
	return fetch[[]RuneTree](ctx, c.httpClient, ddragonURL(cdn, ver, loc, "runesReforged.json"))
}

func ddragonURL(cdn, ver, loc, file string) string {
	return fmt.Sprintf("%s/%s/data/%s/%s", cdn, ver, loc, file)
}

func fetch[T any](ctx context.Context, client *http.Client, url string) (target T, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return target, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return target, err
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close response body: %w", closeErr)
		}
	}()
	if resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return target, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	if err = json.NewDecoder(resp.Body).Decode(&target); err != nil {
		return target, fmt.Errorf("decode %s: %w", url, err)
	}
	return target, nil
}

type LiveSnapshot struct {
	Cdn       string
	Version   string
	FetchedAt time.Time
}

// Global live realm used by package-level URL helpers and background refreshers.
// It remains package-scoped mutable state to keep existing helper APIs stable.
var L = NewLiveRealm()

type LiveRealm struct {
	mu        sync.RWMutex
	cdn       string
	version   string
	fetchedAt time.Time
}

func NewLiveRealm() *LiveRealm {
	return &LiveRealm{
		cdn:     BaseURL,
		version: DefaultVersion,
	}
}

func (l *LiveRealm) Update(version string, fetchedAt time.Time) {
	if l == nil {
		return
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return
	}
	l.mu.Lock()
	l.version = version
	if !fetchedAt.IsZero() {
		l.fetchedAt = fetchedAt
	}
	l.mu.Unlock()
}

func (l *LiveRealm) Snapshot() LiveSnapshot {
	if l == nil {
		return LiveSnapshot{Cdn: BaseURL, Version: DefaultVersion}
	}
	l.mu.RLock()
	s := LiveSnapshot{Cdn: l.cdn, Version: l.version, FetchedAt: l.fetchedAt}
	l.mu.RUnlock()
	return s
}

func (l *LiveRealm) ProfileIconURL(id int) string {
	s := l.Snapshot()
	return fmt.Sprintf("%s/%s/img/profileicon/%d.png", s.Cdn, s.Version, id)
}

func ProfileIconURL(id int) string {
	return L.ProfileIconURL(id)
}

// StartAutoUpdate performs an initial fetch and then starts a background refresher.
func StartAutoUpdate(ctx context.Context, interval time.Duration, log *slog.Logger) error {
	if log == nil {
		log = slog.Default()
	}
	if interval <= 0 {
		interval = DefaultInterval
	}

	update := func() error {
		fetchCtx, cancel := context.WithTimeout(ctx, versionFetchTimeout)
		defer cancel()

		versions, err := NewClient().FetchVersions(fetchCtx)
		if err != nil {
			return err
		}
		L.Update(versions[0], time.Now().UTC())
		log.Info("Riot version updated", "version", versions[0])
		return nil
	}

	if err := update(); err != nil {
		return fmt.Errorf("initial version fetch failed: %w", err)
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := update(); err != nil {
					log.Warn("Failed to refresh version", "err", err)
				}
			}
		}
	}()
	return nil
}

type Database interface {
	UpsertQueues(ctx context.Context, queues []Queue, fetchedAt time.Time) error
	UpsertMaps(ctx context.Context, maps []GameMap, fetchedAt time.Time) error
	UpsertChampions(ctx context.Context, version string, champs map[string]Champion, fetchedAt time.Time) error
	UpsertItems(ctx context.Context, version string, items map[string]Item, fetchedAt time.Time) error
	UpsertSummonerSpells(ctx context.Context, version string, spells map[string]SummonerSpell, fetchedAt time.Time) error
	UpsertRunes(ctx context.Context, trees []RuneTree, fetchedAt time.Time) error
}

type DiscordIcons struct {
	Champions      map[int]string
	SummonerSpells map[int]string
	RuneTrees      map[int]string
	Runes          map[int]string
	Ranks          map[string]string
}

type coreAssets struct {
	Champions      ChampionList
	Items          ItemList
	SummonerSpells SummonerSpellList
	Runes          []RuneTree
}

func fetchCoreAssets(ctx context.Context, c *Client, ver, loc string, includeItems bool) (coreAssets, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if c == nil {
		return coreAssets{}, fmt.Errorf("client is required")
	}
	if ver = strings.TrimSpace(ver); ver == "" {
		return coreAssets{}, fmt.Errorf("version is required")
	}
	if loc = strings.TrimSpace(loc); loc == "" {
		return coreAssets{}, fmt.Errorf("locale is required")
	}

	var a coreAssets
	g, gctx := errgroup.WithContext(ctx)
	run := func(label string, fn func() error) {
		g.Go(func() error {
			if err := fn(); err != nil {
				return fmt.Errorf("fetch %s %s: %w", label, loc, err)
			}
			return nil
		})
	}

	run("champions", func() (err error) {
		a.Champions, err = c.FetchChampions(gctx, BaseURL, ver, loc)
		return err
	})
	run("summoner spells", func() (err error) {
		a.SummonerSpells, err = c.FetchSummonerSpells(gctx, BaseURL, ver, loc)
		return err
	})
	run("runes", func() (err error) {
		a.Runes, err = c.FetchRunes(gctx, BaseURL, ver, loc)
		return err
	})
	if includeItems {
		run("items", func() (err error) {
			a.Items, err = c.FetchItems(gctx, BaseURL, ver, loc)
			return err
		})
	}

	if err := g.Wait(); err != nil {
		return coreAssets{}, err
	}
	return a, nil
}

func SyncBasicData(ctx context.Context, db Database, version string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if db == nil {
		return fmt.Errorf("database is nil")
	}
	if version = strings.TrimSpace(version); version == "" {
		return fmt.Errorf("version is required")
	}

	client := NewClient()
	fetchedAt := time.Now().UTC()
	L.Update(version, fetchedAt)

	queues, err := client.FetchQueues(ctx)
	if err != nil {
		return fmt.Errorf("fetch queues: %w", err)
	}
	if err := db.UpsertQueues(ctx, queues, fetchedAt); err != nil {
		return fmt.Errorf("database queues: %w", err)
	}
	maps, err := client.FetchMaps(ctx)
	if err != nil {
		return fmt.Errorf("fetch maps: %w", err)
	}
	if err := db.UpsertMaps(ctx, maps, fetchedAt); err != nil {
		return fmt.Errorf("database maps: %w", err)
	}

	assets, err := fetchCoreAssets(ctx, client, version, defaultDataLanguage, true)
	if err != nil {
		return err
	}

	if err := db.UpsertChampions(ctx, assets.Champions.Version, assets.Champions.Data, fetchedAt); err != nil {
		return fmt.Errorf("database champions: %w", err)
	}
	if err := db.UpsertItems(ctx, assets.Items.Version, assets.Items.Data, fetchedAt); err != nil {
		return fmt.Errorf("database items: %w", err)
	}
	if err := db.UpsertSummonerSpells(ctx, assets.SummonerSpells.Version, assets.SummonerSpells.Data, fetchedAt); err != nil {
		return fmt.Errorf("database summoner spells: %w", err)
	}
	if err := db.UpsertRunes(ctx, assets.Runes, fetchedAt); err != nil {
		return fmt.Errorf("database runes: %w", err)
	}
	return nil
}
