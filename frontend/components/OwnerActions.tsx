"use client";

import Link from "next/link";
import { useSession } from "@/components/SessionProvider";

// Renders edit affordances on the (server-rendered) deck detail page for whoever
// may mutate the deck. Gates client-side on owner-or-admin, mirroring the
// backend's CanMutateOwned.
export function OwnerActions({
  cubeId,
  deckId,
  ownerId,
  gamesPlayed,
}: {
  cubeId: string;
  deckId: string;
  ownerId: string;
  gamesPlayed: number;
}) {
  const { me } = useSession();

  if (!me || (me.id !== ownerId && me.role !== "admin")) return null;

  const edit = `/cube/${cubeId}/decks/${deckId}/edit`;
  return (
    <div style={{ display: "flex", alignItems: "center", gap: "1rem", margin: "0.5rem 0 0.25rem" }}>
      <Link href={edit} className="button">
        Edit deck
      </Link>
      {gamesPlayed === 0 && (
        <Link href={`${edit}#record`} className="muted">
          + Add a win/loss record
        </Link>
      )}
    </div>
  );
}
