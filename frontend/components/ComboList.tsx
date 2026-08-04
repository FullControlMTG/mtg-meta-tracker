import { Fragment } from "react";
import Image from "next/image";
import type { Combo, ComboPiece } from "@/lib/api";

// Two or three pieces at this width sit on one row inside the 1040px container,
// and a phone drops to one per row rather than shrinking them past reading size.
const PIECE_W = 168;
const PIECE_H = Math.round((PIECE_W * 88) / 63); // MTG card aspect ratio

// Clicking a piece opens its printing on Scryfall, like every other card in the app.
// Undefined when the exact printing isn't known, in which case the image isn't a link.
function scryfallHref(c: ComboPiece): string | undefined {
  if (!c.set_code || !c.collector_number) return undefined;
  return `https://scryfall.com/card/${c.set_code}/${c.collector_number}`;
}

// The combos a deck assembles, each spelled out as its pieces. Named sets of cards the
// cube owner configured; the pieces are shown rather than only listed because "Thassa's
// Oracle + Demonic Consultation" means nothing to a reader who hasn't met the cards.
//
// missingCardIds, when given, marks pieces that are NOT in the cube pool — a red glow
// and a "Not active in cube" label on hover. Presence is never signalled; a piece in the
// pool looks like an ordinary card, and the reader assumes it belongs.
export function ComboList({
  combos,
  missingCardIds,
  title,
}: {
  combos: Combo[];
  missingCardIds?: Set<string>;
  // A section heading, e.g. "Combos" on a deck page. Omitted where the surrounding UI
  // already labels the list (the cube's Combos tab).
  title?: string;
}) {
  if (combos.length === 0) return null;

  return (
    <section style={{ marginTop: "1.5rem" }}>
      {title && (
        <h2 style={{ marginBottom: "0.5rem" }}>
          {title}{" "}
          <span className="muted" style={{ fontWeight: 400 }}>
            ({combos.length})
          </span>
        </h2>
      )}
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
                const href = scryfallHref(c);
                const inner = (
                  <>
                    <Image
                      src={`/api/cards/${c.card_id}/image`}
                      alt={c.card_name}
                      width={PIECE_W}
                      height={PIECE_H}
                      style={{ width: "100%", height: "auto", display: "block", borderRadius: 10 }}
                      unoptimized
                    />
                    {missing && <span className="combo-missing">Not active in cube</span>}
                  </>
                );
                return (
                  <Fragment key={c.card_id}>
                    {/* The pieces read as "A + B (+ C)"; the sign is decoration. */}
                    {i > 0 && (
                      <span className="muted" aria-hidden style={{ fontSize: "1.4rem" }}>
                        +
                      </span>
                    )}
                    {href ? (
                      <a
                        className={`combo-piece${missing ? " missing" : ""}`}
                        href={href}
                        target="_blank"
                        rel="noopener noreferrer"
                        title={missing ? `${c.card_name} — not active in cube` : c.card_name}
                      >
                        {inner}
                      </a>
                    ) : (
                      <div
                        className={`combo-piece${missing ? " missing" : ""}`}
                        title={missing ? `${c.card_name} — not active in cube` : c.card_name}
                      >
                        {inner}
                      </div>
                    )}
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
