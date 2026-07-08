"use client";

import { useEffect, useState, useCallback } from "react";
import {
  apiGet,
  apiGetOptional,
  type ColorStat,
  type CardStat,
  type CardPair,
} from "@/lib/api";
import { ColorPips } from "@/components/ColorPips";
import { ColorWinrateChart } from "@/components/ColorWinrateChart";
import { pct, signedPct } from "@/lib/format";

type Sort = "inclusion_rate" | "winrate_lift" | "wilson_lower";

const SORTS: { key: Sort; label: string }[] = [
  { key: "inclusion_rate", label: "Popularity" },
  { key: "winrate_lift", label: "Lift" },
  { key: "wilson_lower", label: "Wilson" },
];

export function AnalyticsDashboard({ cubes }: { cubes: { id: string; name: string }[] }) {
  const [cubeId, setCubeId] = useState(cubes[0].id);
  const [colors, setColors] = useState<ColorStat[]>([]);
  const [cards, setCards] = useState<CardStat[]>([]);
  const [sort, setSort] = useState<Sort>("inclusion_rate");
  const [selected, setSelected] = useState<CardStat | null>(null);
  const [pairs, setPairs] = useState<CardPair[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let live = true;
    setLoading(true);
    setErr(null);
    setSelected(null);
    Promise.all([
      apiGetOptional<ColorStat[]>(`/analytics/colors?cube=${cubeId}`),
      apiGetOptional<CardStat[]>(`/analytics/cards?cube=${cubeId}&sort=${sort}&limit=100`),
    ])
      .then(([c, k]) => {
        if (!live) return;
        setColors(c ?? []);
        setCards(k ?? []);
        if (c === null && k === null) setErr("No analytics available for this cube yet.");
      })
      .catch((e) => live && setErr(String(e)))
      .finally(() => live && setLoading(false));
    return () => {
      live = false;
    };
  }, [cubeId, sort]);

  const selectCard = useCallback(
    (c: CardStat) => {
      setSelected(c);
      apiGet<CardPair[]>(`/analytics/pairs?cube=${cubeId}&card=${c.card_id}&limit=15`)
        .then(setPairs)
        .catch(() => setPairs([]));
    },
    [cubeId]
  );

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "1.5rem" }}>
      {cubes.length > 1 && (
        <div>
          <select
            value={cubeId}
            onChange={(e) => setCubeId(e.target.value)}
            style={{ maxWidth: 260 }}
          >
            {cubes.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
              </option>
            ))}
          </select>
        </div>
      )}

      {err && <p className="muted">{err}</p>}
      {loading && <p className="muted">Loading…</p>}

      {!loading && !err && (
        <>
          <section className="card">
            <h2>Winrate by color</h2>
            <p className="muted" style={{ marginTop: "-0.25rem" }}>
              Decks containing each color (deck count in parentheses).
            </p>
            <ColorWinrateChart stats={colors} />
          </section>

          <section
            style={{
              display: "grid",
              gap: "1.5rem",
              gridTemplateColumns: "minmax(0, 2fr) minmax(0, 1fr)",
              alignItems: "start",
            }}
          >
            <div className="card">
              <div
                style={{
                  display: "flex",
                  justifyContent: "space-between",
                  alignItems: "center",
                  marginBottom: "0.5rem",
                }}
              >
                <h2 style={{ margin: 0 }}>Cards</h2>
                <div style={{ display: "flex", gap: 6 }}>
                  {SORTS.map((s) => (
                    <button
                      key={s.key}
                      onClick={() => setSort(s.key)}
                      className="pill"
                      style={{
                        cursor: "pointer",
                        background: sort === s.key ? "var(--accent-weak)" : "transparent",
                        color: sort === s.key ? "var(--text)" : "var(--text-secondary)",
                      }}
                    >
                      {s.label}
                    </button>
                  ))}
                </div>
              </div>
              <div style={{ overflowX: "auto" }}>
                <table>
                  <thead>
                    <tr>
                      <th>Card</th>
                      <th className="num">Incl.</th>
                      <th className="num">WR</th>
                      <th className="num">Lift</th>
                      <th className="num">Wilson</th>
                    </tr>
                  </thead>
                  <tbody>
                    {cards.map((c) => (
                      <tr
                        key={c.card_id}
                        onClick={() => selectCard(c)}
                        style={{
                          cursor: "pointer",
                          background:
                            selected?.card_id === c.card_id ? "var(--accent-weak)" : undefined,
                        }}
                      >
                        <td>
                          <span style={{ display: "inline-flex", alignItems: "center", gap: 6 }}>
                            <ColorPips bits={c.color_identity} />
                            {c.name}
                          </span>
                        </td>
                        <td className="num">{pct(c.inclusion_rate, 0)}</td>
                        <td className="num">{pct(c.winrate)}</td>
                        <td
                          className="num"
                          style={{
                            color:
                              (c.winrate_lift ?? 0) > 0
                                ? "var(--good)"
                                : (c.winrate_lift ?? 0) < 0
                                ? "var(--bad)"
                                : undefined,
                          }}
                        >
                          {signedPct(c.winrate_lift)}
                        </td>
                        <td className="num">{pct(c.wilson_lower)}</td>
                      </tr>
                    ))}
                    {cards.length === 0 && (
                      <tr>
                        <td colSpan={5} className="muted">
                          No cards in analyzed decks.
                        </td>
                      </tr>
                    )}
                  </tbody>
                </table>
              </div>
            </div>

            <div className="card">
              <h2>Played with</h2>
              {!selected ? (
                <p className="muted">Select a card to see its strongest associations.</p>
              ) : (
                <>
                  <p style={{ marginTop: "-0.25rem" }}>
                    <strong>{selected.name}</strong>
                  </p>
                  {pairs.length === 0 ? (
                    <p className="muted">No co-occurring cards (needs ≥2 shared decks).</p>
                  ) : (
                    <table>
                      <thead>
                        <tr>
                          <th>Card</th>
                          <th className="num">Lift</th>
                          <th className="num">Conf.</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pairs.map((p) => (
                          <tr key={p.card_b_id}>
                            <td>{p.name}</td>
                            <td className="num">{p.lift.toFixed(2)}×</td>
                            <td className="num">{pct(p.confidence_ab, 0)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                </>
              )}
            </div>
          </section>
        </>
      )}
    </div>
  );
}
