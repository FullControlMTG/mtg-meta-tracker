import Link from "next/link";
import { notFound } from "next/navigation";
import {
  apiGetOptional,
  type CardStat,
  type ColorStat,
  type ColorTrendPoint,
  type CubeView,
  type Overview,
} from "@/lib/api";
import { getCubes } from "@/lib/cube";
import { COLORS } from "@/lib/colors";
import { CardStatsTable } from "@/components/CardStatsTable";
import { ColorTrendChart } from "@/components/ColorTrendChart";
import { CubeSwitcher } from "@/components/CubeSwitcher";
import { UNDEFEATED_TERMS, deckListHref, quoteTerm } from "@/lib/deckQuery";
import { RadarChart, type RadarAxis } from "@/components/RadarChart";
import { StatTile } from "@/components/StatTile";
import { num, pct } from "@/lib/format";

// The cube's stats page: overview counters and the analytics breakdown, merged.
// Re-rendered on demand by the backend revalidation webhook; hourly fallback.
export const revalidate = 3600;

// Share of the cube's decks landing on an axis. Null with no decks, so pct()
// renders an em dash rather than NaN.
const share = (count: number, total: number) => (total > 0 ? count / total : null);

// The single_color facet, as one radar axis per WUBRG color: how often each color
// is played. A 2-color deck counts on both of its axes, so the shares sum past 100%.
function colorAxes(stats: ColorStat[], totalDecks: number): RadarAxis[] {
  const byKey = new Map(stats.filter((s) => s.facet === "single_color").map((s) => [s.facet_key, s]));
  return COLORS.map((c) => {
    const decks = byKey.get(c.bit)?.deck_count ?? 0;
    const s = share(decks, totalDecks);
    return {
      key: c.code,
      label: c.name,
      value: decks,
      hex: c.hex,
      share: s,
      note: `${pct(s, 0)} of decks play ${c.name}`,
    };
  });
}

// The splash_color facet: how often each color is splashed rather than played —
// on fewer than 10% of a deck's nonlands. A splash is not one of the deck's colors,
// so these decks are absent from that color's axis on the breakdown above.
function splashAxes(stats: ColorStat[], totalDecks: number): RadarAxis[] {
  const byKey = new Map(stats.filter((s) => s.facet === "splash_color").map((s) => [s.facet_key, s]));
  return COLORS.map((c) => {
    const decks = byKey.get(c.bit)?.deck_count ?? 0;
    const s = share(decks, totalDecks);
    return {
      key: c.code,
      label: c.name,
      value: decks,
      hex: c.hex,
      share: s,
      note: `${pct(s, 0)} of decks splash ${c.name}`,
    };
  });
}

// The color_count facet: how many decks play 1, 2, 3, 4, or 5 colors.
function colorCountAxes(stats: ColorStat[], totalDecks: number): RadarAxis[] {
  const byKey = new Map(stats.filter((s) => s.facet === "color_count").map((s) => [s.facet_key, s]));
  return [1, 2, 3, 4, 5].map((n) => {
    const decks = byKey.get(n)?.deck_count ?? 0;
    const s = share(decks, totalDecks);
    return {
      key: String(n),
      label: n === 1 ? "Mono" : `${n} colors`,
      value: decks,
      share: s,
      note: `${pct(s, 0)} of meta plays ${n === 1 ? "one color" : `${n} colors`}`,
    };
  });
}

export default async function CubeStatsPage({ params }: { params: { cube: string } }) {
  const cubeId = params.cube;

  const [view, cubes] = await Promise.all([
    apiGetOptional<CubeView>(`/cubes/${cubeId}`, 300),
    getCubes(),
  ]);
  if (!view) notFound();

  const [overview, colors, trend, cards] = await Promise.all([
    apiGetOptional<Overview>(`/analytics/overview?cube=${cubeId}`, 3600),
    apiGetOptional<ColorStat[]>(`/analytics/colors?cube=${cubeId}`, 3600),
    apiGetOptional<ColorTrendPoint[]>(`/analytics/color-trend?cube=${cubeId}`, 3600),
    apiGetOptional<CardStat[]>(`/analytics/cards?cube=${cubeId}&limit=100`, 3600),
  ]);

  const colorStats = colors ?? [];
  const trendPoints = trend ?? [];
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
              gap: "0.6rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(125px, 1fr))",
            }}
          >
            <StatTile value={String(meta!.total_games)} label="Matches played" />
            <StatTile value={String(meta!.total_decks)} label="Decks recorded" />
            <StatTile value={num(meta!.avg_cmc)} label="Avg. mana value" />
            <StatTile value={pct(meta!.power9_share, 0)} label="Decks playing Power 9" />
            {/* The one tile that counts decks you can go and read. Same query the
                number was computed from, scoped to this cube, best record first. */}
            <StatTile
              value={String(meta!.undefeated_decks)}
              label="Undefeated decks"
              href={deckListHref(
                [`cube:${quoteTerm(view.cube.name)}`, ...UNDEFEATED_TERMS],
                { key: "record", dir: "desc" },
              )}
            />
          </div>

          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
            }}
          >
            <section className="card">
              <h2>Color Breakdown</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Percentage of decks playing each color. Does not include splash colors.
              </p>
              <RadarChart
                axes={colorAxes(colorStats, meta!.total_decks)}
                caption="Deck color distribution"
              />
            </section>

            <section className="card">
              <h2>Deck Colors</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                How many colors decks commit to one or more colors.
              </p>
              <RadarChart
                axes={colorCountAxes(colorStats, meta!.total_decks)}
                caption="Deck color choices distribution"
              />
            </section>

            <section className="card">
              <h2>Splashed Colors</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Percentage of decks splashing each color (less than 10% representation).
              </p>
              <RadarChart
                axes={splashAxes(colorStats, meta!.total_decks)}
                caption="Decks splashing each color of the WUBRG pie"
              />
            </section>
          </div>

          <section className="card" style={{ marginTop: "1.5rem" }}>
            <h2>Color Share Over Time</h2>
            <p className="muted" style={{ marginTop: "-0.25rem" }}>
              Stacked percentages of each color's share of the meta, tracked over time. 
              Splash colors are excluded.
            </p>
            <ColorTrendChart points={trendPoints} />
          </section>

          <section className="card" style={{ marginTop: "1.5rem" }}>
            <div style={{ marginBottom: "0.5rem" }}>
              <h2 style={{ margin: 0 }}>Cards</h2>
              <p className="muted" style={{ margin: 0, fontSize: "0.85rem" }}>
                Most played first. Basic lands excluded — every deck plays them.
              </p>
            </div>
            <CardStatsTable cards={cardStats} cubeId={cubeId} />
          </section>
        </>
      )}
    </main>
  );
}
