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

// forwardedHeaders passes the incoming request's cookies through on server-side
// fetches. Server components run in the Next.js process, not the browser, so the
// cross-origin fetch to the Go backend has no cookies of its own — every
// SSR-rendered authenticated page would 401 without this. In the browser the
// cookie is attached automatically for the same-origin /api path, so we return
// undefined and let the platform do it.
//
// A per-request fetch that carries a cookie must also opt out of Next's shared
// ISR cache, otherwise one user's rendered page would be served to another.
// This is why cookies() being called also forces the fetch into no-store mode
// for that request. Downstream we merge those two headers/cache concerns via
// serverFetchOpts below.
async function serverFetchOpts(revalidate: number): Promise<RequestInit> {
  if (typeof window !== "undefined") return cacheOpts(revalidate);
  // Dynamic import so this file remains importable from client components
  // (next/headers throws if imported into a client bundle).
  const { cookies } = await import("next/headers");
  const jar = cookies().toString();
  // A request that carries a cookie is per-user by definition; ISR is unsafe
  // for it, so we always use no-store here rather than the caller's revalidate.
  return jar
    ? { cache: "no-store", headers: { cookie: jar } }
    : cacheOpts(revalidate);
}

// When the backend says the caller has no session on a server-rendered page,
// the page cannot render. Redirect to /login rather than surface a 500 error
// boundary. On the client we do nothing here — a background poll that suddenly
// 401s should not yank the user out of what they were doing, and /auth/me
// specifically expects a 401 to mean "logged out" rather than "please log in
// right now". The client-side callers already handle null / throw themselves.
async function redirectToLoginIfServer(): Promise<void> {
  if (typeof window !== "undefined") return;
  const { redirect } = await import("next/navigation");
  redirect("/login");
}

export async function apiGet<T>(path: string, revalidate = 60): Promise<T> {
  const opts = await serverFetchOpts(revalidate);
  const res = await fetch(base() + "/api" + path, opts);
  if (res.status === 401) await redirectToLoginIfServer();
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json() as Promise<T>;
}

// apiGetOptional returns null on any non-2xx (e.g. 404 when no analytics yet) or
// when the backend is unreachable (e.g. during a build with no server running).
// On the server a 401 is turned into a /login redirect first, so an SSR page
// that requires auth never renders empty for an anonymous visitor; on the
// client 401 stays null (SessionProvider polls /auth/me precisely to observe
// that state).
export async function apiGetOptional<T>(path: string, revalidate = 60): Promise<T | null> {
  let res: Response;
  try {
    const opts = await serverFetchOpts(revalidate);
    res = await fetch(base() + "/api" + path, opts);
  } catch (e) {
    // A page that renders empty because the backend was down is indistinguishable
    // from one that renders empty because there is no data. Leave a trace.
    console.warn(`GET ${path}: backend unreachable`, e);
    return null;
  }
  if (res.status === 401) await redirectToLoginIfServer();
  if (!res.ok) return null;
  return (await res.json()) as T;
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
// One card of a combo, at the printing the admin configured it from. The field
// names mirror DecklistCard/CubeCard so a piece renders through the same card
// components as the rest of the app.
export interface ComboPiece {
  card_id: string;
  card_name: string;
  slug: string;
  image_normal?: string;
  set_code?: string;
  collector_number?: string;
  color_identity: number;
}
// A named set of cards an admin has registered as a combo for a cube. A deck
// reports it when its main board plays every piece; the match is computed on
// read, so editing a definition changes every deck's answer at once.
export interface Combo {
  id: string;
  cube_id: string;
  name: string;
  description?: string;
  cards: ComboPiece[];
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
  // Owner and cube, denormalized onto the listing so the deck table can filter on
  // them (lib/deckQuery.ts) — `user:` and `cube:` have nothing to match against the
  // bare ids on the decklist. Absent for a deck whose owner or cube has been deleted.
  user?: PublicUser;
  cube_name?: string;
}
export interface DecklistDetail {
  decklist: Decklist;
  color_string: string;
  cards: DecklistCard[];
  // The configured combos this deck's main board assembles. Always present,
  // possibly empty.
  combos: Combo[];
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
  // Share of decks running any of the Power Nine, and how many decks have played a
  // game and lost none. Null / 0 on a snapshot computed before these were added.
  power9_share: number | null;
  undefeated_decks: number;
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
  // Combos the list assembles as typed — the same match the saved deck reports.
  combos: Combo[];
}
