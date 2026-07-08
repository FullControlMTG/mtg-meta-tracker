import Link from "next/link";
import { notFound } from "next/navigation";
import { apiGetOptional, type DecklistDetail } from "@/lib/api";
import { ColorPips } from "@/components/ColorPips";
import { CardFan } from "@/components/CardFan";
import { StatTile } from "@/components/StatTile";
import { OwnerActions } from "@/components/OwnerActions";
import { pct } from "@/lib/format";

export const revalidate = 3600;

export default async function DecklistDetailPage({ params }: { params: { id: string } }) {
  const detail = await apiGetOptional<DecklistDetail>(`/decklists/${params.id}`, 3600);
  if (!detail) notFound();

  const { decklist: d, cards, user } = detail;
  const main = cards.filter((c) => c.board === "main");
  const unresolved = cards.filter((c) => !c.is_resolved);

  return (
    <main className="container">
      <p className="muted" style={{ marginBottom: "0.25rem" }}>
        <Link href="/decklists">← Decklists</Link>
      </p>
      <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
        <h1 style={{ margin: 0 }}>{d.name}</h1>
        <ColorPips bits={d.color_identity} showCode />
      </div>
      <p className="muted">
        {user && (
          <>
            by <Link href={`/users/${user.username}`}>{user.display_name}</Link> ·{" "}
          </>
        )}
        {d.card_count} cards
        {d.archetype && <> · {d.archetype}</>}
      </p>
      {d.description && <p>{d.description}</p>}

      <OwnerActions deckId={d.id} ownerId={d.user_id} gamesPlayed={d.games_played} />

      {d.games_played > 0 && (
        <div
          className="grid"
          style={{ gridTemplateColumns: "repeat(auto-fit, minmax(120px, 1fr))", margin: "1rem 0" }}
        >
          <StatTile value={`${d.wins}-${d.losses}-${d.draws}`} label="Record (W-L-D)" />
          <StatTile value={pct(d.winrate)} label="Winrate" />
          <StatTile value={String(d.games_played)} label="Games" />
          {d.placement != null && <StatTile value={`#${d.placement}`} label="Placement" />}
        </div>
      )}

      <div
        style={{
          display: "grid",
          gridTemplateColumns: "auto minmax(0, 1fr)",
          gap: "2rem",
          marginTop: "1rem",
          alignItems: "start",
        }}
      >
        <CardFan cards={main} />
        <div>
          <h2>Decklist</h2>
          <div style={{ columnCount: 2, columnGap: "1.5rem" }}>
            {main.map((c) => (
              <div
                key={c.card_name}
                style={{
                  breakInside: "avoid",
                  opacity: c.is_resolved ? 1 : 0.5,
                }}
              >
                <span className="muted">{c.quantity}×</span> {c.card_name}
                {!c.is_resolved && <span className="muted"> (unresolved)</span>}
              </div>
            ))}
          </div>
          {unresolved.length > 0 && (
            <p className="muted" style={{ marginTop: "1rem", fontSize: "0.85rem" }}>
              {unresolved.length} card{unresolved.length > 1 ? "s" : ""} could not be matched to the
              card database.
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
      </div>
    </main>
  );
}
