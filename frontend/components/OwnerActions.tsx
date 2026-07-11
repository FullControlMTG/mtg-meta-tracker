"use client";

import Link from "next/link";
import { useSession } from "@/components/SessionProvider";

// Renders edit affordances on the (server-rendered) deck detail page for whoever
// may mutate the deck. Gates client-side on owner-or-admin, mirroring the
// backend's CanMutateOwned.
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

  if (!me || (me.id !== ownerId && me.role !== "admin")) return null;

  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", margin: "0.5rem 0 0.25rem" }}>
      <Link href={`/decks/${deckId}/edit`} className="button">
        Edit deck
      </Link>
      {gamesPlayed === 0 && (
        <Link href={`/decks/${deckId}/edit#record`} className="muted">
          + Add a win/loss record
        </Link>
      )}
    </div>
  );
}
