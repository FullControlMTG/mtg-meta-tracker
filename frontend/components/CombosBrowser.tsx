"use client";

import { useMemo, useState } from "react";
import type { Combo } from "@/lib/api";
import { ComboList } from "@/components/ComboList";

// The cube's combos with a card-name filter. A combo matches when any of its pieces'
// names contains the query. Absence from the pool (poolCardIds) is computed here and
// handed to ComboList, so the missing-card marking survives the search.
export function CombosBrowser({ combos, poolCardIds }: { combos: Combo[]; poolCardIds: string[] }) {
  const [query, setQuery] = useState("");

  const missing = useMemo(() => {
    const pool = new Set(poolCardIds);
    const s = new Set<string>();
    for (const combo of combos) {
      for (const piece of combo.cards) if (!pool.has(piece.card_id)) s.add(piece.card_id);
    }
    return s;
  }, [combos, poolCardIds]);

  const q = query.trim().toLowerCase();
  const filtered = q
    ? combos.filter((c) => c.cards.some((p) => p.card_name.toLowerCase().includes(q)))
    : combos;

  return (
    <div style={{ marginTop: "0.5rem" }}>
      <input
        className="search"
        type="search"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Filter combos by card…"
        aria-label="Filter combos by card name"
      />
      {missing.size > 0 && (
        <p className="muted" style={{ fontSize: "0.85rem", marginTop: "0.5rem" }}>
          Cards outlined in red are not active in the cube.
        </p>
      )}
      {filtered.length === 0 ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          No combos include a card matching “{query.trim()}”.
        </p>
      ) : (
        <ComboList combos={filtered} missingCardIds={missing} />
      )}
    </div>
  );
}
