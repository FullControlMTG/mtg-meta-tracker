import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type DecklistCard, type DecklistDetail } from "@/lib/api";
import { ColorPips } from "@/components/ColorPips";
import { CardBrowser } from "@/components/CardBrowser";
import { ComboList } from "@/components/ComboList";
import { StatTile } from "@/components/StatTile";
import { OwnerActions } from "@/components/OwnerActions";
import { sortCards } from "@/lib/colors";
import { fmtDate, pct } from "@/lib/format";

export const revalidate = 3600;

// A deck's fans live inside the 1040px .container, so cards land at ~248px — larger
// and more readable than the cube's full-bleed 200px. The cap keeps it that way.
const DECK_MAX_COLS = 5;

const BOARDS: { key: string; label: string }[] = [
  { key: "main", label: "Mainboard" },
  { key: "side", label: "Sideboard" },
  { key: "maybe", label: "Maybeboard" },
];

export default async function DecklistDetailPage({ params }: { params: { id: string } }) {
  const detail = await apiGetOptional<DecklistDetail>(`/decklists/${params.id}`, 3600);
  if (!detail) notFound();

  const { decklist: d, cards, user } = detail;
  // Matched fresh on every read against the cube's configured combos, so this
  // list reflects the current definitions rather than what they said when the
  // deck was uploaded. Absent on a response from a backend older than combos.
  const combos = detail.combos ?? [];
  // Each board reads in the cube's display order (color → cmc → name) rather than
  // the backend's flat alphabetical one.
  const sections = BOARDS.map((b) => ({
    ...b,
    cards: sortCards(cards.filter((c) => c.board === b.key)),
  })).filter((s) => s.cards.length > 0);
  // CardFan drops unresolved cards, so with the text list gone this warning is the
  // only trace of a card that is in the deck but not on the page. Name the names.
  const unresolved = cards.filter((c) => !c.is_resolved);

  return (
    <main className="container">
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href="/decks">← Decks</Link>
      </p>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <h1 style={{ margin: 0 }}>{d.name}</h1>
        <ColorPips bits={d.color_identity} splash={d.splash_colors} showCode />
      </div>
      <p className="muted">
        {user && (
          <>
            by <Link href={`/users/${user.username}`}>{user.display_name}</Link> ·{" "}
          </>
        )}
        {fmtDate(d.played_at)} · {d.card_count} cards
        {d.archetype && <> · {d.archetype}</>}
      </p>
      {d.description && <p>{d.description}</p>}

      <OwnerActions deckId={d.id} ownerId={d.user_id} gamesPlayed={d.games_played} />

      {d.games_played > 0 && (
        <div
          className="grid"
          style={{ gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))", margin: "1rem 0" }}
        >
          <StatTile value={`${d.wins}-${d.losses}`} label="Record (W-L)" />
          <StatTile value={pct(d.winrate)} label="Winrate" />
          <StatTile value={String(d.games_played)} label="Games" />
        </div>
      )}

      {sections.length === 0 ? (
        <p className="muted" style={{ marginTop: "1rem" }}>
          This deck has no cards yet.
        </p>
      ) : (
        <div style={{ marginTop: "1.5rem" }}>
          {/* No search here: a deck is forty cards laid out on one screen, and the
              header already counts them. The cube pool is the page that needs it. */}
          <CardBrowser sections={sections} maxCols={DECK_MAX_COLS} countQuantity searchable={false} />
        </div>
      )}

      <ComboList combos={combos} />

      {unresolved.length > 0 && (
        <p className="muted" style={{ marginTop: "1.5rem", fontSize: "0.85rem" }}>
          Not shown — {unresolved.length} card{unresolved.length > 1 ? "s" : ""} could not be matched
          to the card database: {unresolved.map((c) => c.card_name).join(", ")}
        </p>
      )}

      {d.source_url && (
        <p style={{ marginTop: "1rem" }}>
          <a href={d.source_url} target="_blank" rel="noreferrer">
            Source ↗
          </a>
        </p>
      )}
    </main>
  );
}
