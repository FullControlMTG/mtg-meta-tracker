import Link from "next/link";
import { apiGetOptional, type DecklistListItem } from "@/lib/api";
import { DeckTable } from "@/components/DeckTable";

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
          <DeckTable decks={decks} showArchetype />
        </div>
      )}
    </main>
  );
}
