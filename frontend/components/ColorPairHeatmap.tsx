"use client";

import { Fragment, useState } from "react";
import { COLORS } from "@/lib/colors";
import { pct } from "@/lib/format";
import { placeFloating, type Placed } from "@/lib/hover";

// A 5×5 matrix of how often a player pairs the colors: the diagonal is decks playing
// that color at all, and cell (row, col) is decks playing both. Symmetric — "W with U"
// and "U with W" are the same decks — so it reads the same across a row or down a
// column, and the reader can pick whichever direction they started scanning.
//
// This is a magnitude encoding, so it takes one hue (--accent) light-to-dark rather
// than the mana colors: the cell is about a *count*, and painting it green because
// green is in the pair would put two meanings on one channel. The colors being crossed
// are named on the axes, where their own pips carry them.
//
// The ramp tops out well short of solid so the count printed in each cell keeps its
// contrast on both surfaces — the number is the value, the fill is the glance.
//
// Splashes are left out by the caller, like everywhere else in the app: a deck's colors
// are the ones it is built on.

const MAX_ALPHA = 0.55;
const TIP_W = 220;
const TIP_H = 96;

export function ColorPairHeatmap({
  bitsets,
  unit = "deck",
}: {
  bitsets: number[]; // one color identity per deck
  unit?: string;
}) {
  const [tip, setTip] = useState<(Placed & { row: number; col: number }) | null>(null);

  const total = bitsets.length;
  // counts[i][j]: decks playing both colors i and j; the diagonal is decks playing i.
  const counts = COLORS.map((a) =>
    COLORS.map((b) => bitsets.filter((bits) => (bits & a.bit) !== 0 && (bits & b.bit) !== 0).length),
  );
  const max = Math.max(...counts.flat(), 0);

  if (total === 0) return <p className="muted">No decks yet.</p>;

  const show = (row: number, col: number, x: number, y: number) =>
    setTip({ row, col, ...placeFloating(x, y, TIP_W, TIP_H) });

  const describe = (row: number, col: number) => {
    const n = counts[row][col];
    const plural = `${n} ${unit}${n === 1 ? "" : "s"}`;
    return row === col
      ? `${plural} play ${COLORS[row].name}`
      : `${plural} play ${COLORS[row].name} and ${COLORS[col].name} together`;
  };

  const pip = (i: number) => (
    <span
      style={{
        width: 11,
        height: 11,
        borderRadius: "50%",
        background: COLORS[i].hex,
        border: "1px solid var(--pip-ring)",
        display: "inline-block",
      }}
    />
  );

  return (
    <div>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: `1.6rem repeat(${COLORS.length}, minmax(0, 1fr))`,
          gap: 2,
          fontSize: "0.8rem",
        }}
      >
        {/* Column headers, offset by the empty corner above the row headers. */}
        <span />
        {COLORS.map((c, i) => (
          <span
            key={c.bit}
            title={c.name}
            style={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 2,
              paddingBottom: 2,
            }}
          >
            {pip(i)}
            <span className="muted" style={{ fontSize: "0.7rem" }}>
              {c.code}
            </span>
          </span>
        ))}

        {/* The grid is one flat list of cells — a wrapper element per row would break
            the column tracks — so a row is a fragment of its header plus five cells. */}
        {COLORS.map((rowColor, row) => (
          <Fragment key={rowColor.bit}>
            <span
              title={rowColor.name}
              style={{ display: "flex", alignItems: "center", justifyContent: "center", gap: 3 }}
            >
              {pip(row)}
            </span>
            {COLORS.map((colColor, col) => {
              const n = counts[row][col];
              const active = tip?.row === row && tip?.col === col;
              return (
                <div
                  key={colColor.bit}
                  tabIndex={0}
                  role="img"
                  aria-label={describe(row, col)}
                  onMouseEnter={(e) => show(row, col, e.clientX, e.clientY)}
                  onMouseMove={(e) => show(row, col, e.clientX, e.clientY)}
                  onMouseLeave={() => setTip(null)}
                  onFocus={(e) => {
                    const r = e.currentTarget.getBoundingClientRect();
                    show(row, col, r.right, r.top + r.height / 2);
                  }}
                  onBlur={() => setTip(null)}
                  style={{
                    position: "relative",
                    aspectRatio: "1",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    borderRadius: 4,
                    background: "var(--grid)",
                    // The diagonal is a different question from the rest of the grid
                    // ("plays it" vs "pairs it"), so it wears a border rather than
                    // pretending to be one more cell of the same series.
                    outline: row === col ? "1px solid var(--border)" : "none",
                    outlineOffset: -1,
                    boxShadow: active ? "0 0 0 2px var(--accent)" : "none",
                    cursor: "pointer",
                  }}
                >
                  <span
                    style={{
                      position: "absolute",
                      inset: 0,
                      borderRadius: 4,
                      background: "var(--accent)",
                      opacity: max > 0 ? (n / max) * MAX_ALPHA : 0,
                    }}
                  />
                  <span
                    style={{
                      position: "relative",
                      fontVariantNumeric: "tabular-nums",
                      color: n === 0 ? "var(--muted)" : "var(--text)",
                    }}
                  >
                    {n}
                  </span>
                </div>
              );
            })}
          </Fragment>
        ))}
      </div>

      <p className="muted" style={{ fontSize: "0.75rem", margin: "0.5rem 0 0" }}>
        Decks playing each color (outlined diagonal) and each pair of colors together.
      </p>

      {tip && (
        <div className="chart-tip" style={{ left: tip.left, top: tip.top }} role="status">
          <strong>
            {tip.row === tip.col
              ? COLORS[tip.row].name
              : `${COLORS[tip.row].name} + ${COLORS[tip.col].name}`}
          </strong>
          <span>{describe(tip.row, tip.col)}</span>
          <span className="muted">{pct(counts[tip.row][tip.col] / total, 0)} of their decks</span>
        </div>
      )}
    </div>
  );
}
