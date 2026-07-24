"use client";

import { useState } from "react";
import { type ColorTrendPoint } from "@/lib/api";
import { COLORS } from "@/lib/colors";
import { fmtDate, pct } from "@/lib/format";
import { placeFloating, type Placed } from "@/lib/hover";

// A 100% stacked area of the color pie over time: each band is one of WUBRG, and the
// five together fill the plot on every day, so the chart reads as "what share of the
// meta was each color" rather than "how many decks were there".
//
// The x axis is real time, not the point index — two decks a day apart and two decks a
// month apart should not draw the same width of band. Points exist only for days a deck
// was played (nothing changed in between), and the straight line between two points is
// the honest reading of a period nobody played in.
//
// Hand-rolled SVG; the app carries no chart library.
//
// On the palette: these are MTG's own five colors, so the hues are semantic and cannot
// be re-picked for contrast — White is a near-white and Black a near-black, and they
// fail a generic palette check against both surfaces. Every band therefore carries a
// --pip-ring outline (the same rule as ColorPips), a legend, an in-band letter where it
// fits, and a tooltip that names every series: identity never rests on hue alone.
// The outline does the job a 2px surface gap would do between stacked segments, without
// punching holes through a chart whose whole claim is that it sums to 100%.

const W = 720;
const H = 250;
const PAD = { top: 10, right: 10, bottom: 24, left: 38 };
const PLOT_W = W - PAD.left - PAD.right;
const PLOT_H = H - PAD.top - PAD.bottom;
const TIP_W = 220;
const TIP_H = 150;
// Under this share a band is thinner than its own label.
const LABEL_MIN_SHARE = 0.1;

const dayMs = 86400000;
// "2026-07-24" → epoch day. Parsed as UTC (the Z), so the arithmetic never picks up a
// local DST hour and lands the point a day early.
const epochDay = (iso: string) => Date.parse(iso + "T00:00:00Z") / dayMs;

const share = (p: ColorTrendPoint, bit: number) =>
  p.colors.find((c) => c.color === bit)?.share ?? 0;
const count = (p: ColorTrendPoint, bit: number) =>
  p.colors.find((c) => c.color === bit)?.deck_count ?? 0;

export function ColorTrendChart({ points }: { points: ColorTrendPoint[] }) {
  const [tip, setTip] = useState<(Placed & { i: number }) | null>(null);

  if (points.length === 0) return <p className="muted">No dated decks yet.</p>;

  const days = points.map((p) => epochDay(p.as_of));
  const first = days[0];
  const span = days[days.length - 1] - first;

  // A single day — or several decks all played on one — has no span to scale across.
  // Draw it as one flat stack the width of the plot: the shares are real, the passage
  // of time is what there isn't any of yet.
  const x = (i: number) =>
    span > 0
      ? PAD.left + ((days[i] - first) / span) * PLOT_W
      : PAD.left + PLOT_W / 2;
  // Share runs up the axis: 0% on the baseline, 100% at the top, as a share axis
  // conventionally does. The stack therefore builds from the bottom, and WUBRG reads
  // bottom-to-top — the same order as the legend.
  const y = (cum: number) => PAD.top + PLOT_H - cum * PLOT_H;

  // Cumulative share *under* each band, per point, stacked in WUBRG order from the base.
  const base: number[][] = [];
  let running = points.map(() => 0);
  for (const c of COLORS) {
    base.push(running);
    running = running.map((v, i) => v + share(points[i], c.bit));
  }

  const flat = span <= 0;
  const bandPath = (ci: number) => {
    const bottom = base[ci];
    const top = bottom.map((v, i) => v + share(points[i], COLORS[ci].bit));
    if (flat) {
      // One point: a rectangle across the plot rather than a zero-width sliver.
      return `M ${PAD.left} ${y(top[0])} L ${PAD.left + PLOT_W} ${y(top[0])} L ${
        PAD.left + PLOT_W
      } ${y(bottom[0])} L ${PAD.left} ${y(bottom[0])} Z`;
    }
    // Along the top edge left to right, back along the bottom edge right to left.
    const down = top.map((v, i) => `${i === 0 ? "M" : "L"} ${x(i)} ${y(v)}`);
    const back = [];
    for (let i = bottom.length - 1; i >= 0; i--)
      back.push(`L ${x(i)} ${y(bottom[i])}`);
    return `${down.join(" ")} ${back.join(" ")} Z`;
  };

  const show = (i: number, cx: number, cy: number) =>
    setTip({ i, ...placeFloating(cx, cy, TIP_W, TIP_H) });

  // The crosshair finds the x: the reader aims at a date, not at a 1px line. Compared
  // in viewBox space, since the SVG is scaled to whatever width its container has.
  const nearest = (clientX: number, rect: DOMRect) => {
    const vx = ((clientX - rect.left) / rect.width) * PLOT_W + PAD.left;
    let best = 0;
    for (let i = 1; i < points.length; i++) {
      if (Math.abs(x(i) - vx) < Math.abs(x(best) - vx)) best = i;
    }
    return best;
  };

  const track = (e: React.MouseEvent<SVGRectElement>) => {
    const rect = e.currentTarget.getBoundingClientRect();
    show(nearest(e.clientX, rect), e.clientX, e.clientY);
  };

  // Keyboard gets the same readout as the pointer: step the crosshair along the series.
  const onKey = (e: React.KeyboardEvent<SVGRectElement>) => {
    if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
    e.preventDefault();
    const cur = tip?.i ?? 0;
    const next = Math.min(
      points.length - 1,
      Math.max(0, cur + (e.key === "ArrowRight" ? 1 : -1)),
    );
    const r = e.currentTarget.getBoundingClientRect();
    const at = r.left + (x(next) / W) * r.width;
    show(next, at, r.top + r.height / 2);
  };

  const active = tip ? points[tip.i] : null;

  return (
    <div>
      <svg
        viewBox={`0 0 ${W} ${H}`}
        width="100%"
        style={{ display: "block", maxHeight: 300, overflow: "visible" }}
        role="img"
        aria-label={`Share of the color pie over time, ${points.length} day${
          points.length === 1 ? "" : "s"
        } from ${points[0].as_of} to ${points[points.length - 1].as_of}`}
      >
        {/* Grid: quarters of the stack, recessive. */}
        {[0, 0.25, 0.5, 0.75, 1].map((g) => (
          <g key={g}>
            <line
              x1={PAD.left}
              y1={y(g)}
              x2={PAD.left + PLOT_W}
              y2={y(g)}
              stroke="var(--grid)"
              strokeWidth={1}
            />
            <text
              x={PAD.left - 6}
              y={y(g)}
              textAnchor="end"
              dominantBaseline="middle"
              fill="var(--muted)"
              fontSize={10}
            >
              {Math.round(g * 100)}%
            </text>
          </g>
        ))}

        {COLORS.map((c, ci) => (
          <path
            key={c.bit}
            d={bandPath(ci)}
            fill={c.hex}
            stroke="var(--pip-ring)"
            strokeWidth={1}
            strokeLinejoin="round"
          />
        ))}

        {/* In-band letters — secondary encoding, so a band is identifiable without its
            hue. Only on the last point, and only where the band is tall enough. */}
        {COLORS.map((c, ci) => {
          const last = points.length - 1;
          const s = share(points[last], c.bit);
          if (s < LABEL_MIN_SHARE) return null;
          return (
            <text
              key={c.bit}
              x={PAD.left + PLOT_W - 6}
              y={y(base[ci][last] + s / 2)}
              textAnchor="end"
              dominantBaseline="middle"
              fontSize={11}
              fontWeight={600}
              fill="var(--text)"
              stroke="var(--surface)"
              strokeWidth={2.5}
              paintOrder="stroke"
            >
              {c.code}
            </text>
          );
        })}

        {/* Crosshair. */}
        {tip && !flat && (
          <line
            x1={x(tip.i)}
            y1={PAD.top}
            x2={x(tip.i)}
            y2={PAD.top + PLOT_H}
            stroke="var(--text)"
            strokeWidth={1}
            strokeDasharray="3 3"
            pointerEvents="none"
          />
        )}

        {/* Dates: the ends only. Every point gets a tick, but a label per point
            collides the moment a cube has a dozen play nights. */}
        {points.map((p, i) => (
          <line
            key={p.as_of}
            x1={x(i)}
            y1={PAD.top + PLOT_H}
            x2={x(i)}
            y2={PAD.top + PLOT_H + 4}
            stroke="var(--grid)"
            strokeWidth={1}
          />
        ))}
        <text x={PAD.left} y={H - 6} fill="var(--muted)" fontSize={10}>
          {points[0].as_of}
        </text>
        {points.length > 1 && (
          <text
            x={PAD.left + PLOT_W}
            y={H - 6}
            textAnchor="end"
            fill="var(--muted)"
            fontSize={10}
          >
            {points[points.length - 1].as_of}
          </text>
        )}

        {/* One hit target over the whole plot: the pointer only has to be nearest. */}
        <rect
          x={PAD.left}
          y={PAD.top}
          width={PLOT_W}
          height={PLOT_H}
          fill="transparent"
          tabIndex={0}
          onMouseMove={track}
          onMouseEnter={track}
          onMouseLeave={() => setTip(null)}
          onFocus={(e) => {
            const r = e.currentTarget.getBoundingClientRect();
            show(points.length - 1, r.right, r.top + r.height / 2);
          }}
          onBlur={() => setTip(null)}
          onKeyDown={onKey}
          style={{ cursor: "crosshair" }}
        />
      </svg>

      {/* Legend: five series, so identity never rests on the bands alone. */}
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: "0.75rem",
          marginTop: "0.5rem",
          fontSize: "0.78rem",
        }}
      >
        {COLORS.map((c) => (
          <span
            key={c.bit}
            style={{ display: "flex", alignItems: "center", gap: 5 }}
          >
            <span
              style={{
                width: 11,
                height: 11,
                borderRadius: 3,
                background: c.hex,
                border: "1px solid var(--pip-ring)",
              }}
            />
            <span className="muted">{c.name}</span>
          </span>
        ))}
      </div>

      {tip && active && (
        <div
          className="chart-tip"
          style={{ left: tip.left, top: tip.top }}
          role="status"
        >
          <strong>{fmtDate(active.as_of)}</strong>
          <span className="muted">
            {active.total_decks} deck{active.total_decks === 1 ? "" : "s"} so
            far
          </span>
          {COLORS.map((c) => (
            <span
              key={c.bit}
              style={{ display: "flex", alignItems: "center", gap: 6 }}
            >
              <span
                style={{
                  width: 10,
                  height: 2,
                  background: c.hex,
                  border: "1px solid var(--pip-ring)",
                  flexShrink: 0,
                }}
              />
              <strong>{pct(share(active, c.bit), 0)}</strong>
              <span className="muted">
                {c.name} · {count(active, c.bit)}
              </span>
            </span>
          ))}
        </div>
      )}
    </div>
  );
}
