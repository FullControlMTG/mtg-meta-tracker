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

// --- cube card grouping ---

export interface CardGroup<T> {
  key: string;
  label: string;
  cards: T[];
}

const popcount = (n: number) => n.toString(2).replace(/0/g, "").length;

const LAND_RE = /\bland\b/i;

// --- display order ---

// The one shared sort for both the cube pool and a deck's boards: color, then
// converted mana cost, then name.
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

// Rank of a card's color section, matching groupCubeCards' section order:
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

// Copies — the caller's array (a server-fetched payload) is not ours to mutate.
export function sortCards<T extends SortableCard>(cards: T[]): T[] {
  return [...cards].sort(compareCards);
}

// Bucket cube cards into the display sections sectionRank ranks: mono W/U/B/R/G,
// then Multicolor (>1 color), then Colorless (0 colors, non-land), then Lands last
// (anything whose type line is a land, whatever its colors). Empty sections drop.
export function groupCubeCards<T extends { group_colors: number; type_line?: string }>(
  cards: T[],
): CardGroup<T>[] {
  const order = ["W", "U", "B", "R", "G", "M", "C", "L"];
  const labels: Record<string, string> = {
    W: "White",
    U: "Blue",
    B: "Black",
    R: "Red",
    G: "Green",
    M: "Multicolor",
    C: "Colorless",
    L: "Lands",
  };
  const buckets: Record<string, T[]> = {};
  for (const card of cards) {
    let key: string;
    if (card.type_line && LAND_RE.test(card.type_line)) {
      key = "L";
    } else if (card.group_colors === 0) {
      key = "C";
    } else if (popcount(card.group_colors) > 1) {
      key = "M";
    } else {
      key = colorByBit(card.group_colors).code;
    }
    (buckets[key] ??= []).push(card);
  }
  return order
    .filter((k) => buckets[k]?.length)
    .map((k) => ({ key: k, label: labels[k], cards: buckets[k] }));
}
