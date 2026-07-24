import Link from "next/link";
import { apiGetOptional, type DecklistListItem } from "@/lib/api";
import { DeckTable } from "@/components/DeckTable";

// Rendered per request. As a static page this was prerendered during `next build`,
// where the backend does not exist, so it shipped an empty deck list and served it
// for a full revalidate window.
export const dynamic = "force-dynamic";

// ?q, ?sort and ?dir are read here rather than through useSearchParams so the table
// renders filtered on the server too — this is the page other pages link into when a
// stat wants to show its working (see the analytics tiles), and the list those links
// land on should be the filtered one on first paint.
export default async function DecklistsPage({
  searchParams,
}: {
  searchParams: { q?: string; sort?: string; dir?: string };
}) {
  const decks = (await apiGetOptional<DecklistListItem[]>("/decklists", 0)) ?? [];

  return (
    <main className="container">
      <DeckTable
        decks={decks}
        showArchetype
        syncUrl
        initialQuery={searchParams.q ?? ""}
        initialSort={{ key: searchParams.sort, dir: searchParams.dir }}
        heading={<h1>Decklists</h1>}
        actions={
          <Link href="/decks/new" className="button">
            New deck
          </Link>
        }
      />
    </main>
  );
}
