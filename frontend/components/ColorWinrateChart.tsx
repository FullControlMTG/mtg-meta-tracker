"use client";

import { type ColorStat } from "@/lib/api";
import { colorByBit } from "@/lib/colors";
import { pct } from "@/lib/format";

// Horizontal winrate bars for the single_color facet — one bar per WUBRG color,
// each in its mana color, with the winrate direct-labeled (relief rule) so the
// lighter hues never rely on color-contrast alone.
export function ColorWinrateChart({ stats }: { stats: ColorStat[] }) {
  const rows = stats
    .filter((s) => s.facet === "single_color")
    .sort((a, b) => a.facet_key - b.facet_key);

  if (rows.length === 0) return <p className="muted">No color data.</p>;

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
      {rows.map((s) => {
        const c = colorByBit(s.facet_key);
        const wr = s.winrate ?? 0;
        return (
          <div key={s.facet_key} style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <span style={{ width: 46, fontSize: "0.85rem" }} className="muted">
              {c.name}
            </span>
            <div
              style={{
                flex: 1,
                background: "var(--grid)",
                borderRadius: 4,
                height: 20,
                position: "relative",
              }}
            >
              <div
                style={{
                  width: `${Math.max(wr * 100, 1)}%`,
                  background: c.hex,
                  // White's bar is near-white, black's near-black — the ring is what keeps
                  // each a bar rather than a hole in the track.
                  border: "1px solid var(--pip-ring)",
                  height: "100%",
                  borderRadius: 4,
                }}
              />
            </div>
            <span
              style={{ width: 96, textAlign: "right", fontVariantNumeric: "tabular-nums" }}
            >
              {pct(s.winrate)}{" "}
              <span className="muted" style={{ fontSize: "0.8rem" }}>
                ({s.deck_count})
              </span>
            </span>
          </div>
        );
      })}
    </div>
  );
}
