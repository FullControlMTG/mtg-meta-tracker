"use client";

import Link from "next/link";
import { useSession } from "@/components/SessionProvider";

// Renders owner-only edit affordances on the (server-rendered) deck detail page.
// Gates client-side by comparing the session user against the deck owner.
export function OwnerActions({
  deckId,
  ownerId,
  gamesPlayed,
}: {
  deckId: string;
  ownerId: string;
  gamesPlayed: number;
}) {
  const { me } = useSession();

  if (!me || me.id !== ownerId) return null;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", margin: "0.5rem 0 0.25rem" }}>
      <Link href={`/decklists/${deckId}/edit`} className="button">
        Edit deck
      </Link>
      {gamesPlayed === 0 && (
        <Link href={`/decklists/${deckId}/edit#record`} className="muted">
          + Add a win/loss record
        </Link>
      )}
    </div>
  );
}
