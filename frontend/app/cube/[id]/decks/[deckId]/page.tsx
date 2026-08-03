import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type DecklistDetail } from "@/lib/api";
import { cubePath } from "@/lib/cube";
import { ColorPips } from "@/components/ColorPips";
import { CardBrowser } from "@/components/CardBrowser";
import { ComboList } from "@/components/ComboList";
import { StatTile } from "@/components/StatTile";
import { OwnerActions } from "@/components/OwnerActions";
import { sortCards } from "@/lib/colors";
import { fmtDate, pct } from "@/lib/format";

export const revalidate = 3600;

const DECK_MAX_COLS = 5;

const BOARDS: { key: string; label: string }[] = [
  { key: "main", label: "Mainboard" },
  { key: "side", label: "Sideboard" },
  { key: "maybe", label: "Maybeboard" },
];

export default async function DecklistDetailPage({
  params,
}: {
  params: { id: string; deckId: string };
}) {
  const cubeId = params.id;
  const detail = await apiGetOptional<DecklistDetail>(`/decklists/${params.deckId}`, 3600);
  if (!detail) notFound();

  const { decklist: d, cards, user } = detail;
  const combos = detail.combos ?? [];
  const sections = BOARDS.map((b) => ({
    ...b,
    cards: sortCards(cards.filter((c) => c.board === b.key)),
  })).filter((s) => s.cards.length > 0);
  const unresolved = cards.filter((c) => !c.is_resolved);

  return (
    <div style={{ marginTop: "1rem" }}>
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href={cubePath(cubeId, "/decks")}>← Decks</Link>
      </p>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <h1 style={{ margin: 0 }}>{d.name}</h1>
        <ColorPips bits={d.color_identity} splash={d.splash_colors} showCode />
      </div>
      <p className="muted">
        {user && (
          <>
            by <Link href={`/users/${user.username}?cube=${cubeId}`}>{user.display_name}</Link> ·{" "}
          </>
        )}
        {fmtDate(d.played_at)} · {d.card_count} cards
        {d.archetype && <> · {d.archetype}</>}
      </p>
      {d.description && <p>{d.description}</p>}

      <OwnerActions cubeId={cubeId} deckId={d.id} ownerId={d.user_id} gamesPlayed={d.games_played} />

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
          <CardBrowser sections={sections} maxCols={DECK_MAX_COLS} countQuantity searchable={false} />
        </div>
      )}

      <ComboList combos={combos} cubeId={cubeId} />

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
    </div>
  );
}
