import { apiGetOptional, type Combo, type CubeCard } from "@/lib/api";
import { ComboList } from "@/components/ComboList";

export const revalidate = 300;

// The cube's configured combos, with their pieces. A piece no longer in the pool is
// flagged (see ComboList) — a combo can outlive a card's removal from the list.
export default async function CubeCombosPage({ params }: { params: { id: string } }) {
  const [combos, cards] = await Promise.all([
    apiGetOptional<Combo[]>(`/cubes/${params.id}/combos`, 300),
    apiGetOptional<CubeCard[]>(`/cubes/${params.id}/cards`, 300),
  ]);

  const pool = new Set((cards ?? []).map((c) => c.card_id));
  const missing = new Set<string>();
  for (const combo of combos ?? []) {
    for (const piece of combo.cards) {
      if (!pool.has(piece.card_id)) missing.add(piece.card_id);
    }
  }

  if (!combos || combos.length === 0) {
    return (
      <p className="muted" style={{ marginTop: "1rem" }}>
        No combos configured for this cube yet. An owner can add them from Manage cube.
      </p>
    );
  }

  return (
    <div style={{ marginTop: "0.5rem" }}>
      {missing.size > 0 && (
        <p className="muted" style={{ fontSize: "0.85rem" }}>
          Pieces marked <span style={{ color: "var(--bad, #b00)", fontWeight: 600 }}>Not in cube</span>{" "}
          are no longer in the pool.
        </p>
      )}
      <ComboList combos={combos} cubeId={params.id} missingCardIds={missing} />
    </div>
  );
}
