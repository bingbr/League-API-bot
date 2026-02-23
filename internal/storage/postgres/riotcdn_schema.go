package postgres

const createRiotCDNQueuesSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_queues (
    queue_id int PRIMARY KEY,
    name text NOT NULL,
    game_select_category text NOT NULL,
    data jsonb NOT NULL,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNMapsSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_maps (
    map_id int PRIMARY KEY,
    name text NOT NULL,
    data jsonb NOT NULL,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNChampionsSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_champions (
    champion_id int PRIMARY KEY,
    version text NOT NULL,
    name text,
    discord_icon text,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNItemsSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_items (
    item_id text PRIMARY KEY,
    version text NOT NULL,
    name text,
    plaintext text,
    tags text[] NOT NULL DEFAULT '{}',
    data jsonb,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNSummonerSpellsSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_summoner_spells (
    spell_id int PRIMARY KEY,
    version text NOT NULL,
    name text,
    discord_icon text,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNRuneTreesSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_rune_trees (
    tree_id int PRIMARY KEY,
    key text NOT NULL,
    name text,
    icon text NOT NULL,
    discord_icon text,
    data jsonb,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNRunesSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_runes (
    rune_id int PRIMARY KEY,
    tree_id int NOT NULL,
    key text NOT NULL,
    name text,
    icon text NOT NULL,
    discord_icon text,
    short_desc text,
    long_desc text,
    data jsonb,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNRankedTiersSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_ranked_tiers (
    tier text PRIMARY KEY,
    discord_icon text NOT NULL,
    fetched_at timestamptz NOT NULL
)`

const createRiotCDNEmojiSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_emoji_manifest (
    asset_key text PRIMARY KEY,
    kind text NOT NULL,
    asset_id int,
    tier text,
    emoji_name text NOT NULL,
    emoji_id text NOT NULL,
    discord_icon text NOT NULL,
    source_url text NOT NULL,
    content_hash text NOT NULL,
    source_etag text,
    source_last_modified text,
    source_content_length bigint,
    updated_at timestamptz NOT NULL
)`

const createRiotCDNSyncStateSQL = `
CREATE TABLE IF NOT EXISTS riot_cdn_sync_state (
    sync_key text PRIMARY KEY,
    version text NOT NULL,
    updated_at timestamptz NOT NULL
)`

const updateRiotCDNChampionDiscordIconSQL = `
UPDATE riot_cdn_champions
SET discord_icon = $2,
    fetched_at = GREATEST(fetched_at, $3)
WHERE champion_id = $1`

const updateRiotCDNSummonerSpellDiscordIconSQL = `
UPDATE riot_cdn_summoner_spells
SET discord_icon = $2,
    fetched_at = GREATEST(fetched_at, $3)
WHERE spell_id = $1`

const updateRiotCDNRuneTreeDiscordIconSQL = `
UPDATE riot_cdn_rune_trees
SET discord_icon = $2,
    fetched_at = GREATEST(fetched_at, $3)
WHERE tree_id = $1`

const updateRiotCDNRuneDiscordIconSQL = `
UPDATE riot_cdn_runes
SET discord_icon = $2,
    fetched_at = GREATEST(fetched_at, $3)
WHERE rune_id = $1`

const upsertRiotCDNRankedTierDiscordIconSQL = `
INSERT INTO riot_cdn_ranked_tiers (tier, discord_icon, fetched_at)
VALUES ($1, $2, $3)
ON CONFLICT (tier) DO UPDATE
SET discord_icon = excluded.discord_icon,
    fetched_at = GREATEST(riot_cdn_ranked_tiers.fetched_at, excluded.fetched_at)`
