import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type CubeView, type CubeCard } from "@/lib/api";
import { CardFan } from "@/components/CardFan";
import { groupCubeCards } from "@/lib/colors";

export const revalidate = 300;

function fmtDate(s?: string): string {
  if (!s) return "never";
  const d = new Date(s);
  return isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

const MOXFIELD_URL = (publicId: string) => `https://www.moxfield.com/decks/${publicId}`;

export default async function CubeDetailPage({ params }: { params: { id: string } }) {
  const view = await apiGetOptional<CubeView>(`/cubes/${params.id}`, 300);
  if (!view) notFound();

  const cards = (await apiGetOptional<CubeCard[]>(`/cubes/${params.id}/cards`, 300)) ?? [];
  const groups = groupCubeCards(cards);
  const { cube } = view;

  return (
    <main className="container">
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href="/cubes">← Cubes</Link>
      </p>
      <h1 style={{ marginBottom: "0.25rem" }}>{cube.name}</h1>
      <p className="muted">
        {view.card_count} cards · synced {fmtDate(cube.last_synced_at)}
        {cube.moxfield_public_id && (
          <>
            {" · "}
            <a href={MOXFIELD_URL(cube.moxfield_public_id)} target="_blank" rel="noreferrer">
              Moxfield ↗
            </a>
          </>
        )}
      </p>
      {cube.description && <p>{cube.description}</p>}

      {cards.length === 0 ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          This cube has no cards yet.
        </p>
      ) : (
        <div style={{ marginTop: "1.5rem", display: "flex", flexDirection: "column", gap: "2.5rem" }}>
          {groups.map((g) => (
            <section key={g.key}>
              <h2 style={{ textTransform: "uppercase", letterSpacing: "0.03em", fontSize: "1rem" }}>
                {g.label}{" "}
                <span className="muted" style={{ fontWeight: 400 }}>
                  · {g.cards.length}
                </span>
              </h2>
              <div
                style={{
                  display: "grid",
                  gridTemplateColumns: "auto minmax(0, 1fr)",
                  gap: "2rem",
                  alignItems: "start",
                }}
              >
                <CardFan cards={g.cards} />
                <div style={{ columnCount: 2, columnGap: "1.5rem" }}>
                  {g.cards.map((c) => (
                    <div key={c.card_id} style={{ breakInside: "avoid" }}>
                      {c.card_name}
                    </div>
                  ))}
                </div>
              </div>
            </section>
          ))}
        </div>
      )}
    </main>
  );
}
