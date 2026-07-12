"use client";

import { useEffect, useMemo, useState } from "react";
import Image from "next/image";
import Link from "next/link";
import { type CardStat } from "@/lib/api";
import { pct } from "@/lib/format";
import { canHover, placeFloating, type Placed } from "@/lib/hover";
import { ColorPips } from "@/components/ColorPips";
import { InfoHint } from "@/components/InfoHint";

// The cube's card stats: sortable on every column, and each row shows the card's art on
// hover so the table can be browsed without leaving it.

type SortKey = "name" | "deck_count" | "inclusion_rate" | "winrate";
type Dir = "asc" | "desc";

// The direction a column is actually read in, for its first click: names A→Z, numbers
// biggest first. Clicking the active column flips it.
const DEFAULT_DIR: Record<SortKey, Dir> = {
  name: "asc",
  deck_count: "desc",
  inclusion_rate: "desc",
  winrate: "desc",
};

const PREVIEW_W = 240;
const PREVIEW_H = Math.round(PREVIEW_W * (88 / 63)); // MTG card aspect ratio

const byName = (a: CardStat, b: CardStat) =>
  a.name.localeCompare(b.name, undefined, { sensitivity: "base" });

function compare(a: CardStat, b: CardStat, key: SortKey, dir: Dir): number {
  const sign = dir === "asc" ? 1 : -1;
  if (key === "name") return sign * byName(a, b);

  const av = a[key];
  const bv = b[key];
  // A card nobody has played yet has an unknown winrate, not a zero one — so it must never
  // outrank a real number. Resolved before the direction sign is applied, which is what
  // sinks the blanks to the bottom in *both* directions rather than floating them to the
  // top on the ascending pass.
  const aNull = av === null || av === undefined;
  const bNull = bv === null || bv === undefined;
  if (aNull || bNull) return aNull && bNull ? 0 : aNull ? 1 : -1;

  if (av !== bv) return sign * (av - bv);
  return byName(a, b); // a total order, so re-sorting never shuffles ties
}

export function CardStatsTable({ cards, cubeId }: { cards: CardStat[]; cubeId: string }) {
  // Matches the order the API already returns, so the first paint doesn't reshuffle.
  const [sort, setSort] = useState<{ key: SortKey; dir: Dir }>({
    key: "inclusion_rate",
    dir: "desc",
  });
  const [preview, setPreview] = useState<(Placed & { card: CardStat }) | null>(null);
  const [hoverable, setHoverable] = useState(false);
  useEffect(() => setHoverable(canHover()), []);

  const rows = useMemo(
    () => [...cards].sort((a, b) => compare(a, b, sort.key, sort.dir)), // a copy: the prop isn't ours to sort in place
    [cards, sort],
  );

  const toggle = (key: SortKey) =>
    setSort((s) =>
      s.key === key
        ? { key, dir: s.dir === "asc" ? "desc" : "asc" }
        : { key, dir: DEFAULT_DIR[key] },
    );

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

  const SortTh = ({ col, label, hint }: { col: SortKey; label: string; hint?: string }) => {
    const active = sort.key === col;
    const dir = active ? sort.dir : DEFAULT_DIR[col];
    return (
      <th
        scope="col"
        className={[col === "name" ? "" : "num", "sortable", active ? "active" : ""]
          .filter(Boolean)
          .join(" ")}
        aria-sort={active ? (sort.dir === "asc" ? "ascending" : "descending") : "none"}
      >
        <button type="button" className="th-sort" onClick={() => toggle(col)}>
          {label}
          <span className="sort-caret" aria-hidden="true">
            {dir === "asc" ? "▲" : "▼"}
          </span>
        </button>
        {/* Outside the button: InfoHint is itself focusable, and a focusable inside a
            button is invalid. */}
        {hint && <InfoHint text={hint} />}
      </th>
    );
  };

  return (
    <>
      <div style={{ overflowX: "auto" }}>
        <table>
          <thead>
            <tr>
              <SortTh col="name" label="Card" />
              <SortTh col="deck_count" label="Decks" />
              <SortTh
                col="inclusion_rate"
                label="Incl."
                hint="Share of decks in this cube that play the card."
              />
              <SortTh
                col="winrate"
                label="WR"
                hint="Winrate of the decks playing this card."
              />
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
                <td colSpan={4} className="muted">
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
