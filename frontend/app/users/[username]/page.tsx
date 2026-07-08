import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type PublicUser, type DecklistListItem } from "@/lib/api";
import { ColorPips } from "@/components/ColorPips";
import { pct } from "@/lib/format";

export const revalidate = 3600;

export default async function UserPage({ params }: { params: { username: string } }) {
  const user = await apiGetOptional<PublicUser>(`/users/${params.username}`, 3600);
  if (!user) notFound();

  const decks =
    (await apiGetOptional<DecklistListItem[]>(`/decklists?user=${user.id}`, 3600)) ?? [];

  return (
    <main className="container">
      <h1>{user.display_name}</h1>
      <p className="muted">
        @{user.username}
        {user.role === "admin" && <span className="pill" style={{ marginLeft: 8 }}>admin</span>}
      </p>
      {user.bio && <p>{user.bio}</p>}

      <h2 style={{ marginTop: "1.5rem" }}>Decklists</h2>
      {decks.length === 0 ? (
        <p className="muted">No decklists yet.</p>
      ) : (
        <div className="card" style={{ overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Deck</th>
                <th>Colors</th>
                <th className="num">Record</th>
                <th className="num">Winrate</th>
              </tr>
            </thead>
            <tbody>
              {decks.map(({ decklist: d }) => (
                <tr key={d.id}>
                  <td>
                    <Link href={`/decklists/${d.id}`}>{d.name}</Link>
                  </td>
                  <td>
                    <ColorPips bits={d.color_identity} showCode />
                  </td>
                  <td className="num">
                    {d.games_played > 0 ? `${d.wins}-${d.losses}-${d.draws}` : "—"}
                  </td>
                  <td className="num">{pct(d.winrate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </main>
  );
}
