import Link from "next/link";
import { apiGetOptional, type DecklistListItem } from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { DeckTable } from "@/components/DeckTable";

export const dynamic = "force-dynamic";

// ?q/?sort/?dir are read server-side so the table renders filtered on first paint —
// this is the list the analytics tiles link into. Decks are scoped to this cube.
export default async function CubeDecksPage({
  params,
  searchParams,
}: {
  params: { id: string };
  searchParams: { q?: string; sort?: string; dir?: string };
}) {
  const decks =
    (await apiGetOptional<DecklistListItem[]>(`/decklists?cube=${params.id}`, 0)) ?? [];

  return (
    <div style={{ marginTop: "1rem" }}>
      <DeckTable
        decks={decks}
        cubeId={params.id}
        showArchetype
        showOwner
        syncUrl
        initialQuery={searchParams.q ?? ""}
        initialSort={{ key: searchParams.sort, dir: searchParams.dir }}
        heading={<h2 style={{ margin: 0 }}>Decklists</h2>}
        actions={
          <Link href={cubePath(params.id, "/decks/new")} className="button">
            New deck
          </Link>
        }
      />
    </div>
  );
}
