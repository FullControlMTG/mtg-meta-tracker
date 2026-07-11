import Image from "next/image";
import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type CardDetail } from "@/lib/api";
import { getDefaultCube } from "@/lib/cube";
import { ColorPips } from "@/components/ColorPips";
import { ColorWinrateChart } from "@/components/ColorWinrateChart";
import { RadarChart, type RadarAxis } from "@/components/RadarChart";
import { StatTile } from "@/components/StatTile";
import { num, pct, signedPct } from "@/lib/format";

export const revalidate = 300;

const CARD_W = 300;
const CARD_H = Math.round(CARD_W * (88 / 63)); // MTG card aspect ratio

function colorCountAxes(stats: CardDetail["color_count_split"]): RadarAxis[] {
  const byKey = new Map(stats.map((s) => [s.facet_key, s]));
  return [1, 2, 3, 4, 5].map((n) => ({
    key: String(n),
    label: n === 1 ? "Mono" : `${n} colors`,
    value: byKey.get(n)?.deck_count ?? 0,
  }));
}

export default async function CardPage({
  params,
  searchParams,
}: {
  params: { slug: string };
  searchParams: { cube?: string };
}) {
  // Card stats only mean anything within a cube, so scope to one — the requested
  // cube, or the default.
  const cubeId = searchParams.cube ?? (await getDefaultCube())?.cube.id;
  if (!cubeId) notFound();

  const detail = await apiGetOptional<CardDetail>(
    `/cards/${params.slug}?cube=${cubeId}`,
    300
  );
  if (!detail) notFound();

  const { card, stat, decks, pairs, in_pool } = detail;
  const analytics = `/analytics/${cubeId}`;

  return (
    <main className="container">
      <p className="muted" style={{ marginBottom: "0.75rem" }}>
        <Link href={analytics}>← Cube stats</Link>
      </p>

      <div style={{ display: "flex", gap: "1.5rem", flexWrap: "wrap", alignItems: "flex-start" }}>
        <Image
          src={`/api/cards/${card.card_id}/image`}
          alt={card.name}
          width={CARD_W}
          height={CARD_H}
          className="fan-card"
          unoptimized
          style={{ flexShrink: 0 }}
        />
        <div style={{ flex: "1 1 320px", minWidth: 280 }}>
          <div style={{ display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
            <h1 style={{ margin: 0 }}>{card.name}</h1>
            <ColorPips bits={card.color_identity} showCode />
          </div>
          <p className="muted" style={{ marginTop: "0.25rem" }}>
            {card.type_line}
            {card.mana_cost && <> · {card.mana_cost}</>}
            {card.cmc != null && <> · MV {num(card.cmc, 0)}</>}
          </p>
          <p>
            <span
              className="pill"
              style={{
                background: in_pool ? "var(--accent-weak)" : "transparent",
                color: in_pool ? "var(--text)" : "var(--muted)",
              }}
            >
              {in_pool ? "In the cube pool" : "Not in the pool"}
            </span>
          </p>
          {card.oracle_text && (
            <p style={{ whiteSpace: "pre-line", fontSize: "0.9rem" }}>{card.oracle_text}</p>
          )}
        </div>
      </div>

      {!stat ? (
        <div className="card" style={{ marginTop: "1.5rem" }}>
          <p style={{ margin: 0 }}>
            {in_pool
              ? "This card is in the pool but has not appeared in any analyzed deck yet."
              : "This card has not appeared in any analyzed deck in this cube."}
          </p>
        </div>
      ) : (
        <>
          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(130px, 1fr))",
            }}
          >
            <StatTile value={pct(stat.inclusion_rate, 0)} label="Play rate" />
            <StatTile value={String(stat.deck_count)} label="Decks including" />
            <StatTile value={String(stat.games)} label="Games with" />
            <StatTile value={pct(stat.winrate)} label="Winrate" />
            <StatTile value={signedPct(stat.winrate_lift)} label="Winrate lift" />
            <StatTile value={pct(stat.wilson_lower)} label="Wilson lower" />
            {detail.rank_by_inclusion != null && (
              <StatTile
                value={`#${detail.rank_by_inclusion}`}
                label={`Popularity of ${detail.total_ranked}`}
              />
            )}
          </div>

          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
            }}
          >
            <section className="card">
              <h2>Typical deck colors</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Colors of the decks that play this card, and how those decks fare.
              </p>
              <ColorWinrateChart stats={detail.color_split} />
            </section>

            <section className="card">
              <h2>Color count of those decks</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                How many colors the decks playing this card commit to.
              </p>
              <RadarChart
                axes={colorCountAxes(detail.color_count_split)}
                caption={`Decks playing ${card.name}, by number of colors`}
              />
            </section>
          </div>

          <section className="card" style={{ marginTop: "1.5rem" }}>
            <h2>Top 10 played with</h2>
            <p className="muted" style={{ marginTop: "-0.25rem" }}>
              Cards that appear alongside this one more than chance would predict.
              Lift is how many times likelier than chance; confidence is the share of
              this card&apos;s decks that also play it.
            </p>
            {pairs.length === 0 ? (
              <p className="muted">No co-occurring cards yet (needs ≥2 shared decks).</p>
            ) : (
              <div style={{ overflowX: "auto" }}>
                <table>
                  <thead>
                    <tr>
                      <th>Card</th>
                      <th className="num">Lift</th>
                      <th className="num">Confidence</th>
                      <th className="num">Shared decks</th>
                    </tr>
                  </thead>
                  <tbody>
                    {pairs.map((p) => (
                      <tr key={p.card_b_id}>
                        <td>
                          <Link
                            href={`/cards/${p.slug}?cube=${cubeId}`}
                            style={{
                              display: "inline-flex",
                              alignItems: "center",
                              gap: 6,
                              color: "var(--text)",
                            }}
                          >
                            <ColorPips bits={p.color_identity} />
                            {p.name}
                          </Link>
                        </td>
                        <td className="num">{p.lift.toFixed(2)}×</td>
                        <td className="num">{pct(p.confidence_ab, 0)}</td>
                        <td className="num">{p.co_count}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </section>
        </>
      )}

      {decks.length > 0 && (
        <section className="card" style={{ marginTop: "1.5rem" }}>
          <h2>Decks playing this card</h2>
          <div style={{ overflowX: "auto" }}>
            <table>
              <thead>
                <tr>
                  <th>Deck</th>
                  <th>Colors</th>
                  <th className="num">Copies</th>
                  <th className="num">Record</th>
                  <th className="num">Winrate</th>
                </tr>
              </thead>
              <tbody>
                {decks.map((d) => (
                  <tr key={d.id}>
                    <td>
                      <Link href={`/decks/${d.id}`}>{d.name}</Link>
                      {d.owner && <span className="muted"> · {d.owner}</span>}
                    </td>
                    <td>
                      <ColorPips bits={d.color_identity} />
                    </td>
                    <td className="num">{d.quantity}</td>
                    <td className="num">
                      {d.wins}-{d.losses}-{d.draws}
                    </td>
                    <td className="num">{pct(d.winrate)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </main>
  );
}
