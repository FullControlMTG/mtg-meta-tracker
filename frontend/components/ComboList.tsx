import { Fragment } from "react";
import Image from "next/image";
import Link from "next/link";
import type { Combo } from "@/lib/api";

// Two or three pieces at this width sit on one row inside the 1040px container,
// and a phone drops to one per row rather than shrinking them past reading size.
const PIECE_W = 168;
const PIECE_H = Math.round((PIECE_W * 88) / 63); // MTG card aspect ratio

// The combos a deck assembles, each spelled out as its pieces. Named sets of cards
// the cube owner configured; the pieces are shown rather than only listed because
// "Thassa's Oracle + Demonic Consultation" means nothing to a reader who has not met
// the cards. cubeId scopes the piece links to this cube's card pages.
//
// missingCardIds, when given, flags pieces no longer in the cube pool — a combo can
// outlive a card's removal from the list. Only absence is signalled; a piece that's in
// the pool gets no marking.
export function ComboList({
  combos,
  cubeId,
  missingCardIds,
}: {
  combos: Combo[];
  cubeId: string;
  missingCardIds?: Set<string>;
}) {
  if (combos.length === 0) return null;

  return (
    <section style={{ marginTop: "1.5rem" }}>
      <h2 style={{ marginBottom: "0.5rem" }}>
        Combos{" "}
        <span className="muted" style={{ fontWeight: 400 }}>
          ({combos.length})
        </span>
      </h2>
      <div style={{ display: "flex", flexDirection: "column", gap: "0.75rem" }}>
        {combos.map((combo) => (
          <div key={combo.id} className="card">
            <strong style={{ fontSize: "1.05rem" }}>{combo.name}</strong>
            {combo.description && (
              <p className="muted" style={{ margin: "0.25rem 0 0", fontSize: "0.9rem" }}>
                {combo.description}
              </p>
            )}
            <div
              style={{
                display: "flex",
                alignItems: "center",
                flexWrap: "wrap",
                gap: "0.5rem",
                marginTop: "0.75rem",
              }}
            >
              {combo.cards.map((c, i) => {
                const missing = missingCardIds?.has(c.card_id) ?? false;
                return (
                  <Fragment key={c.card_id}>
                    {/* The pieces read as "A + B (+ C)"; the sign is decoration, and the
                        card links beside it already carry the names. */}
                    {i > 0 && (
                      <span className="muted" aria-hidden style={{ fontSize: "1.4rem" }}>
                        +
                      </span>
                    )}
                    <Link
                      href={`/cube/${cubeId}/cards/${c.slug}`}
                      title={missing ? `${c.card_name} — not in the cube pool` : c.card_name}
                      style={{ width: PIECE_W, maxWidth: "100%", display: "block", position: "relative" }}
                    >
                      <Image
                        src={`/api/cards/${c.card_id}/image`}
                        alt={c.card_name}
                        width={PIECE_W}
                        height={PIECE_H}
                        style={{
                          width: "100%",
                          height: "auto",
                          display: "block",
                          borderRadius: 10,
                          boxShadow: "0 2px 8px rgba(0, 0, 0, 0.35)",
                          // Dim a piece that's fallen out of the pool; the badge names why.
                          opacity: missing ? 0.5 : 1,
                        }}
                        unoptimized
                      />
                      {missing && <span className="combo-missing">Not in cube</span>}
                    </Link>
                  </Fragment>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
