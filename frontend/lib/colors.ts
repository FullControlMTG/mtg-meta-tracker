// WUBRG color-identity bitset helpers, mirroring backend/internal/domain/color.go
// (W=1 U=2 B=4 R=8 G=16, colorless=0). Hexes are readable stand-ins for the MTG
// pie tuned for contrast on both light and dark surfaces.

export interface ManaColor {
  bit: number;
  code: string;
  name: string;
  hex: string;
}

export const COLORS: ManaColor[] = [
  { bit: 1, code: "W", name: "White", hex: "#c9a227" },
  { bit: 2, code: "U", name: "Blue", hex: "#2a78d6" },
  { bit: 4, code: "B", name: "Black", hex: "#6b6b6b" },
  { bit: 8, code: "R", name: "Red", hex: "#e34948" },
  { bit: 16, code: "G", name: "Green", hex: "#1baf7a" },
];

const COLORLESS: ManaColor = { bit: 0, code: "C", name: "Colorless", hex: "#898781" };

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

// --- cube card grouping ---

export interface CardGroup<T> {
  key: string;
  label: string;
  cards: T[];
}

const popcount = (n: number) => n.toString(2).replace(/0/g, "").length;

// Bucket cube cards into color sections for display: Lands first (anything whose
// type line is a land, regardless of color identity), then mono W/U/B/R/G, then
// Multicolor (>1 color), then Colorless (0 colors, non-land). Empty sections drop.
export function groupCubeCards<T extends { color_identity: number; type_line?: string }>(
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
    if (card.type_line && /\bland\b/i.test(card.type_line)) {
      key = "L";
    } else if (card.color_identity === 0) {
      key = "C";
    } else if (popcount(card.color_identity) > 1) {
      key = "M";
    } else {
      key = colorByBit(card.color_identity).code;
    }
    (buckets[key] ??= []).push(card);
  }
  return order
    .filter((k) => buckets[k]?.length)
    .map((k) => ({ key: k, label: labels[k], cards: buckets[k] }));
}
