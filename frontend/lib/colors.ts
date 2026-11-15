import type { SortDir } from "@/lib/tableSort";

// WUBRG color-identity bitset helpers, mirroring backend/internal/domain/color.go
// (W=1 U=2 B=4 R=8 G=16, colorless=0). The hexes are MTG's own pie, so white is a
// near-white and black a near-black: each disappears into one of the two surfaces, and
// every fill made from them wears a --pip-ring outline to stay a shape. Contrast lives
// in that ring, not in the hex — don't "fix" a color by lightening it.

export interface ManaColor {
  bit: number;
  code: string;
  name: string;
  hex: string;
}

export const COLORS: ManaColor[] = [
  { bit: 1, code: "W", name: "White", hex: "#fffbe8" },
  { bit: 2, code: "U", name: "Blue", hex: "#0e68ab" },
  { bit: 4, code: "B", name: "Black", hex: "#150b00" },
  { bit: 8, code: "R", name: "Red", hex: "#d3202a" },
  { bit: 16, code: "G", name: "Green", hex: "#00733e" },
];

const COLORLESS: ManaColor = { bit: 0, code: "C", name: "Colorless", hex: "#c5c0b8" };

export function identityString(bits: number): string {
  if (bits === 0) return "C";
  return COLORS.filter((c) => bits & c.bit)
    .map((c) => c.code)
    .join("");
}

export function identityColors(bits: number): ManaColor[] {
  if (bits === 0) return [COLORLESS];
  return COLORS.filter((c) => bits & c.bit);
}

export function colorByBit(bit: number): ManaColor {
  return COLORS.find((c) => c.bit === bit) ?? COLORLESS;
}

// How many of the five colors a bitset holds. Colorless is 0 — unlike
// identityColors, which hands back a single "colorless" swatch to render.
export function colorCount(bits: number): number {
  return COLORS.filter((c) => bits & c.bit).length;
}

// The ten guilds and the ten shards and wedges, by bitset. These are what a
// playgroup says out loud — nobody calls a deck "UBG", they call it Sultai — so a
// combination wears its name wherever there is room for one.
//
// Four- and five-color decks stop here on purpose. They have printed names too
// (Yore-Tiller, Glint-Eye) and no one has ever used them; players say "WUBR", or
// they say "four-color, no green". The letters are the honest label.
// Kept five to a line, one line per family, because that grouping is the point:
// each line is one turn of the color pie. See ColorWheelGrid, which lays them out.
const COMBO_NAMES: Record<number, string> = {
  // allied guilds
  3: "Azorius", 6: "Dimir", 12: "Rakdos", 24: "Gruul", 17: "Selesnya",
  // enemy guilds
  5: "Orzhov", 10: "Izzet", 20: "Golgari", 9: "Boros", 18: "Simic",
  // shards — a color and both its allies
  19: "Bant", 7: "Esper", 14: "Grixis", 28: "Jund", 25: "Naya",
  // wedges — a color and both its enemies
  13: "Mardu", 26: "Temur", 21: "Abzan", 11: "Jeskai", 22: "Sultai",
};

export function comboName(bits: number): string {
  if (bits === 0) return "Colorless";
  const named = COMBO_NAMES[bits];
  if (named) return named;
  if (colorCount(bits) === 1) return `Mono-${colorByBit(bits).name.toLowerCase()}`;
  return identityString(bits);
}

// --- card grouping ---

export interface CardGroup<T> {
  key: string;
  label: string;
  cards: T[];
}

const popcount = (n: number) => n.toString(2).replace(/0/g, "").length;

const LAND_RE = /\bland\b/i;

// --- display order ---

// The default order for both the cube pool and a deck's boards — color, then converted
// mana cost, then name — and the tiebreak under every other order a reader picks.
export interface SortableCard {
  card_name: string;
  cmc?: number;
  type_line?: string;
  // The colors a card is *displayed* under — its casting cost's colors, or, for a
  // land, every color it is related to. Derived server-side; see domain.GroupColors.
  // Optional because an unresolved deck card has no `cards` row to join to.
  group_colors?: number;
  color_identity?: number;
}

// Rank of a card's color section, indexing COLOR_SECTIONS:
// mono W,U,B,R,G → Multicolor → Colorless → Lands.
const MULTICOLOR_RANK = COLORS.length; // 5
const COLORLESS_RANK = MULTICOLOR_RANK + 1;
const LAND_RANK = COLORLESS_RANK + 1;

function sectionRank(card: SortableCard): number {
  if (card.type_line && LAND_RE.test(card.type_line)) return LAND_RANK;
  const bits = card.group_colors ?? 0;
  if (bits === 0) return COLORLESS_RANK;
  if (popcount(bits) > 1) return MULTICOLOR_RANK;
  return COLORS.findIndex((c) => c.bit === bits);
}

// Order two color identities: fewer colors first (pairs, then shards/wedges, then
// four-color, then five), and within a count, in WUBRG order — so the guilds come
// out WU, WB, WR, WG, UB, ... rather than interleaved. Identical within a mono or
// colorless section, where every card shares one identity.
export function compareIdentity(a: number, b: number): number {
  if (a === b) return 0;
  const byCount = popcount(a) - popcount(b);
  if (byCount !== 0) return byCount;
  for (const { bit } of COLORS) {
    const hasA = (a & bit) !== 0;
    const hasB = (b & bit) !== 0;
    if (hasA !== hasB) return hasA ? -1 : 1;
  }
  return 0;
}

export function compareCards(a: SortableCard, b: SortableCard): number {
  const bySection = sectionRank(a) - sectionRank(b);
  if (bySection !== 0) return bySection;
  const byGroup = compareIdentity(a.group_colors ?? 0, b.group_colors ?? 0);
  if (byGroup !== 0) return byGroup;
  // A card with no cached cmc (only ever an unresolved deck entry) curves as a 0.
  const byCMC = (a.cmc ?? 0) - (b.cmc ?? 0);
  if (byCMC !== 0) return byCMC;
  // Only now the full identity, which still knows about the colored abilities the
  // group colors deliberately ignore — so the Azorius rocks cluster together among
  // the two-drop artifacts. It has to break the tie *under* cmc rather than over it,
  // or a Noble Hierarch (green, but WUG identity) would sort to the back of the
  // green section and break the curve the section is read for.
  const byIdentity = compareIdentity(a.color_identity ?? 0, b.color_identity ?? 0);
  if (byIdentity !== 0) return byIdentity;
  return a.card_name.localeCompare(b.card_name, undefined, { sensitivity: "base" });
}

// --- user-chosen order ---

export type CardSortKey = "color" | "cmc" | "name" | "type";

// The order a card browser's sort picker lists them in; the first is the default.
export const CARD_SORTS: { key: CardSortKey; label: string }[] = [
  { key: "color", label: "Color" },
  { key: "cmc", label: "Mana value" },
  { key: "name", label: "Name" },
  { key: "type", label: "Type" },
];

// The order a type sort reads in: spells by how a deck is usually counted, lands last.
// A card is ranked by the first of these its front face has, so an artifact creature is
// a creature — except a land, which is a land whatever else it is, as in sectionRank.
const TYPE_ORDER = [
  /\bcreature\b/i,
  /\bplaneswalker\b/i,
  /\binstant\b/i,
  /\bsorcery\b/i,
  /\bartifact\b/i,
  /\benchantment\b/i,
  /\bbattle\b/i,
];

function typeRank(card: SortableCard): number {
  const front = (card.type_line ?? "").split("//")[0];
  if (LAND_RE.test(front)) return TYPE_ORDER.length;
  const i = TYPE_ORDER.findIndex((re) => re.test(front));
  // Unresolved cards have no type line at all; they trail even the lands.
  return i === -1 ? TYPE_ORDER.length + 1 : i;
}

// Only the key a sort is *for*. Direction flips this alone; every tie then falls through
// to compareCards ascending, so a descending mana-value sort still reads each cmc in color
// order rather than backwards through the pie.
const PRIMARY: Record<CardSortKey, (a: SortableCard, b: SortableCard) => number> = {
  color: (a, b) =>
    sectionRank(a) - sectionRank(b) ||
    compareIdentity(a.group_colors ?? 0, b.group_colors ?? 0),
  cmc: (a, b) => (a.cmc ?? 0) - (b.cmc ?? 0),
  name: (a, b) => a.card_name.localeCompare(b.card_name, undefined, { sensitivity: "base" }),
  type: (a, b) => typeRank(a) - typeRank(b),
};

export function sortCardsBy<T extends SortableCard>(
  cards: T[],
  key: CardSortKey,
  dir: SortDir,
): T[] {
  const sign = dir === "asc" ? 1 : -1;
  const primary = PRIMARY[key];
  // Copies — the caller's array (a server-fetched payload) is not ours to mutate.
  return [...cards].sort((a, b) => sign * primary(a, b) || compareCards(a, b));
}

// --- user-chosen grouping ---

// The sections a card browser splits its cards into, picked independently of the sort:
// the sort orders the cards *within* each section. "board" only means something for a
// deck, so a browser offers just the groupings that fit its cards.
export type CardGroupKey = "color" | "cmc" | "type" | "board" | "none";

export const CARD_GROUPS: { key: CardGroupKey; label: string }[] = [
  { key: "color", label: "Color" },
  { key: "cmc", label: "Mana value" },
  { key: "type", label: "Type" },
  { key: "board", label: "Board" },
  { key: "none", label: "None" },
];

// The sort whose first key a grouping already is. When the two match, the sections
// themselves turn around with a descending sort; any other sort leaves them in place.
export const GROUP_SORT: Partial<Record<CardGroupKey, CardSortKey>> = {
  color: "color",
  cmc: "cmc",
  type: "type",
};

export interface GroupableCard extends SortableCard {
  board?: string;
}

const COLOR_SECTIONS = ["White", "Blue", "Black", "Red", "Green", "Multicolor", "Colorless", "Lands"];

// Plural, in TYPE_ORDER's order, then the two ranks typeRank puts past its end.
const TYPE_SECTIONS = [
  "Creatures",
  "Planeswalkers",
  "Instants",
  "Sorceries",
  "Artifacts",
  "Enchantments",
  "Battles",
  "Lands",
  "Unknown type",
];

const BOARD_SECTIONS: { key: string; label: string }[] = [
  { key: "main", label: "Mainboard" },
  { key: "side", label: "Sideboard" },
  { key: "maybe", label: "Maybeboard" },
];

// Mana values past this share one section: a cube's handful of 8- and 9-drops would
// otherwise each be a section of one.
const CMC_CAP = 7;
const CMC_LAND = CMC_CAP + 1;

// A card's section, as a rank (the section order) and the label it shows under.
function sectionOf(card: GroupableCard, key: CardGroupKey): { rank: number; label: string } {
  switch (key) {
    case "color": {
      const rank = sectionRank(card);
      return { rank, label: COLOR_SECTIONS[rank] };
    }
    case "cmc": {
      // Lands are not part of the curve; a "0" section full of them hides the real zeros.
      if (card.type_line && LAND_RE.test(card.type_line)) return { rank: CMC_LAND, label: "Lands" };
      const cmc = Math.min(Math.floor(card.cmc ?? 0), CMC_CAP);
      return { rank: cmc, label: cmc === CMC_CAP ? `Mana value ${CMC_CAP}+` : `Mana value ${cmc}` };
    }
    case "type": {
      const rank = typeRank(card);
      return { rank, label: TYPE_SECTIONS[rank] };
    }
    case "board": {
      const i = BOARD_SECTIONS.findIndex((b) => b.key === card.board);
      // A board this list doesn't know still shows, after the known ones, under its own name.
      return i === -1
        ? { rank: BOARD_SECTIONS.length, label: card.board ?? "Other" }
        : { rank: i, label: BOARD_SECTIONS[i].label };
    }
    case "none":
      return { rank: 0, label: "All cards" };
  }
}

// Split cards into the sections a grouping names, in that grouping's order. Cards keep
// their input order within a section — sorting them is the caller's, and separate — and
// a section with no cards does not appear.
export function groupCardsBy<T extends GroupableCard>(cards: T[], key: CardGroupKey): CardGroup<T>[] {
  const byLabel = new Map<string, { rank: number; group: CardGroup<T> }>();
  for (const card of cards) {
    const { rank, label } = sectionOf(card, key);
    let entry = byLabel.get(label);
    if (!entry) {
      entry = { rank, group: { key: `${key}:${label}`, label, cards: [] } };
      byLabel.set(label, entry);
    }
    entry.group.cards.push(card);
  }
  return [...byLabel.values()]
    .sort((a, b) => a.rank - b.rank || a.group.label.localeCompare(b.group.label))
    .map((e) => e.group);
}
