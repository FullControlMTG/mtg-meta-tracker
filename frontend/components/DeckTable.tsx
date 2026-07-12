"use client";

import Link from "next/link";
import { ColorPips } from "@/components/ColorPips";
import { SortHeader } from "@/components/SortHeader";
import type { DecklistListItem } from "@/lib/api";
import { compareIdentity } from "@/lib/colors";
import { pct } from "@/lib/format";
import { useTableSort, type SortColumn } from "@/lib/tableSort";

const byName = (a: DecklistListItem, b: DecklistListItem) =>
  a.decklist.name.localeCompare(b.decklist.name, undefined, { sensitivity: "base" });

// See lib/tableSort.ts for the rules these obey: comparators ascending, blanks last,
// numeric columns descending on the first click.
const COLUMNS: SortColumn<DecklistListItem>[] = [
  { key: "name", label: "Deck", compare: byName },
  {
    key: "colors",
    label: "Colors",
    compare: (a, b) => compareIdentity(a.decklist.color_identity, b.decklist.color_identity),
  },
  {
    key: "record",
    label: "Record",
    num: true,
    descFirst: true,
    // Fewest wins first, and among equal wins the most losses first — so reversed (the
    // default click) this reads most wins, then fewest losses. Draws are not collected,
    // so the chain ends there.
    compare: (a, b) =>
      a.decklist.wins - b.decklist.wins || b.decklist.losses - a.decklist.losses,
  },
  {
    key: "winrate",
    label: "Winrate",
    num: true,
    descFirst: true,
    // A deck that has played no games has no winrate to rank — `unknown` sinks it, so the
    // ?? 0 here is never reached with a blank.
    unknown: (d) => d.decklist.winrate == null,
    compare: (a, b) => (a.decklist.winrate ?? 0) - (b.decklist.winrate ?? 0),
  },
];

export function DeckTable({
  decks,
  showArchetype = false,
}: {
  decks: DecklistListItem[];
  showArchetype?: boolean;
}) {
  // No initial sort: the server returns newest-first, which is the default view.
  const { rows, sort, toggle } = useTableSort(decks, COLUMNS, { tiebreak: byName });

  return (
    <table>
      <thead>
        <tr>
          {COLUMNS.map((col) => (
            <SortHeader key={col.key} col={col} sort={sort} onSort={toggle} />
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map(({ decklist: d }) => (
          <tr key={d.id}>
            <td>
              <Link href={`/decks/${d.id}`}>{d.name}</Link>
              {showArchetype && d.archetype && (
                <span className="muted" style={{ marginLeft: 6, fontSize: "0.85rem" }}>
                  {d.archetype}
                </span>
              )}
            </td>
            <td>
              <ColorPips bits={d.color_identity} showCode />
            </td>
            <td className="num">{d.games_played > 0 ? `${d.wins}-${d.losses}` : "—"}</td>
            <td className="num">{pct(d.winrate)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
