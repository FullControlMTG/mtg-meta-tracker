"use client";

import { useEffect, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { ColorPips } from "@/components/ColorPips";
import { SortHeader } from "@/components/SortHeader";
import { type CardStat } from "@/lib/api";
import { pct } from "@/lib/format";
import { canHover, placeFloating, type Placed } from "@/lib/hover";
import { useTableSort, type SortColumn } from "@/lib/tableSort";

// The cube's card stats: sortable on every column, and each row shows the card's art on
// hover so the table can be browsed without leaving it.

const PREVIEW_W = 240;
const PREVIEW_H = Math.round(PREVIEW_W * (88 / 63)); // MTG card aspect ratio

const byName = (a: CardStat, b: CardStat) =>
  a.name.localeCompare(b.name, undefined, { sensitivity: "base" });

// See lib/tableSort.ts for the rules these obey: comparators ascending, blanks last,
// numeric columns descending on the first click.
const COLUMNS: SortColumn<CardStat>[] = [
  { key: "name", label: "Card", compare: byName },
  {
    key: "deck_count",
    label: "Decks",
    num: true,
    descFirst: true,
    compare: (a, b) => a.deck_count - b.deck_count,
  },
  {
    key: "inclusion_rate",
    label: "Incl.",
    num: true,
    descFirst: true,
    hint: "Share of decks in this cube that play the card.",
    compare: (a, b) => a.inclusion_rate - b.inclusion_rate,
  },
  {
    key: "winrate",
    label: "WR",
    num: true,
    descFirst: true,
    hint: "Winrate of the decks playing this card.",
    // A card nobody has played yet has an unknown winrate, not a zero one, so it must
    // never outrank a real number — `unknown` sinks it either way the column points.
    unknown: (c) => c.winrate == null,
    compare: (a, b) => (a.winrate ?? 0) - (b.winrate ?? 0),
  },
];

export function CardStatsTable({ cards, cubeId }: { cards: CardStat[]; cubeId: string }) {
  // Matches the order the API already returns, so the first paint doesn't reshuffle.
  const { rows, sort, toggle } = useTableSort(cards, COLUMNS, {
    initial: { key: "inclusion_rate", dir: "desc" },
    tiebreak: byName,
  });

  const [preview, setPreview] = useState<(Placed & { card: CardStat }) | null>(null);
  const [hoverable, setHoverable] = useState(false);
  useEffect(() => setHoverable(canHover()), []);

  const show = (card: CardStat, x: number, y: number) =>
    setPreview({ card, ...placeFloating(x, y, PREVIEW_W, PREVIEW_H) });

  const rowHover = (card: CardStat) =>
    hoverable
      ? {
          onMouseEnter: (e: React.MouseEvent) => show(card, e.clientX, e.clientY),
          onMouseMove: (e: React.MouseEvent) => show(card, e.clientX, e.clientY),
          // Guarded: a fast pointer already inside the next row shouldn't have that row's
          // preview cleared by this row's leave.
          onMouseLeave: () =>
            setPreview((p) => (p?.card.card_id === card.card_id ? null : p)),
        }
      : {};

  return (
    <>
      <div style={{ overflowX: "auto" }}>
        <table>
          <thead>
            <tr>
              {COLUMNS.map((col) => (
                <SortHeader key={col.key} col={col} sort={sort} onSort={toggle} />
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((c) => (
              <tr key={c.card_id} className="stat-row" {...rowHover(c)}>
                <td>
                  <Link
                    href={`/cards/${c.slug}?cube=${cubeId}`}
                    style={{
                      display: "inline-flex",
                      alignItems: "center",
                      gap: 6,
                      color: "var(--text)",
                    }}
                    onFocus={(e) => {
                      const r = e.currentTarget.getBoundingClientRect();
                      show(c, r.right, r.top + r.height / 2);
                    }}
                    onBlur={() => setPreview(null)}
                  >
                    <ColorPips bits={c.color_identity} />
                    {c.name}
                  </Link>
                </td>
                <td className="num">{c.deck_count}</td>
                <td className="num">{pct(c.inclusion_rate, 0)}</td>
                <td className="num">{pct(c.winrate)}</td>
              </tr>
            ))}
            {rows.length === 0 && (
              <tr>
                <td colSpan={COLUMNS.length} className="muted">
                  No cards in analyzed decks.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      {/* A sibling of the scroll container, and fixed — so that container can't clip it.
          Decorative: the row's link already names the card. */}
      {preview && (
        <div
          className="card-preview"
          style={{ left: preview.left, top: preview.top, width: PREVIEW_W }}
          aria-hidden="true"
        >
          <Image
            src={`/api/cards/${preview.card.card_id}/image`}
            alt=""
            width={PREVIEW_W}
            height={PREVIEW_H}
            unoptimized
          />
        </div>
      )}
    </>
  );
}
