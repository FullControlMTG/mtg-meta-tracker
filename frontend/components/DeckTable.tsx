"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { ColorPips } from "@/components/ColorPips";
import { compareIdentity } from "@/lib/colors";
import { pct } from "@/lib/format";
import type { Decklist, DecklistListItem } from "@/lib/api";

type SortKey = "name" | "colors" | "record" | "winrate";

interface Sort {
  key: SortKey;
  dir: "asc" | "desc";
}

const byName = (a: Decklist, b: Decklist) =>
  a.name.localeCompare(b.name, undefined, { sensitivity: "base" });

// Every comparator is written ascending — "worst"/"first" to "best"/"last". A
// descending sort negates it, so encoding a descent here as well would cancel
// out and quietly sort the column backwards.
const COMPARATORS: Record<SortKey, (a: Decklist, b: Decklist) => number> = {
  name: byName,

  colors: (a, b) => compareIdentity(a.color_identity, b.color_identity),

  // Fewest wins first, and among equal wins the most losses first — so reversed
  // (the default click) this reads most wins, then fewest losses. Draws are not
  // collected, so the chain ends there.
  record: (a, b) => a.wins - b.wins || b.losses - a.losses,

  // Safe on the ?? 0: a deck with no winrate never reaches here — see below.
  winrate: (a, b) => (a.winrate ?? 0) - (b.winrate ?? 0),
};

// Numeric columns read best-first, so their first click sorts descending.
const DESC_FIRST: Record<SortKey, boolean> = {
  name: false,
  colors: false,
  record: true,
  winrate: true,
};

export function DeckTable({
  decks,
  showArchetype = false,
}: {
  decks: DecklistListItem[];
  showArchetype?: boolean;
}) {
  // Null until the first click: the server returns newest-first, which is the
  // default view.
  const [sort, setSort] = useState<Sort | null>(null);

  const rows = useMemo(() => {
    if (!sort) return decks;
    const cmp = COMPARATORS[sort.key];
    const sign = sort.dir === "asc" ? 1 : -1;
    return [...decks].sort(({ decklist: a }, { decklist: b }) => {
      // winrate is null until a deck has played a game. A deck with no rate has
      // nothing to rank, so it sinks to the bottom whichever way the column is
      // pointed — hence the check sits outside `sign`, above the comparator.
      if (sort.key === "winrate" && (a.winrate === undefined || b.winrate === undefined)) {
        if (a.winrate !== undefined) return -1;
        if (b.winrate !== undefined) return 1;
        return byName(a, b);
      }
      // Name breaks every tie, so equal rows hold one predictable order.
      return sign * cmp(a, b) || byName(a, b);
    });
  }, [decks, sort]);

  function toggle(key: SortKey) {
    setSort((cur) =>
      cur?.key === key
        ? { key, dir: cur.dir === "asc" ? "desc" : "asc" }
        : { key, dir: DESC_FIRST[key] ? "desc" : "asc" },
    );
  }

  function Header({ sortKey, label, num }: { sortKey: SortKey; label: string; num?: boolean }) {
    const active = sort?.key === sortKey;
    const cls = ["sortable", num ? "num" : "", active ? "active" : ""].filter(Boolean).join(" ");
    return (
      <th className={cls} aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}>
        <button
          type="button"
          onClick={() => toggle(sortKey)}
          style={{
            background: "none",
            border: "none",
            padding: 0,
            font: "inherit",
            letterSpacing: "inherit",
            textTransform: "inherit",
            color: "inherit",
            cursor: "pointer",
          }}
        >
          {label}
          {active && <span aria-hidden> {sort.dir === "asc" ? "▲" : "▼"}</span>}
        </button>
      </th>
    );
  }

  return (
    <table>
      <thead>
        <tr>
          <Header sortKey="name" label="Deck" />
          <Header sortKey="colors" label="Colors" />
          <Header sortKey="record" label="Record" num />
          <Header sortKey="winrate" label="Winrate" num />
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
