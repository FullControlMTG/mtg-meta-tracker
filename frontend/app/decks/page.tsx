import Link from "next/link";
import { apiGetOptional, type DecklistListItem } from "@/lib/api";
import { ColorPips } from "@/components/ColorPips";
import { pct } from "@/lib/format";

// Rendered per request. As a static page this was prerendered during `next build`,
// where the backend does not exist, so it shipped an empty deck list and served it
// for a full revalidate window.
export const dynamic = "force-dynamic";

export default async function DecklistsPage() {
  const decks = (await apiGetOptional<DecklistListItem[]>("/decklists", 0)) ?? [];

  return (
    <main className="container">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h1>Decklists</h1>
        <Link href="/decks/new" className="button">
          New deck
        </Link>
      </div>

      {decks.length === 0 ? (
        <p className="muted">No decklists yet.</p>
      ) : (
        <div className="card" style={{ marginTop: "1rem", overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Deck</th>
                <th>Colors</th>
                <th className="num">Cards</th>
                <th className="num">Record</th>
                <th className="num">Winrate</th>
                <th>Status</th>
              </tr>
            </thead>
            <tbody>
              {decks.map(({ decklist: d }) => (
                <tr key={d.id}>
                  <td>
                    <Link href={`/decks/${d.id}`}>{d.name}</Link>
                    {d.archetype && (
                      <span className="muted" style={{ marginLeft: 6, fontSize: "0.85rem" }}>
                        {d.archetype}
                      </span>
                    )}
                  </td>
                  <td>
                    <ColorPips bits={d.color_identity} showCode />
                  </td>
                  <td className="num">{d.card_count}</td>
                  <td className="num">
                    {d.games_played > 0 ? `${d.wins}-${d.losses}` : "—"}
                  </td>
                  <td className="num">{pct(d.winrate)}</td>
                  <td>
                    <span className="pill">{d.status}</span>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
