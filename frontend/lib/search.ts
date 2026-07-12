// Card-name matching for the search boxes. Nobody typing into a filter is going to
// reach for a comma, an accent or a "//", so neither punctuation nor case may be load
// bearing: both sides are flattened to lowercase words before they meet.
//
// The flattening mirrors the slug the database already generates
// (schema.sql: regexp_replace(lower(name), '[^a-z0-9]+', '-', 'g')), on a space.

// NFKD splits an accent off its letter (Jötun → Jo + ◌̈) and the combining mark is then
// dropped by the [^a-z0-9] pass. What it does *not* split are the true ligatures, so
// those are spelled out first — otherwise "aether" would never find Æther Vial.
const LIGATURES: Record<string, string> = {
  æ: "ae",
  œ: "oe",
  ø: "o",
  ł: "l",
  ß: "ss",
};

export function normalizeName(s: string): string {
  return (
    s
      .toLowerCase()
      .replace(/[æœøłß]/g, (c) => LIGATURES[c])
      .normalize("NFKD")
      // Apostrophes are deleted rather than spaced, because they sit *inside* a word:
      // "Blacksmith's Skill" has to be findable by typing "blacksmiths skill", which it
      // would not be if the possessive broke into a "blacksmith" and a stray "s".
      .replace(/['’`]/g, "")
      .replace(/[^a-z0-9]+/g, " ")
      .trim()
  );
}

// Every word of the query has to appear somewhere in the name, in any order — so "jace
// mind" finds Jace, the Mind Sculptor, and a bare "bolt" finds Lightning Bolt. An empty
// query matches everything, which is what makes clearing the box restore the full list.
export function matchesQuery(name: string, query: string): boolean {
  const tokens = normalizeName(query).split(" ").filter(Boolean);
  if (tokens.length === 0) return true;
  const haystack = normalizeName(name);
  return tokens.every((t) => haystack.includes(t));
}
