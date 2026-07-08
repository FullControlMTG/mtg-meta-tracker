// Fetch helpers. Server components hit the backend origin directly; client
// components use the same-origin /api rewrite so the session cookie is sent.

function base(): string {
  if (typeof window === "undefined") {
    return process.env.BACKEND_ORIGIN ?? "http://localhost:8080";
  }
  return "";
}

export async function apiGet<T>(path: string, revalidate = 60): Promise<T> {
  const res = await fetch(base() + "/api" + path, { next: { revalidate } });
  if (!res.ok) throw new Error(`GET ${path}: ${res.status}`);
  return res.json() as Promise<T>;
}

// apiGetOptional returns null on any non-2xx (e.g. 404 when no analytics yet) or
// when the backend is unreachable (e.g. during a build with no server running).
export async function apiGetOptional<T>(path: string, revalidate = 60): Promise<T | null> {
  try {
    const res = await fetch(base() + "/api" + path, { next: { revalidate } });
    if (!res.ok) return null;
    return (await res.json()) as T;
  } catch {
    return null;
  }
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
  description?: string;
  last_synced_at?: string;
}
export interface CubeView {
  cube: Cube;
  card_count: number;
}

export interface Decklist {
  id: string;
  cube_id: string;
  user_id: string;
  name: string;
  description?: string;
  color_identity: number;
  archetype?: string;
  source_url?: string;
  decklist_raw: string;
  card_count: number;
  status: string;
  games_played: number;
  wins: number;
  losses: number;
  draws: number;
  placement?: number;
  winrate?: number;
  created_at: string;
}
export interface DecklistCard {
  card_id?: string;
  card_name: string;
  quantity: number;
  is_resolved: boolean;
  board: string;
  image_art_crop?: string;
  image_normal?: string;
  cmc?: number;
  type_line?: string;
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
  draws: number;
  winrate: number | null;
  avg_placement: number | null;
}
export interface CardStat {
  card_id: string;
  name: string;
  image_normal?: string;
  image_art_crop?: string;
  color_identity: number;
  deck_count: number;
  inclusion_rate: number;
  games: number;
  wins: number;
  winrate: number | null;
  winrate_shrunk: number | null;
  winrate_lift: number | null;
  wilson_lower: number | null;
}
export interface CardPair {
  card_b_id: string;
  name: string;
  co_count: number;
  support: number;
  confidence_ab: number;
  lift: number;
  pair_winrate: number | null;
}

export interface InferResult {
  color_identity: number;
  color_string: string;
  resolved: string[] | null;
  unresolved: string[] | null;
}
