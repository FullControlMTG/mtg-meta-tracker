import Link from "next/link";
import { notFound } from "next/navigation";
import {
  apiGetOptional,
  type CardStat,
  type ColorStat,
  type CubeView,
  type Overview,
} from "@/lib/api";
import { getCubes } from "@/lib/cube";
import { COLORS } from "@/lib/colors";
import { ColorPips } from "@/components/ColorPips";
import { CubeSwitcher } from "@/components/CubeSwitcher";
import { RadarChart, type RadarAxis } from "@/components/RadarChart";
import { StatTile } from "@/components/StatTile";
import { num, pct, signedPct } from "@/lib/format";

// The cube's stats page: overview counters and the analytics breakdown, merged.
// Re-rendered on demand by the backend revalidation webhook; hourly fallback.
export const revalidate = 3600;

type Sort = "inclusion_rate" | "winrate_lift" | "wilson_lower";

const SORTS: { key: Sort; label: string }[] = [
  { key: "inclusion_rate", label: "Popularity" },
  { key: "winrate_lift", label: "Lift" },
  { key: "wilson_lower", label: "Wilson" },
];

const isSort = (s?: string): s is Sort => SORTS.some((x) => x.key === s);

// The single_color facet, as one radar axis per WUBRG color: how often each color
// is played. A 2-color deck counts on both of its axes.
function colorAxes(stats: ColorStat[]): RadarAxis[] {
  const byKey = new Map(stats.filter((s) => s.facet === "single_color").map((s) => [s.facet_key, s]));
  return COLORS.map((c) => ({
    key: c.code,
    label: c.name,
    value: byKey.get(c.bit)?.deck_count ?? 0,
    hex: c.hex,
    sublabel: pct(byKey.get(c.bit)?.winrate, 0),
  }));
}

// The color_count facet: how many decks play 1, 2, 3, 4, or 5 colors.
function colorCountAxes(stats: ColorStat[]): RadarAxis[] {
  const byKey = new Map(stats.filter((s) => s.facet === "color_count").map((s) => [s.facet_key, s]));
  return [1, 2, 3, 4, 5].map((n) => ({
    key: String(n),
    label: n === 1 ? "Mono" : `${n} colors`,
    value: byKey.get(n)?.deck_count ?? 0,
    sublabel: pct(byKey.get(n)?.winrate, 0),
  }));
}

export default async function CubeStatsPage({
  params,
  searchParams,
}: {
  params: { cube: string };
  searchParams: { sort?: string };
}) {
  const cubeId = params.cube;
  const sort: Sort = isSort(searchParams.sort) ? searchParams.sort : "inclusion_rate";

  const [view, cubes] = await Promise.all([
    apiGetOptional<CubeView>(`/cubes/${cubeId}`, 300),
    getCubes(),
  ]);
  if (!view) notFound();

  const [overview, colors, cards] = await Promise.all([
    apiGetOptional<Overview>(`/analytics/overview?cube=${cubeId}`, 3600),
    apiGetOptional<ColorStat[]>(`/analytics/colors?cube=${cubeId}`, 3600),
    apiGetOptional<CardStat[]>(`/analytics/cards?cube=${cubeId}&sort=${sort}&limit=100`, 3600),
  ]);

  const colorStats = colors ?? [];
  const cardStats = cards ?? [];
  const meta = overview?.meta;
  const hasDecks = (meta?.total_decks ?? 0) > 0;

  return (
    <main className="container">
      <div
        style={{
          display: "flex",
          alignItems: "baseline",
          justifyContent: "space-between",
          gap: "1rem",
          flexWrap: "wrap",
        }}
      >
        <div>
          <h1 style={{ marginBottom: "0.25rem" }}>{view.cube.name}</h1>
          <p className="muted" style={{ margin: 0 }}>
            <Link href={`/cubes/${cubeId}`}>{view.card_count} cards in pool</Link>
          </p>
        </div>
        <CubeSwitcher cubes={cubes.map((c) => ({ id: c.cube.id, name: c.cube.name }))} current={cubeId} />
      </div>

      {!hasDecks ? (
        <div className="card" style={{ marginTop: "1.5rem" }}>
          <p style={{ margin: 0 }}>
            No decklists analyzed yet for this cube.{" "}
            <Link href="/decks/new">Upload the first deck</Link> to populate the meta.
          </p>
        </div>
      ) : (
        <>
          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(140px, 1fr))",
            }}
          >
            <StatTile value={String(meta!.total_games)} label="Games played" />
            <StatTile value={String(meta!.total_decks)} label="Decks recorded" />
            <StatTile value={num(meta!.avg_cmc)} label="Avg. mana value" />
          </div>

          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
            }}
          >
            <section className="card">
              <h2>Color usage</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Decks playing each color, with its winrate. Multicolor decks count on
                every color they play.
              </p>
              <RadarChart
                axes={colorAxes(colorStats)}
                caption="Decks playing each color of the WUBRG pie"
              />
            </section>

            <section className="card">
              <h2>Deck color count</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                How many colors decks commit to, with each bracket&apos;s winrate.
              </p>
              <RadarChart
                axes={colorCountAxes(colorStats)}
                caption="Decks by number of colors played, one through five"
              />
            </section>
          </div>

          <section className="card" style={{ marginTop: "1.5rem" }}>
            <div
              style={{
                display: "flex",
                justifyContent: "space-between",
                alignItems: "center",
                marginBottom: "0.5rem",
                gap: "1rem",
                flexWrap: "wrap",
              }}
            >
              <div>
                <h2 style={{ margin: 0 }}>Cards</h2>
                <p className="muted" style={{ margin: 0, fontSize: "0.85rem" }}>
                  Basic lands excluded — every deck plays them.
                </p>
              </div>
              <div style={{ display: "flex", gap: 6 }}>
                {SORTS.map((s) => (
                  <Link
                    key={s.key}
                    href={`/analytics/${cubeId}?sort=${s.key}`}
                    scroll={false}
                    className="pill"
                    style={{
                      background: sort === s.key ? "var(--accent-weak)" : "transparent",
                      color: sort === s.key ? "var(--text)" : "var(--text-secondary)",
                      textDecoration: "none",
                    }}
                  >
                    {s.label}
                  </Link>
                ))}
              </div>
            </div>
            <div style={{ overflowX: "auto" }}>
              <table>
                <thead>
                  <tr>
                    <th>Card</th>
                    <th className="num">Decks</th>
                    <th className="num">Incl.</th>
                    <th className="num">WR</th>
                    <th className="num">Lift</th>
                    <th className="num">Wilson</th>
                  </tr>
                </thead>
                <tbody>
                  {cardStats.map((c) => (
                    <tr key={c.card_id}>
                      <td>
                        <Link
                          href={`/cards/${c.slug}?cube=${cubeId}`}
                          style={{
                            display: "inline-flex",
                            alignItems: "center",
                            gap: 6,
                            color: "var(--text)",
                          }}
                        >
                          <ColorPips bits={c.color_identity} />
                          {c.name}
                        </Link>
                      </td>
                      <td className="num">{c.deck_count}</td>
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
                  {cardStats.length === 0 && (
                    <tr>
                      <td colSpan={6} className="muted">
                        No cards in analyzed decks.
                      </td>
                    </tr>
                  )}
                </tbody>
              </table>
            </div>
          </section>
        </>
      )}
    </main>
  );
}
