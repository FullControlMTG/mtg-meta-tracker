"use client";

import { useState } from "react";
import { pct } from "@/lib/format";
import { placeFloating, type Placed } from "@/lib/hover";

// Star/radar plot for a single series over a handful of named axes (the WUBRG
// color pie; deck color count 1–5). Hand-rolled SVG — the app carries no chart
// library and two 5-axis plots don't justify one.
//
// One series, so the polygon wears a single hue (--accent) rather than being
// striped by axis: an axis's own color lives in its vertex dot. The plot itself only
// carries each axis's name; the numbers live in the hover tooltip, which is what lets the
// three parts of a data point (name, count, share) be read together as one sentence
// instead of stacked as three anonymous figures around the rim.

export interface RadarAxis {
  key: string;
  label: string; // "Blue", "3 colors", "Mono"
  value: number; // the count, and what gets plotted
  hex?: string; // the axis's own identity color, for its dot
  share?: number | null; // 0..1 of the caller's denominator
  note?: string; // the caller's sentence, e.g. "86% of meta plays 3 colors". A
  // pre-rendered string, not a formatter: this is a client
  // component, and a function prop can't cross the boundary.
}

const SIZE = 260; // viewBox is square; the SVG scales to its container
const CENTER = SIZE / 2;
const RADIUS = 78; // leaves room for the labels ringing the plot
const RINGS = 4;
const TIP_W = 220;
const TIP_H = 76; // nominal, only for clamping the tooltip inside the viewport

// Vertex i, at fraction f of full radius. Starts at 12 o'clock and goes clockwise,
// which is how the MTG color pie is conventionally drawn.
function point(i: number, n: number, f: number): [number, number] {
  const angle = (i / n) * 2 * Math.PI - Math.PI / 2;
  return [CENTER + Math.cos(angle) * RADIUS * f, CENTER + Math.sin(angle) * RADIUS * f];
}

const polygon = (pts: [number, number][]) => pts.map(([x, y]) => `${x},${y}`).join(" ");

export function RadarChart({
  axes,
  caption,
  unit = "deck",
}: {
  axes: RadarAxis[];
  caption?: string;
  unit?: string; // singular; pluralized here → "12 decks", "1 deck"
}) {
  const [tip, setTip] = useState<(Placed & { axis: RadarAxis }) | null>(null);

  const n = axes.length;
  if (n < 3) return null;

  const count = (v: number) => `${v} ${unit}${v === 1 ? "" : "s"}`;

  // Max-normalized: the largest axis lands exactly on the outer ring, so the shape always
  // fills the plot and the reader compares the axes against each other rather than against
  // a rounded-up number that needed a caption to explain it. The rings are bare reference
  // now — which is why every value is spelled out with its unit in the tooltip.
  const max = Math.max(...axes.map((a) => a.value), 0);
  const empty = max <= 0;
  const shape = axes.map((a, i) => point(i, n, a.value / (max || 1)));

  const show = (axis: RadarAxis, x: number, y: number) =>
    setTip({ axis, ...placeFloating(x, y, TIP_W, TIP_H) });

  // Hovering the vertex or its name both open the tooltip — the dot alone is a 9px target.
  const hover = (axis: RadarAxis) => ({
    onMouseEnter: (e: React.MouseEvent) => show(axis, e.clientX, e.clientY),
    onMouseMove: (e: React.MouseEvent) => show(axis, e.clientX, e.clientY),
    onMouseLeave: () => setTip(null),
    onFocus: (e: React.FocusEvent<SVGElement>) => {
      const r = e.currentTarget.getBoundingClientRect();
      show(axis, r.right, r.top + r.height / 2);
    },
    onBlur: () => setTip(null),
    tabIndex: 0,
    role: "img",
    "aria-label": [axis.label, count(axis.value), axis.note].filter(Boolean).join(" — "),
    style: { cursor: "pointer" },
  });

  return (
    <div>
      <svg
        viewBox={`0 0 ${SIZE} ${SIZE}`}
        width="100%"
        style={{ display: "block", maxHeight: 300, overflow: "visible" }}
        role="img"
        aria-label={
          caption ??
          `Radar chart: ${axes.map((a) => `${a.label} ${a.value}`).join(", ")}`
        }
      >
        {/* Grid: solid hairlines, one shade off the surface. */}
        {Array.from({ length: RINGS }, (_, r) => {
          const f = (r + 1) / RINGS;
          return (
            <polygon
              key={r}
              points={polygon(axes.map((_, i) => point(i, n, f)))}
              fill="none"
              stroke="var(--grid)"
              strokeWidth={1}
            />
          );
        })}
        {axes.map((a, i) => {
          const [x, y] = point(i, n, 1);
          return (
            <line
              key={a.key}
              x1={CENTER}
              y1={CENTER}
              x2={x}
              y2={y}
              stroke="var(--grid)"
              strokeWidth={1}
            />
          );
        })}

        {/* The series. */}
        {!empty && (
          <polygon
            points={polygon(shape)}
            fill="var(--accent)"
            fillOpacity={0.16}
            stroke="var(--accent)"
            strokeWidth={2}
            strokeLinejoin="round"
          />
        )}

        {/* Vertices carry each axis's identity color, ringed in the surface so they stay
            separate where the polygon edge runs underneath, then ringed again in
            --pip-ring so white and black stay visible against their own surface. */}
        {!empty &&
          axes.map((a, i) => {
            const [x, y] = shape[i];
            const active = tip?.axis.key === a.key;
            return (
              <g key={a.key} pointerEvents="none">
                <circle
                  cx={x}
                  cy={y}
                  r={active ? 6 : 4.5}
                  fill={a.hex ?? "var(--accent)"}
                  stroke="var(--surface)"
                  strokeWidth={2}
                />
                <circle
                  cx={x}
                  cy={y}
                  r={(active ? 6 : 4.5) + 1}
                  fill="none"
                  stroke="var(--pip-ring)"
                  strokeWidth={1}
                />
              </g>
            );
          })}

        {/* Hit targets, over the dots — the visible ones are pointer-transparent. */}
        {!empty &&
          axes.map((a, i) => {
            const [x, y] = shape[i];
            return <circle key={a.key} cx={x} cy={y} r={11} fill="transparent" {...hover(a)} />;
          })}

        {/* Direct labels — the name only; the numbers are in the tooltip. */}
        {axes.map((a, i) => {
          const [x, y] = point(i, n, 1.34);
          const anchor = Math.abs(x - CENTER) < 4 ? "middle" : x > CENTER ? "start" : "end";
          return (
            <text
              key={a.key}
              x={x}
              y={y}
              textAnchor={anchor}
              fill={tip?.axis.key === a.key ? "var(--text)" : "var(--text-secondary)"}
              fontSize={12}
              dominantBaseline="middle"
              {...(empty ? {} : hover(a))}
            >
              {a.label}
            </text>
          );
        })}
      </svg>

      {empty && (
        <p className="muted" style={{ fontSize: "0.78rem", textAlign: "center", margin: 0 }}>
          No decks yet.
        </p>
      )}

      {/* Fixed, and a sibling of the SVG rather than a <g> inside it: the viewBox scales
          with the container, so an in-SVG tooltip's text would grow and shrink with the
          card's width. */}
      {tip && (
        <div className="chart-tip" style={{ left: tip.left, top: tip.top }} role="status">
          <strong>{tip.axis.label}</strong>
          <span>
            {count(tip.axis.value)}
            {tip.axis.share != null && ` · ${pct(tip.axis.share, 0)}`}
          </span>
          {tip.axis.note && <span className="muted">{tip.axis.note}</span>}
        </div>
      )}
    </div>
  );
}
