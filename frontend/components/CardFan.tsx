"use client";

import Image from "next/image";
import { useEffect, useRef, useState } from "react";

const DEFAULT_MIN_CARD_W = 200; // minimum readable card width in px → drives column count
const DEFAULT_MAX_COLS = 8; // a cube section holds far more cards than a deck; more columns → shorter
const CARD_RATIO = 88 / 63; // MTG card aspect ratio (height / width)
const PEEK_RATIO = 0.14; // fraction of a stacked card's height left visible (its title strip)

// Minimal shape shared by DecklistCard and CubeCard. is_resolved is optional —
// cube cards are always resolved, so treat a missing flag as resolved. quantity is
// optional too — a cube pool is singleton, so only decks ever badge a card.
type FanCard = {
  card_id?: string;
  card_name: string;
  image_normal?: string;
  is_resolved?: boolean;
  quantity?: number;
};

type Dims = { cols: number; cardW: number; cardH: number; strip: number };

function computeDims(containerW: number, maxCols: number, minCardW: number): Dims {
  const cols = Math.min(maxCols, Math.max(1, Math.floor(containerW / minCardW)));
  const cardW = containerW / cols;
  const cardH = cardW * CARD_RATIO;
  const strip = cardH * PEEK_RATIO;
  return { cols, cardW, cardH, strip };
}

// Prefer our same-origin image cache (backend downloads from Scryfall on a miss
// and self-hosts thereafter). Fall back to the raw Scryfall URL if the card has
// no id (e.g. an unresolved decklist entry).
function imageSrc(c: FanCard): string | undefined {
  if (c.card_id) return `/api/cards/${c.card_id}/image`;
  return c.image_normal;
}

// Full-width, responsive, multi-column overlaid spread. The container is measured
// and split into as many minCardW-wide columns as fit; cards fill sequentially down
// each column, stacked with ~86% overlap so only each card's title strip peeks.
// Hovering a card lifts it above its column siblings (see .fan-card in globals.css).
export function CardFan({
  cards,
  maxCols = DEFAULT_MAX_COLS,
  minCardW = DEFAULT_MIN_CARD_W,
}: {
  cards: FanCard[];
  maxCols?: number;
  minCardW?: number;
}) {
  const withArt = cards.filter((c) => c.is_resolved !== false && imageSrc(c));
  const containerRef = useRef<HTMLDivElement>(null);
  const [dims, setDims] = useState<Dims | null>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    const update = () => setDims(computeDims(el.offsetWidth, maxCols, minCardW));
    update();
    const ro = new ResizeObserver(update);
    ro.observe(el);
    return () => ro.disconnect();
  }, [maxCols, minCardW]);

  if (withArt.length === 0) return null;

  const columns: FanCard[][] = [];
  if (dims) {
    const perCol = Math.ceil(withArt.length / dims.cols);
    for (let c = 0; c < dims.cols; c++) {
      const slice = withArt.slice(c * perCol, (c + 1) * perCol);
      if (slice.length > 0) columns.push(slice);
    }
  }

  return (
    <div ref={containerRef} style={{ display: "flex", alignItems: "flex-start", width: "100%" }}>
      {dims &&
        columns.map((col, ci) => (
          <div key={ci} style={{ width: dims.cardW, flexShrink: 0, position: "relative" }}>
            {col.map((c, i) => (
              <div
                key={c.card_name}
                style={{
                  marginTop: i === 0 ? 0 : -(dims.cardH - dims.strip),
                  position: "relative",
                  zIndex: i,
                }}
              >
                <Image
                  className="fan-card"
                  src={imageSrc(c) as string}
                  alt={c.card_name}
                  width={Math.round(dims.cardW)}
                  height={Math.round(dims.cardH)}
                  // The backend already serves self-hosted, correctly-sized, immutable images,
                  // so the Next optimizer is a redundant fetch/optimize/cache layer (and an extra
                  // failure surface under a cold-cache burst). Go straight to the source.
                  unoptimized
                />
                {(c.quantity ?? 1) > 1 && (
                  <span
                    className="pill fan-qty"
                    // Sits on the title strip — the only part of a stacked card still visible.
                    style={{ top: dims.strip * 0.18, right: dims.cardW * 0.06 }}
                  >
                    ×{c.quantity}
                  </span>
                )}
              </div>
            ))}
          </div>
        ))}
    </div>
  );
}
