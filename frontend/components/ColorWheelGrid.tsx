"use client";

import { Fragment, useState } from "react";
import { COLORS, colorCount, comboName, identityString } from "@/lib/colors";
import { pct } from "@/lib/format";
import { placeFloating, type Placed } from "@/lib/hover";

// All 31 color combinations, one cell each, none of them a special case.
//
// A 5×5 grid is the obvious way to show colors crossed with colors, and it is the
// wrong shape: a pair is a Cartesian product, but a deck's colors are a *subset*,
// and subsets of varying size are not a product of anything. The old pairing grid
// paid for that by dissolving every deck above two colors — a Jund deck landed in
// BR, BG and RG, indistinguishable from three separate two-color decks.
//
// The way back to a grid is the pentagon. Magic's five colors carry a fixed cyclic
// order in which neighbours are allied and the skips are enemies, and rotating that
// pentagon by a fifth maps each *kind* of combination onto itself. So each kind has
// exactly five members, one anchored on each color:
//
//   mono · allied pair · enemy pair · shard · wedge · four-color   = 6 × 5 = 30
//
// plus WUBRG, which is its own orbit of one. That is 31 — every combination exactly
// once, nothing missing and nothing duplicated, which is worth checking by hand if
// you ever touch the orbit rules below. It gives two axes that both mean something:
// rows are the anchoring color, columns are the family. And because the families
// happen to run 1, 2, 2, 3, 3, 4 colors deep, the column axis is also the color-count
// axis the pairing grid never had.
//
// A row is therefore *not* "every deck containing white" — Azorius sits in the white
// row and not also in the blue one. That question is the Colors Played radar's, and
// this grid deliberately leaves it there rather than duplicating cells to answer it.
//
// Splashes are left out by the caller, like everywhere else in the app: a deck's
// colors are the ones it is built on.

const ALL = COLORS.reduce((bits, c) => bits | c.bit, 0);

// COLORS is in WUBRG order, which *is* the pentagon's order, so a neighbour is one
// step and an enemy is two. Everything below is that one fact.
const step = (i: number) => COLORS[i % COLORS.length].bit;

interface Family {
  key: string;
  label: string;
  kind: string; // how the tooltip says it, in a sentence
  of: (i: number) => number;
}

const FAMILIES: Family[] = [
  { key: "mono", label: "Mono", kind: "mono", of: (i) => step(i) },
  { key: "allied", label: "Allied", kind: "allied pair", of: (i) => step(i) | step(i + 1) },
  { key: "enemy", label: "Enemy", kind: "enemy pair", of: (i) => step(i) | step(i + 2) },
  {
    key: "shard",
    label: "Shard",
    kind: "shard",
    of: (i) => step(i) | step(i + 1) | step(i + 4),
  },
  {
    key: "wedge",
    label: "Wedge",
    kind: "wedge",
    of: (i) => step(i) | step(i + 2) | step(i + 3),
  },
  { key: "four", label: "Four-color", kind: "four-color", of: (i) => ALL & ~step(i) },
];

// The glyph is drawn in a fixed 32-unit box and scaled by CSS, so one set of
// geometry serves the 70px cells of a wide card and the 40px cells of a phone.
const BOX = 32;
const MID = BOX / 2;
const RIM = MID - 1;

// Sector i is centred on color i's direction with white at twelve o'clock, matching
// the radar and the pie every player already has in their head.
function sector(i: number): string {
  const at = (deg: number): [number, number] => {
    const r = ((deg - 90) * Math.PI) / 180;
    return [MID + Math.cos(r) * RIM, MID + Math.sin(r) * RIM];
  };
  const [x1, y1] = at(i * 72 - 36);
  const [x2, y2] = at(i * 72 + 36);
  const f = (n: number) => n.toFixed(2);
  return `M ${MID} ${MID} L ${f(x1)} ${f(y1)} A ${RIM} ${RIM} 0 0 1 ${f(x2)} ${f(y2)} Z`;
}

const SECTORS = COLORS.map((_, i) => sector(i));

const MAX_ALPHA = 0.5;
const TIP_W = 220;
const TIP_H = 104;

// A combination they have never built keeps its shape and loses its color: the map
// is the same for every player, and only the data lights it. Drawn in --grid rather
// than dimmed with a filter — a filter on any ancestor of a `position: fixed` box
// makes it the containing block, which is exactly the trap lib/hover.ts warns about.
function Wheel({ bits, built }: { bits: number; built: boolean }) {
  return (
    <svg
      viewBox={`0 0 ${BOX} ${BOX}`}
      style={{ width: "100%", maxWidth: 34, height: "auto", display: "block" }}
      aria-hidden="true"
    >
      {/* The glyph carries its own ground, so the lit sectors read the same over a
          plain cell and over one tinted by the count behind it. */}
      <circle cx={MID} cy={MID} r={RIM} fill="var(--surface)" />
      {COLORS.map((c, i) =>
        (bits & c.bit) === 0 ? null : (
          <path
            key={c.bit}
            d={SECTORS[i]}
            fill={built ? c.hex : "var(--grid)"}
            stroke={built ? "var(--pip-ring)" : "none"}
            strokeWidth={0.75}
          />
        ),
      )}
      <circle cx={MID} cy={MID} r={RIM} fill="none" stroke="var(--grid)" />
    </svg>
  );
}

export function ColorWheelGrid({
  bitsets,
  unit = "deck",
}: {
  bitsets: number[]; // one color identity per deck
  unit?: string;
}) {
  const [tip, setTip] = useState<(Placed & { bits: number; kind: string }) | null>(null);

  const total = bitsets.length;
  const counts = new Map<number, number>();
  for (const bits of bitsets) counts.set(bits, (counts.get(bits) ?? 0) + 1);
  const max = Math.max(...counts.values(), 0);

  if (total === 0) return <p className="muted">No decks yet.</p>;

  const show = (bits: number, kind: string, x: number, y: number) =>
    setTip({ bits, kind, ...placeFloating(x, y, TIP_W, TIP_H) });

  const plural = (n: number) => `${n} ${unit}${n === 1 ? "" : "s"}`;

  // Four- and five-color combinations are *named* by their letters, so spelling the
  // letters out again beside the name would read "UBRG (UBRG)".
  const named = (bits: number) => comboName(bits) !== identityString(bits);

  const describe = (bits: number) => {
    const n = counts.get(bits) ?? 0;
    const title = named(bits)
      ? `${comboName(bits)} (${identityString(bits)})`
      : comboName(bits);
    return n === 0
      ? `${title} — never built`
      : `${title} — ${plural(n)}, ${pct(n / total, 0)} of their decks`;
  };

  const cell = (bits: number, kind: string, style?: React.CSSProperties) => {
    const n = counts.get(bits) ?? 0;
    const active = tip?.bits === bits;
    return (
      <div
        key={bits}
        tabIndex={0}
        role="img"
        aria-label={describe(bits)}
        onMouseEnter={(e) => show(bits, kind, e.clientX, e.clientY)}
        onMouseMove={(e) => show(bits, kind, e.clientX, e.clientY)}
        onMouseLeave={() => setTip(null)}
        onFocus={(e) => {
          const r = e.currentTarget.getBoundingClientRect();
          show(bits, kind, r.right, r.top + r.height / 2);
        }}
        onBlur={() => setTip(null)}
        style={{
          position: "relative",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: 3,
          padding: "5px 3px 4px",
          borderRadius: 5,
          background: n > 0 ? "var(--grid)" : "transparent",
          // An unbuilt combination is an outline rather than a filled tile, so the
          // lit part of the grid is what the eye lands on first.
          boxShadow: n > 0 ? "none" : "inset 0 0 0 1px var(--grid)",
          outline: active ? "2px solid var(--accent)" : "none",
          outlineOffset: 1,
          cursor: "pointer",
          ...style,
        }}
      >
        <span
          style={{
            position: "absolute",
            inset: 0,
            borderRadius: 5,
            background: "var(--accent)",
            opacity: max > 0 ? (n / max) * MAX_ALPHA : 0,
          }}
        />
        <span style={{ position: "relative", width: "100%", display: "flex", justifyContent: "center" }}>
          <Wheel bits={bits} built={n > 0} />
        </span>
        {/* An interpunct rather than a "0": twenty of the thirty-one cells are empty
            on a normal player's page, and twenty zeroes read as data. */}
        <span
          style={{
            position: "relative",
            fontSize: "0.7rem",
            lineHeight: 1,
            fontVariantNumeric: "tabular-nums",
            color: n === 0 ? "var(--muted)" : "var(--text)",
          }}
        >
          {n === 0 ? "·" : n}
        </span>
      </div>
    );
  };

  return (
    <div>
      <div
        style={{
          display: "grid",
          gridTemplateColumns: `1.1rem repeat(${FAMILIES.length}, minmax(0, 1fr))`,
          gap: 3,
        }}
      >
        {/* Column headers, offset by the empty corner above the row headers. */}
        <span />
        {FAMILIES.map((f) => (
          <span
            key={f.key}
            className="muted"
            style={{
              fontSize: "0.62rem",
              fontWeight: 600,
              letterSpacing: "0.05em",
              textTransform: "uppercase",
              textAlign: "center",
              lineHeight: 1.2,
              paddingBottom: 2,
            }}
          >
            {f.label}
          </span>
        ))}

        {/* One flat list of cells — a wrapper per row would break the column tracks —
            so a row is a fragment of its color pip plus one cell per family. */}
        {COLORS.map((c, i) => (
          <Fragment key={c.bit}>
            <span
              title={c.name}
              style={{ display: "flex", alignItems: "center", justifyContent: "center" }}
            >
              <span
                style={{
                  width: 11,
                  height: 11,
                  borderRadius: "50%",
                  background: c.hex,
                  border: "1px solid var(--pip-ring)",
                  display: "inline-block",
                }}
              />
            </span>
            {FAMILIES.map((f) => cell(f.of(i), f.kind))}
          </Fragment>
        ))}
      </div>

      {/* WUBRG is the one combination with no orbit of five to sit in, so it gets a
          line of its own under the grid rather than a column that would be empty
          four rows out of five. */}
      <div
        style={{
          display: "flex",
          alignItems: "center",
          gap: "0.6rem",
          marginTop: "0.6rem",
          paddingTop: "0.6rem",
          borderTop: "1px solid var(--border)",
        }}
      >
        {cell(ALL, "five-color", { flex: "0 0 3rem" })}
        <span style={{ fontSize: "0.8rem", color: "var(--text-secondary)" }}>
          All five — <strong style={{ color: "var(--text)" }}>WUBRG</strong>
        </span>
      </div>

      <p className="muted" style={{ fontSize: "0.75rem", margin: "0.5rem 0 0" }}>
        Each shape is the slice of the color pie a deck is built on. Grey shapes are
        combinations they have never played.
      </p>

      {tip && (
        <div className="chart-tip" style={{ left: tip.left, top: tip.top }} role="status">
          <strong>{comboName(tip.bits)}</strong>
          <span className="muted">
            {named(tip.bits) ? `${identityString(tip.bits)} · ${tip.kind}` : tip.kind}
          </span>
          <span>
            {(counts.get(tip.bits) ?? 0) === 0
              ? "Never built"
              : `${plural(counts.get(tip.bits) ?? 0)} on ${colorCount(tip.bits) === 1 ? "this color" : "these colors"}`}
          </span>
          <span className="muted">
            {pct((counts.get(tip.bits) ?? 0) / total, 0)} of their decks
          </span>
        </div>
      )}
    </div>
  );
}
