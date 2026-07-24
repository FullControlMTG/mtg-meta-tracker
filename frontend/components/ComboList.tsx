import { Fragment } from "react";
import Image from "next/image";
import Link from "next/link";
import type { Combo } from "@/lib/api";

// Two or three pieces at this width sit on one row inside the 1040px container,
// and a phone drops to one per row rather than shrinking them past reading size.
const PIECE_W = 168;
const PIECE_H = Math.round((PIECE_W * 88) / 63); // MTG card aspect ratio

// The combos a deck assembles, each spelled out as its pieces. Named sets of
// cards an admin configured per cube (see /admin/combos); the pieces are shown
// rather than only listed because "Thassa's Oracle + Demonic Consultation" means
// nothing to a reader who has not met the cards.
export function ComboList({ combos }: { combos: Combo[] }) {
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
              {combo.cards.map((c, i) => (
                <Fragment key={c.card_id}>
                  {/* The pieces read as "A + B (+ C)"; the sign is decoration, and the
                      card links beside it already carry the names. */}
                  {i > 0 && (
                    <span className="muted" aria-hidden style={{ fontSize: "1.4rem" }}>
                      +
                    </span>
                  )}
                  <Link
                    href={`/cards/${c.slug}`}
                    title={c.card_name}
                    style={{ width: PIECE_W, maxWidth: "100%", display: "block" }}
                  >
                    <Image
                      // Same self-hosted image cache the card fans use; the Next
                      // optimizer would only re-fetch what the backend already
                      // sized and pinned.
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
                      }}
                      unoptimized
                    />
                  </Link>
                </Fragment>
              ))}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
}
