-- MTG Meta Tracker — database schema (PostgreSQL)
--
-- Single source of truth for the schema. Hand-maintained and fully idempotent
-- (CREATE ... IF NOT EXISTS): the backend embeds this file and applies it on every
-- startup via store.EnsureSchema, so a fresh or partially-initialized database
-- converges to this shape without touching existing data.
--
-- Editing the schema: change this file directly. IF NOT EXISTS is additive-only — it
-- never ALTERs an existing table, so changing a column/constraint on an already-created
-- table needs a manual ALTER or a dev DB reset (remove the postgres data dir + restart).
-- `make db-schema` writes a pg_dump to a scratch file for diffing only; do not paste its
-- non-idempotent output over this file. Back up / restore data with `make db-dump` / `make db-restore`.
--
-- Color identity is a 5-bit bitset: W=1 U=2 B=4 R=8 G=16 (colorless=0).

CREATE EXTENSION IF NOT EXISTS "pgcrypto";  -- gen_random_uuid()

-- ---------------------------------------------------------------------------
-- Users, auth
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username      text NOT NULL UNIQUE,
    email         text NOT NULL UNIQUE,
    display_name  text NOT NULL,
    bio           text,
    avatar_url    text,
    role          text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    password_hash text,                       -- NULL for OAuth-only accounts
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS oauth_accounts (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id             uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider            text NOT NULL CHECK (provider IN ('google')),
    provider_account_id text NOT NULL,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_account_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    id         text PRIMARY KEY,             -- opaque random token (cookie value)
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);

-- Admin-invites-only onboarding: no open registration. An admin creates an
-- invite; the invitee redeems the token to set username + password.
CREATE TABLE IF NOT EXISTS invites (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email      text NOT NULL,
    role       text NOT NULL DEFAULT 'user' CHECK (role IN ('user','admin')),
    token_hash text NOT NULL UNIQUE,               -- sha256 of the raw token
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invites_open ON invites(lower(email)) WHERE accepted_at IS NULL;

-- ---------------------------------------------------------------------------
-- Cube (card pool) + Scryfall card cache
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS cubes (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name             text NOT NULL,
    moxfield_public_id text UNIQUE,   -- kept as display-only metadata (link back to the source deck)
    description      text,
    card_list        text,            -- raw pasted decklist (standard format); source of truth for the pool
    content_hash     text,            -- fingerprint of last-built card_list; skip re-resolve when unchanged
    last_synced_at   timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS cards (
    scryfall_id    uuid PRIMARY KEY,
    oracle_id      uuid,
    name           text NOT NULL,
    mana_cost      text,
    cmc            numeric,
    type_line      text,
    oracle_text    text,
    colors         smallint NOT NULL DEFAULT 0,          -- bitset
    color_identity smallint NOT NULL DEFAULT 0,          -- bitset
    rarity         text,
    layout         text,
    image_small    text,
    image_normal   text,
    image_art_crop text,
    set_code       text,
    collector_number text,
    raw            jsonb,                                 -- full Scryfall payload
    -- URL slug for /cards/<slug>. Generated, so it can never drift from the name:
    -- "Jace, the Mind Sculptor" -> jace-the-mind-sculptor, "Fire // Ice" -> fire-ice.
    -- Not unique — two printings of a name are two rows — so slug lookups tie-break
    -- (see store.GetCardBySlug).
    slug           text GENERATED ALWAYS AS
                     (btrim(regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g'), '-')) STORED,
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_cards_name        ON cards(lower(name));
CREATE INDEX IF NOT EXISTS idx_cards_color_ident ON cards(color_identity);
-- idx_cards_slug is created in the migrations section below: on an already-created
-- cards table the CREATE TABLE above is a no-op, so slug does not exist until the
-- ALTER runs, and indexing it here would fail on every existing database.

CREATE TABLE IF NOT EXISTS cube_cards (
    cube_id    uuid NOT NULL REFERENCES cubes(id) ON DELETE CASCADE,
    card_id    uuid NOT NULL REFERENCES cards(scryfall_id) ON DELETE CASCADE,
    is_active  boolean NOT NULL DEFAULT true,
    added_at   timestamptz NOT NULL DEFAULT now(),
    removed_at timestamptz,
    PRIMARY KEY (cube_id, card_id)
);
CREATE INDEX IF NOT EXISTS idx_cube_cards_active ON cube_cards(cube_id) WHERE is_active;

-- ---------------------------------------------------------------------------
-- Decklists (metadata + list + record, all together)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS decklists (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cube_id           uuid NOT NULL REFERENCES cubes(id) ON DELETE CASCADE,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name              text NOT NULL,
    description       text,
    color_identity    smallint NOT NULL DEFAULT 0,       -- inferred bitset
    archetype         text
                        CHECK (archetype IN ('aggro','control','midrange','tempo','combo')),
    source_url        text,
    decklist_raw      text NOT NULL,                     -- raw list text
    card_count        int  NOT NULL DEFAULT 0,
    status            text NOT NULL DEFAULT 'active'
                        CHECK (status IN ('draft','active','archived')),
    -- record (nullable / added after the fact)
    games_played      int NOT NULL DEFAULT 0,
    wins              int NOT NULL DEFAULT 0,
    losses            int NOT NULL DEFAULT 0,
    event_name        text,
    played_at         date,
    record_updated_at timestamptz,
    winrate           numeric GENERATED ALWAYS AS
        (CASE WHEN games_played > 0 THEN wins::numeric / games_played END) STORED,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT decklists_record_check CHECK (wins + losses <= games_played)
);
CREATE INDEX IF NOT EXISTS idx_decklists_user      ON decklists(user_id);
CREATE INDEX IF NOT EXISTS idx_decklists_cube      ON decklists(cube_id);
CREATE INDEX IF NOT EXISTS idx_decklists_color     ON decklists(color_identity);
CREATE INDEX IF NOT EXISTS idx_decklists_archetype ON decklists(archetype);

CREATE TABLE IF NOT EXISTS decklist_cards (
    decklist_id uuid NOT NULL REFERENCES decklists(id) ON DELETE CASCADE,
    card_id     uuid REFERENCES cards(scryfall_id),      -- NULL if unresolved
    card_name   text NOT NULL,                           -- as written in the list
    quantity    int  NOT NULL DEFAULT 1,
    is_resolved boolean NOT NULL DEFAULT false,
    board       text NOT NULL DEFAULT 'main'
                  CHECK (board IN ('main','side','maybe')),
    PRIMARY KEY (decklist_id, card_name, board)
);
CREATE INDEX IF NOT EXISTS idx_decklist_cards_card ON decklist_cards(card_id);

-- ---------------------------------------------------------------------------
-- Analytics snapshots (see docs/DESIGN.md §4)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS analytics_runs (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cube_id        uuid NOT NULL REFERENCES cubes(id) ON DELETE CASCADE,
    trigger        text NOT NULL,             -- deck_created|deck_updated|record_updated|cube_synced|manual|scheduled
    status         text NOT NULL DEFAULT 'running'
                     CHECK (status IN ('running','ok','failed')),
    decks_included int  NOT NULL DEFAULT 0,
    games_included int  NOT NULL DEFAULT 0,
    is_current     boolean NOT NULL DEFAULT false,
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz
);
-- at most one current run per cube
CREATE UNIQUE INDEX IF NOT EXISTS idx_runs_current ON analytics_runs(cube_id) WHERE is_current;

CREATE TABLE IF NOT EXISTS color_stats (
    run_id        uuid NOT NULL REFERENCES analytics_runs(id) ON DELETE CASCADE,
    facet         text NOT NULL CHECK (facet IN ('exact_identity','single_color','color_count')),
    facet_key     smallint NOT NULL,
    deck_count    int NOT NULL,
    games         int NOT NULL DEFAULT 0,
    wins          int NOT NULL DEFAULT 0,
    losses        int NOT NULL DEFAULT 0,
    winrate       numeric,
    PRIMARY KEY (run_id, facet, facet_key)
);

CREATE TABLE IF NOT EXISTS card_stats (
    run_id         uuid NOT NULL REFERENCES analytics_runs(id) ON DELETE CASCADE,
    card_id        uuid NOT NULL REFERENCES cards(scryfall_id),
    deck_count     int NOT NULL,
    inclusion_rate numeric NOT NULL,
    games          int NOT NULL DEFAULT 0,
    wins           int NOT NULL DEFAULT 0,
    losses         int NOT NULL DEFAULT 0,
    winrate        numeric,           -- raw
    winrate_shrunk numeric,           -- Bayesian-smoothed toward global mean
    winrate_lift   numeric,           -- winrate_shrunk - global_winrate
    wilson_lower   numeric,           -- ranking-safe lower bound
    PRIMARY KEY (run_id, card_id)
);
CREATE INDEX IF NOT EXISTS idx_card_stats_lift ON card_stats(run_id, winrate_lift DESC);

CREATE TABLE IF NOT EXISTS card_pair_stats (
    run_id       uuid NOT NULL REFERENCES analytics_runs(id) ON DELETE CASCADE,
    card_a_id    uuid NOT NULL REFERENCES cards(scryfall_id),
    card_b_id    uuid NOT NULL REFERENCES cards(scryfall_id),
    co_count     int NOT NULL,
    support      numeric NOT NULL,
    confidence_ab numeric NOT NULL,   -- P(B | A)
    lift         numeric NOT NULL,
    pair_winrate numeric,
    PRIMARY KEY (run_id, card_a_id, card_b_id),
    CHECK (card_a_id <> card_b_id)
);
CREATE INDEX IF NOT EXISTS idx_pair_stats_a ON card_pair_stats(run_id, card_a_id, lift DESC);

CREATE TABLE IF NOT EXISTS meta_snapshot (
    run_id          uuid PRIMARY KEY REFERENCES analytics_runs(id) ON DELETE CASCADE,
    total_decks     int NOT NULL,
    total_games     int NOT NULL,
    overall_winrate numeric,
    avg_cmc         numeric,
    avg_color_count numeric,
    mono_share      numeric,
    multi_share     numeric
);

CREATE TABLE IF NOT EXISTS deck_metric_stats (
    run_id     uuid NOT NULL REFERENCES analytics_runs(id) ON DELETE CASCADE,
    metric     text NOT NULL,          -- e.g. 'avg_cmc', 'color_count', 'creature_count'
    bucket     text NOT NULL,          -- bucket label
    deck_count int NOT NULL,
    winrate    numeric,
    PRIMARY KEY (run_id, metric, bucket)
);

-- ---------------------------------------------------------------------------
-- Job queue (trigger-driven recompute + scheduled syncs)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS jobs (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type         text NOT NULL,        -- recompute_analytics | sync_cube | refresh_cards
    payload      jsonb NOT NULL DEFAULT '{}'::jsonb,
    status       text NOT NULL DEFAULT 'pending'
                   CHECK (status IN ('pending','running','done','failed')),
    dedup_key    text,                 -- coalesce bursts: unique among pending
    attempts     int NOT NULL DEFAULT 0,
    last_error   text,
    scheduled_at timestamptz NOT NULL DEFAULT now(),
    started_at   timestamptz,
    finished_at  timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
-- only one pending job per dedup_key
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_dedup ON jobs(dedup_key) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_pending ON jobs(scheduled_at) WHERE status = 'pending';

-- ---------------------------------------------------------------------------
-- Idempotent migrations for existing databases
-- ---------------------------------------------------------------------------
ALTER TABLE cubes ADD COLUMN IF NOT EXISTS content_hash text;
ALTER TABLE cubes ADD COLUMN IF NOT EXISTS card_list text;

-- Card URL slug. Backfills itself on add (STORED generated column), so an existing
-- database gets slugs for every cached card the first time the backend boots.
ALTER TABLE cards ADD COLUMN IF NOT EXISTS slug text GENERATED ALWAYS AS
    (btrim(regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g'), '-')) STORED;
CREATE INDEX IF NOT EXISTS idx_cards_slug ON cards(slug);

-- Per-cube progress for the admin "Sync Scryfall images" action. One row per
-- cube, upserted on each sync (the sync_cube:<id> dedup key means at most one
-- active sync per cube). The admin page polls this to show live progress; the
-- image-download phase runs detached from the job and updates images_done here,
-- so this row — not the job's status — is the source of truth for "finished".
CREATE TABLE IF NOT EXISTS cube_sync_progress (
    cube_id       uuid PRIMARY KEY REFERENCES cubes(id) ON DELETE CASCADE,
    status        text NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued','resolving','downloading','done','failed')),
    cards_total   int NOT NULL DEFAULT 0,
    images_total  int NOT NULL DEFAULT 0,
    images_done   int NOT NULL DEFAULT 0,
    images_failed int NOT NULL DEFAULT 0,
    error         text,
    -- Names from the pasted card_list that Scryfall could not resolve. They are
    -- dropped from the pool, so the admin page surfaces them rather than letting
    -- a typo silently shrink the cube. Holds the result of the last resolve.
    unresolved    text[] NOT NULL DEFAULT '{}',
    started_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz
);
ALTER TABLE cube_sync_progress ADD COLUMN IF NOT EXISTS unresolved text[] NOT NULL DEFAULT '{}';

-- Archetype used to be a free-text tag. Normalize what is already there (case/whitespace,
-- then anything still off-list to NULL) before adding the CHECK: a constraint that any row
-- violates would fail EnsureSchema, and that runs on boot, so the server would not start.
UPDATE decklists SET archetype = lower(btrim(archetype))
    WHERE archetype IS NOT NULL AND archetype <> lower(btrim(archetype));
UPDATE decklists SET archetype = NULL
    WHERE archetype IS NOT NULL
      AND archetype NOT IN ('aggro','control','midrange','tempo','combo');
ALTER TABLE decklists DROP CONSTRAINT IF EXISTS decklists_archetype_check;
ALTER TABLE decklists ADD CONSTRAINT decklists_archetype_check
    CHECK (archetype IN ('aggro','control','midrange','tempo','combo'));

-- Draws and placement are no longer collected. Dropping decklists.draws also drops the
-- old unnamed CHECK that referenced it, so the record check is re-added under a stable
-- name (drop-then-add: Postgres has no ADD CONSTRAINT IF NOT EXISTS).
ALTER TABLE decklists   DROP COLUMN IF EXISTS draws;
ALTER TABLE decklists   DROP COLUMN IF EXISTS placement;
ALTER TABLE decklists   DROP CONSTRAINT IF EXISTS decklists_record_check;
ALTER TABLE decklists   ADD  CONSTRAINT decklists_record_check
                          CHECK (wins + losses <= games_played);
ALTER TABLE color_stats DROP COLUMN IF EXISTS draws;
ALTER TABLE color_stats DROP COLUMN IF EXISTS avg_placement;
ALTER TABLE card_stats  DROP COLUMN IF EXISTS draws;
