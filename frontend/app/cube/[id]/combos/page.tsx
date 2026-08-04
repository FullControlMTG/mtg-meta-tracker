import { apiGetOptional, type Combo, type CubeCard } from "@/lib/api";
import { CombosBrowser } from "@/components/CombosBrowser";

export const revalidate = 300;

// The cube's configured combos, filterable by card, with pieces no longer in the pool
// marked (see ComboList) — a combo can outlive a card's removal from the list.
export default async function CubeCombosPage({ params }: { params: { id: string } }) {
  const [combos, cards] = await Promise.all([
    apiGetOptional<Combo[]>(`/cubes/${params.id}/combos`, 300),
    apiGetOptional<CubeCard[]>(`/cubes/${params.id}/cards`, 300),
  ]);

  if (!combos || combos.length === 0) {
    return (
      <p className="muted" style={{ marginTop: "1rem" }}>
        No combos configured for this cube yet. An owner can add them from Manage cube.
      </p>
    );
  }

  return <CombosBrowser combos={combos} poolCardIds={(cards ?? []).map((c) => c.card_id)} />;
}
