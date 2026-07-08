import Link from "next/link";
import { apiGetOptional, type Overview } from "@/lib/api";
import { getDefaultCube } from "@/lib/cube";
import { StatTile } from "@/components/StatTile";
import { pct, num } from "@/lib/format";

// Re-rendered on demand by the backend revalidation webhook; hourly fallback.
export const revalidate = 3600;

export default async function Home() {
  const cube = await getDefaultCube();
  const overview = cube
    ? await apiGetOptional<Overview>(`/analytics/overview?cube=${cube.cube.id}`, 3600)
    : null;

  return (
    <main className="container">
      <h1>Meta Overview</h1>
      {cube ? (
        <p className="muted">
          {cube.cube.name} · {cube.card_count} cards in pool
        </p>
      ) : (
        <p className="muted">No cube configured yet. An admin can add one.</p>
      )}

      {!overview || overview.meta.total_decks === 0 ? (
        <div className="card" style={{ marginTop: "1rem" }}>
          <p style={{ margin: 0 }}>
            No decklists analyzed yet.{" "}
            <Link href="/decks/new">Upload the first deck</Link> to populate the meta.
          </p>
        </div>
      ) : (
        <>
          <div
            className="grid"
            style={{
              marginTop: "1rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(150px, 1fr))",
            }}
          >
            <StatTile value={String(overview.meta.total_decks)} label="Decks" />
            <StatTile value={String(overview.meta.total_games)} label="Games logged" />
            <StatTile value={pct(overview.meta.overall_winrate)} label="Overall winrate" />
            <StatTile value={num(overview.meta.avg_cmc)} label="Avg. mana value" />
            <StatTile value={num(overview.meta.avg_color_count, 1)} label="Avg. colors" />
            <StatTile value={pct(overview.meta.mono_share, 0)} label="Mono-color" />
          </div>
          <p className="muted" style={{ marginTop: "1rem" }}>
            <Link href="/analytics">Explore the full analytics dashboard →</Link>
          </p>
        </>
      )}
    </main>
  );
}
