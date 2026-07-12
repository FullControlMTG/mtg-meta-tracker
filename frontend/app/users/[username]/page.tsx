import { notFound } from "next/navigation";
import { apiGetOptional, type PublicUser, type DecklistListItem } from "@/lib/api";
import { DeckTable } from "@/components/DeckTable";

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
          <DeckTable decks={decks} />
        </div>
      )}
    </main>
  );
}
