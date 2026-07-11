// Star/radar plot for a single series over a handful of named axes (the WUBRG
// color pie; deck color count 1–5). Hand-rolled SVG — the app carries no chart
// library and two 5-axis plots don't justify one.
//
// One series, so the polygon wears a single hue (--accent) rather than being
// striped by axis: an axis's own color lives in its vertex dot and label. Every
// axis is direct-labeled with its name and value, which is also what makes the
// mana palette legible — MTG's black is a zero-chroma gray, and its gold/green
// sit under 3:1 against the surface, so color alone could never carry identity here.

export interface RadarAxis {
  key: string;
  label: string;
  value: number;
  hex?: string; // the axis's own identity color, for its dot + label
  sublabel?: string; // e.g. a winrate, shown under the value
}

const SIZE = 260; // viewBox is square; the SVG scales to its container
const CENTER = SIZE / 2;
const RADIUS = 78; // leaves room for the labels ringing the plot
const RINGS = 4;

// Vertex i, at fraction f of full radius. Starts at 12 o'clock and goes clockwise,
// which is how the MTG color pie is conventionally drawn.
function point(i: number, n: number, f: number): [number, number] {
  const angle = (i / n) * 2 * Math.PI - Math.PI / 2;
  return [CENTER + Math.cos(angle) * RADIUS * f, CENTER + Math.sin(angle) * RADIUS * f];
}

const polygon = (pts: [number, number][]) => pts.map(([x, y]) => `${x},${y}`).join(" ");

// A "nice" upper bound so the outer ring is a round number the reader can anchor on.
function niceMax(values: number[]): number {
  const max = Math.max(...values, 0);
  if (max <= 0) return 1;
  if (max <= 5) return max;
  const step = Math.pow(10, Math.floor(Math.log10(max)));
  return Math.ceil(max / step) * step;
}

export function RadarChart({
  axes,
  caption,
}: {
  axes: RadarAxis[];
  caption?: string;
}) {
  const n = axes.length;
  if (n < 3) return null;

  const max = niceMax(axes.map((a) => a.value));
  const empty = axes.every((a) => a.value === 0);
  const shape = axes.map((a, i) => point(i, n, a.value / max));

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

        {/* Vertices carry each axis's identity color, ringed in the surface so they
            stay separate where the polygon edge runs underneath. */}
        {!empty &&
          axes.map((a, i) => {
            const [x, y] = shape[i];
            return (
              <circle
                key={a.key}
                cx={x}
                cy={y}
                r={4.5}
                fill={a.hex ?? "var(--accent)"}
                stroke="var(--surface)"
                strokeWidth={2}
              >
                <title>{`${a.label}: ${a.value}`}</title>
              </circle>
            );
          })}

        {/* Direct labels — name, value, and the optional sublabel, outside the plot. */}
        {axes.map((a, i) => {
          const [x, y] = point(i, n, 1.34);
          const anchor = Math.abs(x - CENTER) < 4 ? "middle" : x > CENTER ? "start" : "end";
          return (
            <g key={a.key}>
              <text
                x={x}
                y={y}
                textAnchor={anchor}
                fill="var(--text-secondary)"
                fontSize={11}
                dominantBaseline="middle"
              >
                {a.label}
              </text>
              <text
                x={x}
                y={y + 14}
                textAnchor={anchor}
                fill="var(--text)"
                fontSize={13}
                fontWeight={600}
                dominantBaseline="middle"
              >
                {a.value}
                {a.sublabel && (
                  <tspan fill="var(--muted)" fontSize={11} fontWeight={400}>
                    {" "}
                    {a.sublabel}
                  </tspan>
                )}
              </text>
            </g>
          );
        })}
      </svg>
      <p className="muted" style={{ fontSize: "0.78rem", textAlign: "center", margin: 0 }}>
        {empty ? "No decks yet." : `Outer ring = ${max} decks`}
      </p>
    </div>
  );
}
