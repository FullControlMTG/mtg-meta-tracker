// The filter query language behind the deck table. One line of text — `losses:0
// c:ur date>=2026-01` — compiles to a predicate over a DecklistListItem.
//
// The grammar is the familiar `field:value` one (GitHub's issue search, Scryfall's
// card search), because the people using this already type Scryfall queries all day:
//
//   term    := ["-"] (field op value | bareword)
//   op      := ":" | "=" | "!=" | ">" | ">=" | "<" | "<="
//   value   := "quoted string" | unquoted
//
// Terms are ANDed; a leading "-" negates one; a bareword with no field searches the
// deck name. There is deliberately no OR and no parentheses — the extensibility that
// matters here is the FIELDS table below, not boolean algebra. A new filterable field
// is one entry in it, and it is then parseable, documentable and testable for free.
//
// Everything is evaluated client-side over the list the page already fetched, so the
// filter is instant and the server keeps serving one cacheable payload.

import type { DecklistListItem } from "@/lib/api";
import { colorCount } from "@/lib/colors";
import { isoDay } from "@/lib/format";
import { matchesQuery, normalizeName } from "@/lib/search";

export type DeckPredicate = (d: DecklistListItem) => boolean;

const OPS = [":", "=", "!=", ">", ">=", "<", "<="] as const;
export type QueryOp = (typeof OPS)[number];

// --- fields ---

interface FieldDef {
  key: string;
  aliases: string[];
  // What the panel's reference list says about it, and one query that works.
  hint: string;
  example: string;
  // Null means "this operator makes no sense here" — reported, not silently ignored.
  build: (op: QueryOp, value: string) => DeckPredicate | string;
}

// A field whose value is prose. `values` returns every string the term may match, so
// `user:` can look at both the username and the display name without the caller
// having to know there are two.
function text(values: (d: DecklistListItem) => (string | null | undefined)[]) {
  return (op: QueryOp, value: string): DeckPredicate | string => {
    const present = (d: DecklistListItem) => values(d).filter(Boolean) as string[];
    switch (op) {
      case ":":
        // The same word-wise, punctuation-blind match the card search boxes use, so
        // `user:jake` finds "Jake R." and `name:blue moon` finds "Blue Moon".
        return (d) => present(d).some((s) => matchesQuery(s, value));
      case "=":
      case "!=": {
        const want = normalizeName(value);
        const eq = (d: DecklistListItem) => present(d).some((s) => normalizeName(s) === want);
        return op === "=" ? eq : (d) => !eq(d);
      }
      default:
        return `“${op}” compares numbers; this field holds text`;
    }
  };
}

// A field whose value is a number, or null when there is nothing to compare — a deck
// that has played no games has no winrate. Null matches no comparison, in either
// direction, exactly as it sorts nowhere (see lib/tableSort.ts).
function number(
  get: (d: DecklistListItem) => number | null | undefined,
  parse: (v: string) => number = Number,
) {
  return (op: QueryOp, value: string): DeckPredicate | string => {
    const n = parse(value.trim());
    if (!Number.isFinite(n)) return `“${value}” is not a number`;
    const cmp = compareOp(op);
    return (d) => {
      const v = get(d);
      return v != null && cmp(v, n);
    };
  };
}

// Percentages are typed the way the column reads them: `winrate>=50`, not 0.5. A
// trailing % is accepted and means the same thing.
const parsePercent = (v: string) => Number(v.replace(/%$/, "")) / 100;

// Dates are ISO days, which compare correctly as plain strings — so the same
// lexicographic comparison serves `date>=2026-01-01` and, with ":", the prefix match
// that makes `date:2026-07` mean "in July".
function date(get: (d: DecklistListItem) => string) {
  return (op: QueryOp, value: string): DeckPredicate | string => {
    const want = value.trim();
    if (!/^\d{4}(-\d{2}(-\d{2})?)?$/.test(want)) {
      return `“${value}” is not a date (YYYY, YYYY-MM or YYYY-MM-DD)`;
    }
    if (op === ":") return (d) => get(d).startsWith(want);
    if (op === "!=") return (d) => !get(d).startsWith(want);
    const cmp = compareOp(op);
    return (d) => cmp(compareStrings(get(d), want), 0);
  };
}

// A WUBRG bitset. The operators are set relations rather than magnitudes, because
// that is the only reading of "greater" a color combination has: `c:ur` wants the
// decks that play *at least* blue and red, `c=ur` exactly those two.
function colors(get: (d: DecklistListItem) => number) {
  return (op: QueryOp, value: string): DeckPredicate | string => {
    const want = parseColors(value);
    if (want === null) return `“${value}” is not a color combination (w, u, b, r, g or c)`;
    return (d) => {
      const have = get(d);
      switch (op) {
        case "=":
          return have === want;
        case "!=":
          return have !== want;
        case ":":
        case ">=":
          // Colorless is the empty set, which every deck contains — so `c:c` has to
          // mean the decks that play *no* colors, or the term matches everything.
          return want === 0 ? have === 0 : (have & want) === want;
        case ">":
          return (have & want) === want && have !== want;
        case "<=":
          return (have & ~want) === 0;
        case "<":
          return (have & ~want) === 0 && have !== want;
      }
    };
  };
}

// The record column is one cell holding two numbers, so `record:3-0` matches it whole
// and the `wins:` / `losses:` fields address the halves. A deck with no games has no
// record — the table shows an em dash there — and matches nothing.
function record(op: QueryOp, value: string): DeckPredicate | string {
  const m = /^(\d+)\s*-\s*(\d+)$/.exec(value.trim());
  if (!m) return `“${value}” is not a record (wins-losses, e.g. 3-0)`;
  if (op !== ":" && op !== "=" && op !== "!=") {
    return `“${op}” compares numbers; filter wins: or losses: instead`;
  }
  const [wins, losses] = [Number(m[1]), Number(m[2])];
  const eq = (d: DecklistListItem) =>
    d.decklist.games_played > 0 && d.decklist.wins === wins && d.decklist.losses === losses;
  return op === "!=" ? (d) => !eq(d) : eq;
}

export const FIELDS: FieldDef[] = [
  {
    key: "name",
    aliases: ["deck"],
    hint: "Deck name. Bare words with no field: search this.",
    example: "name:reanimator",
    build: text((d) => [d.decklist.name]),
  },
  {
    key: "colors",
    aliases: ["color", "c", "ci"],
    hint: "Colors played. “:” means at least these, “=” exactly, “<=” only within.",
    example: "c:ur",
    build: colors((d) => d.decklist.color_identity),
  },
  {
    key: "colorcount",
    aliases: ["cc"],
    hint: "How many colors the deck plays. Splashes do not count.",
    example: "colorcount<=2",
    build: number((d) => colorCount(d.decklist.color_identity)),
  },
  {
    key: "splash",
    aliases: [],
    hint: "Colors the deck only splashes.",
    example: "splash:r",
    build: colors((d) => d.decklist.splash_colors),
  },
  {
    key: "date",
    aliases: ["played"],
    hint: "The day the deck was played. “:” matches a prefix, so a year or a month works.",
    example: "date>=2026-01",
    build: date((d) => isoDay(d.decklist.played_at)),
  },
  {
    key: "record",
    aliases: ["rec"],
    hint: "The record as written, wins-losses.",
    example: "record:3-0",
    build: record,
  },
  {
    key: "wins",
    aliases: ["w"],
    hint: "Games won.",
    example: "wins>=3",
    build: number((d) => d.decklist.wins),
  },
  {
    key: "losses",
    aliases: ["l"],
    hint: "Games lost.",
    example: "losses:0",
    build: number((d) => d.decklist.losses),
  },
  {
    key: "games",
    aliases: ["gp"],
    hint: "Games played. “games>0” is how you drop the decks that never got played.",
    example: "games>0",
    build: number((d) => d.decklist.games_played),
  },
  {
    key: "winrate",
    aliases: ["wr"],
    hint: "Winrate as a percentage. A deck with no games has none and never matches.",
    example: "winrate>=50",
    build: number((d) => d.decklist.winrate ?? null, parsePercent),
  },
  {
    key: "user",
    aliases: ["owner", "player", "by"],
    hint: "Who built it — username or display name.",
    example: "user:jake",
    build: text((d) => [d.user?.username, d.user?.display_name]),
  },
  {
    key: "cube",
    aliases: [],
    hint: "The cube the deck was built from.",
    example: 'cube:"vintage cube"',
    build: text((d) => [d.cube_name]),
  },
  {
    key: "archetype",
    aliases: ["arch"],
    hint: "Aggro, control, midrange, tempo or combo.",
    example: "archetype:control",
    build: text((d) => [d.decklist.archetype]),
  },
  {
    key: "event",
    aliases: [],
    hint: "The event the record was set at.",
    example: 'event:"friday night"',
    build: text((d) => [d.decklist.event_name]),
  },
  {
    key: "status",
    aliases: [],
    hint: "Draft, active or archived.",
    example: "status:active",
    build: text((d) => [d.decklist.status]),
  },
  {
    key: "cards",
    aliases: ["size"],
    hint: "Cards in the main deck.",
    example: "cards>=40",
    build: number((d) => d.decklist.card_count),
  },
];

const BY_NAME = new Map<string, FieldDef>();
for (const f of FIELDS) {
  BY_NAME.set(f.key, f);
  for (const a of f.aliases) BY_NAME.set(a, f);
}

// --- parsing ---

export interface CompiledQuery {
  // True for every deck when the query is empty or entirely unparseable, so a typo
  // narrows nothing rather than blanking the table.
  predicate: DeckPredicate;
  // Whether anything is actually being filtered — drives the "n of m" readout.
  active: boolean;
  // What could not be understood, phrased for a human. Shown under the input.
  errors: string[];
}

const EMPTY: CompiledQuery = { predicate: () => true, active: false, errors: [] };

export function compileQuery(input: string): CompiledQuery {
  const tokens = tokenize(input);
  if (tokens.length === 0) return EMPTY;

  const predicates: DeckPredicate[] = [];
  const errors: string[] = [];
  for (const token of tokens) {
    const built = buildTerm(token);
    if (typeof built === "string") errors.push(built);
    else predicates.push(built);
  }
  if (predicates.length === 0) return { ...EMPTY, errors };
  return {
    predicate: (d) => predicates.every((p) => p(d)),
    active: true,
    errors,
  };
}

export function filterDecks(decks: DecklistListItem[], query: CompiledQuery): DecklistListItem[] {
  return query.active ? decks.filter(query.predicate) : decks;
}

// A term, already unquoted by the tokenizer. Its operator is the first one that
// appears, so a value may itself contain a colon (`name:re:zero`) without escaping.
const TERM_RE = /^([a-z_]+)(!=|>=|<=|[:=<>])([\s\S]*)$/i;

function buildTerm(token: string): DeckPredicate | string {
  const negated = token.startsWith("-") && token.length > 1;
  const body = negated ? token.slice(1) : token;

  const m = TERM_RE.exec(body);
  // No field: a bare word searches the name, which is what someone who has not read
  // any of this will type first.
  if (!m) {
    const p: DeckPredicate = (d) => matchesQuery(d.decklist.name, body);
    return negated ? (d) => !p(d) : p;
  }

  const [, rawField, op, value] = m;
  const field = BY_NAME.get(rawField.toLowerCase());
  if (!field) return `Unknown field “${rawField}”`;
  if (value.trim() === "") return `“${rawField}${op}” is missing a value`;

  const built = field.build(op as QueryOp, value);
  if (typeof built === "string") return `${field.key}: ${built}`;
  return negated ? (d) => !built(d) : built;
}

// Whitespace separates terms, quotes group them: `event:"friday night"` is one token.
// The quotes are stripped here, so everything downstream sees plain values.
function tokenize(input: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quote: string | null = null;
  let quoted = false;
  for (const ch of input) {
    if (quote) {
      if (ch === quote) quote = null;
      else cur += ch;
      continue;
    }
    if (ch === '"' || ch === "'") {
      quote = ch;
      // An empty pair of quotes is still a value — `event:""` is a real term, and
      // without this flag it would tokenize away to nothing.
      quoted = true;
      continue;
    }
    if (/\s/.test(ch)) {
      if (cur || quoted) out.push(cur);
      cur = "";
      quoted = false;
      continue;
    }
    cur += ch;
  }
  if (cur || quoted) out.push(cur);
  return out;
}

// --- links into the deck list ---

// A stat that counted a subset of the decks can link to that subset, by handing the
// deck list the same query it was counted with. Built here so the query is written in
// one place and the quoting is not everyone's problem.
export function deckListHref(
  terms: string[],
  sort?: { key: string; dir: "asc" | "desc" },
): string {
  const params = new URLSearchParams();
  const q = terms.filter(Boolean).join(" ");
  if (q) params.set("q", q);
  if (sort) {
    params.set("sort", sort.key);
    params.set("dir", sort.dir);
  }
  return `/decks?${params}`;
}

// Wraps a value that may hold spaces — a cube's name, a display name. The language has
// no escape character, so an embedded quote is dropped rather than escaped: text terms
// match punctuation-blind (see normalizeName), so dropping it costs nothing.
export function quoteTerm(value: string): string {
  const clean = value.replace(/["']/g, "");
  return /\s/.test(clean) ? `"${clean}"` : clean;
}

// "Undefeated" as the analytics engine counts it: played at least one game and lost
// none (backend/internal/analytics/compute.go). The tile and the list it links to have
// to agree on that, so both read it from here.
export const UNDEFEATED_TERMS = ["games>0", "losses:0"];

// --- comparison helpers ---

function compareStrings(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

// ":" reads as "is", which for a number is equality — `wins:3` and `wins=3` are the
// same term. It is the inequalities that need the explicit operator.
function compareOp(op: QueryOp): (a: number, b: number) => boolean {
  switch (op) {
    case ":":
    case "=":
      return (a, b) => a === b;
    case "!=":
      return (a, b) => a !== b;
    case ">":
      return (a, b) => a > b;
    case ">=":
      return (a, b) => a >= b;
    case "<":
      return (a, b) => a < b;
    case "<=":
      return (a, b) => a <= b;
  }
}

// A color combination as its bitset: "ur" → U|R, "c" → 0 (colorless). Null when the
// string holds a letter that is not a color, so the term can be reported rather than
// quietly matching nothing.
function parseColors(value: string): number | null {
  const LETTERS: Record<string, number> = { w: 1, u: 2, b: 4, r: 8, g: 16, c: 0 };
  const s = value.trim().toLowerCase();
  if (s === "") return null;
  let bits = 0;
  for (const ch of s) {
    if (!(ch in LETTERS)) return null;
    bits |= LETTERS[ch];
  }
  return bits;
}
