// Fetch helpers. Server components hit the backend origin directly; client
// components use the same-origin /api rewrite so the session cookie is sent.

import type { Archetype } from "./decklist";

function base(): string {
  if (typeof window === "undefined") {
    return process.env.BACKEND_ORIGIN ?? "http://localhost:8080";
  }
  return "";
}

// A revalidate of 0 means never cache. The index pages render per request and must
// reflect the live database: an ISR entry there would be built during `next build`,
// where the backend is unreachable, and the resulting empty page would be served
// until the window expired.
function cacheOpts(revalidate: number): RequestInit {
  return revalidate === 0 ? { cache: "no-store" } : { next: { revalidate } };
}

export async function apiGet<T>(path: string, revalidate = 60): Promise<T> {
  const res = await fetch(base() + "/api" + path, cacheOpts(revalidate));
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json() as Promise<T>;
}

// apiGetOptional returns null on any non-2xx (e.g. 404 when no analytics yet) or
// when the backend is unreachable (e.g. during a build with no server running).
export async function apiGetOptional<T>(path: string, revalidate = 60): Promise<T | null> {
  try {
    const res = await fetch(base() + "/api" + path, cacheOpts(revalidate));
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch (e) {
    // A page that renders empty because the backend was down is indistinguishable
    // from one that renders empty because there is no data. Leave a trace.
    console.warn(`GET ${path}: backend unreachable`, e);
    return null;
  }
}

// apiGetNoStore is a client-only GET that skips caching and sends the session
// cookie — for polling authenticated, fast-changing endpoints (e.g. sync status)
// where apiGet's ISR caching and cookie-less fetch are both wrong.
export async function apiGetNoStore<T>(path: string): Promise<T> {
  const res = await fetch("/api" + path, { credentials: "include", cache: "no-store" });
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json() as Promise<T>;
}

async function mutate<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch("/api" + path, {
    method,
    headers: { "Content-Type": "application/json" },
    credentials: "include",
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = `${method} ${path}: ${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      // ignore
    }
    throw new Error(msg);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export const apiPost = <T>(path: string, body?: unknown) => mutate<T>("POST", path, body);
export const apiPatch = <T>(path: string, body?: unknown) => mutate<T>("PATCH", path, body);
export const apiDelete = <T>(path: string) => mutate<T>("DELETE", path);

// --- shared types (mirror the Go JSON views) ---

export interface Cube {
  id: string;
  name: string;
  moxfield_public_id?: string;
  description?: string;
  card_list?: string;
  last_synced_at?: string;
}
export interface CubeView {
  cube: Cube;
  // Copies in the pool, and the distinct printings behind them. Equal for a
  // singleton cube; a themed cube can run many copies of one card.
  card_count: number;
  unique_count: number;
}
// Live progress of the admin "Sync Scryfall images" action (mirrors the Go
// store.CubeSyncProgress row). "none" is returned for a never-synced cube.
export interface CubeSyncStatus {
  status: "none" | "queued" | "resolving" | "downloading" | "done" | "failed";
  cards_total?: number;
  images_total?: number;
  images_done?: number;
  images_failed?: number;
  error?: string | null;
  // Names from the pasted list that Scryfall could not resolve. They are absent
  // from the pool, so the admin page must show them.
  unresolved?: string[];
  started_at?: string;
  finished_at?: string | null;
}
export interface CubeCard {
  card_id: string;
  card_name: string;
  slug: string;
  mana_cost?: string;
  cmc?: number;
  type_line?: string;
  color_identity: number;
  group_colors: number;
  // Copies in the pool. Usually 1 — a cube is normally singleton — but a themed
  // cube can run 150 of something, and CardFan badges anything above 1.
  quantity: number;
  image_normal?: string;
  image_art_crop?: string;
  // The exact printing — addresses the card on Scryfall.
  set_code?: string;
  collector_number?: string;
}

export interface Decklist {
  id: string;
  cube_id: string;
  user_id: string;
  name: string;
  description?: string;
  color_identity: number;
  // The colors the deck only splashes (under 10% of its nonlands). Disjoint from
  // color_identity, and left out of the meta's color analytics.
  splash_colors: number;
  archetype?: Archetype;
  source_url?: string;
  decklist_raw: string;
  card_count: number;
  status: string;
  // The day the deck was played. Always set — it defaults to the upload date — and
  // served as an RFC3339 timestamp whose time is a meaningless midnight UTC, so read
  // the calendar day off the string (see lib/format.ts) rather than through Date.
  played_at: string;
  games_played: number;
  wins: number;
  losses: number;
  event_name?: string;
  record_updated_at?: string;
  winrate?: number;
  created_at: string;
}
export interface DecklistCard {
  card_id?: string;
  card_name: string;
  // Absent for an unresolved card — there is no cards row, so nothing to link to.
  slug?: string;
  quantity: number;
  is_resolved: boolean;
  board: string;
  image_art_crop?: string;
  image_normal?: string;
  cmc?: number;
  type_line?: string;
  color_identity?: number;
  group_colors?: number;
  // The exact printing — addresses the card on Scryfall.
  set_code?: string;
  collector_number?: string;
}
// The server's own calendar day, in the playgroup's timezone. A date picker opens on
// it so the form and the backend's default agree — the browser's today is its own
// timezone's, which is a different day for anyone travelling.
export interface Today {
  date: string;
  timezone: string;
}
export interface PublicUser {
  id: string;
  username: string;
  display_name: string;
  bio?: string;
  avatar_url?: string;
  role: string;
}
export interface DecklistListItem {
  decklist: Decklist;
  color_string: string;
}
export interface DecklistDetail {
  decklist: Decklist;
  color_string: string;
  cards: DecklistCard[];
  user?: PublicUser;
  // Names the save could not match to a card. Present on the create/update
  // responses; absent on plain reads.
  unresolved?: string[];
}

export interface RunMeta {
  id: string;
  cube_id: string;
  trigger: string;
  status: string;
  decks_included: number;
  games_included: number;
  started_at: string;
  finished_at?: string;
}
export interface MetaSnapshot {
  total_decks: number;
  total_games: number;
  overall_winrate: number | null;
  avg_cmc: number | null;
  avg_color_count: number | null;
  mono_share: number | null;
  multi_share: number | null;
}
export interface Overview {
  run: RunMeta;
  meta: MetaSnapshot;
}
export interface ColorStat {
  facet: string;
  facet_key: number;
  deck_count: number;
  games: number;
  wins: number;
  losses: number;
  winrate: number | null;
}
// One color's standing on one day of the color trend.
export interface ColorTrendColor {
  color: number; // a single WUBRG bit
  deck_count: number;
  // 0..1 of that day's color pie — normalized across the five colors, not against
  // total_decks, since a two-color deck plays two of them. Null on a day whose decks
  // are all colorless, where there is no pie to take a slice of.
  share: number | null;
}
// One day of the color trend, with every color present in WUBRG order — including the
// ones at zero, so the bands of a stacked area have a point at every x.
export interface ColorTrendPoint {
  as_of: string; // "2026-07-24" — a calendar day, never a timestamp
  total_decks: number;
  colors: ColorTrendColor[];
}

export interface CardStat {
  card_id: string;
  name: string;
  slug: string;
  image_normal?: string;
  image_art_crop?: string;
  color_identity: number;
  deck_count: number;
  inclusion_rate: number;
  games: number;
  wins: number;
  winrate: number | null;
}
export interface CardPair {
  card_b_id: string;
  name: string;
  slug: string;
  color_identity: number;
  co_count: number;
  pair_winrate: number | null;
}

// --- card detail (/cards/<slug>) ---

export interface Card {
  card_id: string;
  name: string;
  slug: string;
  mana_cost?: string;
  cmc?: number;
  type_line?: string;
  oracle_text?: string;
  color_identity: number;
  rarity?: string;
  image_normal?: string;
  image_art_crop?: string;
}
export interface DeckBrief {
  id: string;
  name: string;
  color_identity: number;
  splash_colors: number;
  quantity: number;
  games_played: number;
  wins: number;
  losses: number;
  winrate: number | null;
  owner?: string;
}
export interface CardDetail {
  card: Card;
  cube_id: string;
  in_pool: boolean;
  // null when the card is in no analyzed deck, or the cube has no analytics run yet.
  stat: CardStat | null;
  rank_by_inclusion: number | null;
  total_ranked: number;
  color_split: ColorStat[];
  color_count_split: ColorStat[];
  pairs: CardPair[];
  decks: DeckBrief[];
}

export interface InferResult {
  color_identity: number;
  color_string: string;
  splash_colors: number;
  splash_string: string;
  resolved: string[] | null;
  unresolved: string[] | null;
}
