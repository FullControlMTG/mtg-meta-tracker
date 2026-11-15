import { apiGetOptional, type CubeView, type CubeCard } from "@/lib/api";
import { CardBrowser } from "@/components/CardBrowser";
import { groupCubeCards } from "@/lib/colors";

export const revalidate = 300;

function fmtDate(s?: string): string {
  if (!s) return "never";
  const d = new Date(s);
  return isNaN(d.getTime()) ? "—" : d.toLocaleDateString();
}

const MOXFIELD_URL = (publicId: string) => `https://www.moxfield.com/decks/${publicId}`;

export default async function CubeCardsPage({ params }: { params: { id: string } }) {
  const [view, cardsRaw] = await Promise.all([
    apiGetOptional<CubeView>(`/cubes/${params.id}`, 300),
    apiGetOptional<CubeCard[]>(`/cubes/${params.id}/cards`, 300),
  ]);
  const cards = cardsRaw ?? [];
  // Order within each section is CardBrowser's, from its sort control.
  const groups = groupCubeCards(cards);

  return (
    <div style={{ marginTop: "1rem" }}>
      {view && (
        <p className="muted">
          {view.card_count} cards
          {view.unique_count !== view.card_count && <> ({view.unique_count} unique)</>} · synced{" "}
          {fmtDate(view.cube.last_synced_at)}
          {view.cube.moxfield_public_id && (
            <>
              {" · "}
              <a href={MOXFIELD_URL(view.cube.moxfield_public_id)} target="_blank" rel="noreferrer">
                Moxfield ↗
              </a>
            </>
          )}
        </p>
      )}
      {view?.cube.description && <p>{view.cube.description}</p>}

      {cards.length === 0 ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          This cube has no cards yet.
        </p>
      ) : (
        <div style={{ marginTop: "1rem" }}>
          <CardBrowser sections={groups} colorSections countQuantity placeholder="Search the cube…" />
        </div>
      )}
    </div>
  );
}
