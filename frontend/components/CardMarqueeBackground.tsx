"use client";

import { useEffect, useMemo, useState } from "react";
import { apiGet } from "@/lib/api";

// Five vertical lanes drift the pool of card images behind the page's content.
// Each lane gets an independent shuffle of the sampled pool, so the columns
// are visually unrelated. Two identical strips per lane, with translateY(-50%)
// between them — see the CSS for why that is exactly one strip's height and
// therefore seamless.
const LANES = 5;

// Base seconds-per-card for the drift speed. Multiplied by the strip length,
// so a large pool loops slowly and a small one loops quickly rather than the
// same number of cards per second showing at every pool size (which would be
// a wall of blur for a big cube).
const SECONDS_PER_CARD = 4.5;

// The number of Scryfall UUIDs to pull for the marquee. Matches roughly the
// number of cards visible across all five lanes on a tall viewport before the
// loop repeats, so a visitor sees variety before seeing anything twice.
const SAMPLE_SIZE = 300;

interface SampleResponse {
  card_ids: string[];
}

// Card art drifting behind an anonymous page's content. The images are cached
// full-card pngs for a random sample of the card catalog, fetched from a
// dedicated public endpoint so a signed-out visitor never needs to
// authenticate. The background degrades silently: an empty response or a
// failed fetch leaves the tinted overlay and the page still works.
//
// It is pinned to the viewport, so on the landing page — which scrolls for four
// screens — it stays put while the panels travel over it. That is the whole
// parallax: the marquee is the far plane and never moves with the scroll.
export function CardMarqueeBackground() {
  const [cards, setCards] = useState<{ card_id: string }[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await apiGet<SampleResponse>(`/cards/sample?n=${SAMPLE_SIZE}`, 300);
        if (cancelled) return;
        setCards(res.card_ids.map((id) => ({ card_id: id })));
      } catch {
        // Decoration; a failed fetch is not worth surfacing.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Shuffle independently per lane so the columns read as unrelated. The
  // seed folds in the lane index, so a rerender of the same pool produces
  // the same layout (no shuffle churn on state changes elsewhere on the page).
  const lanes = useMemo(() => {
    if (cards.length === 0) return Array.from({ length: LANES }, () => [] as { card_id: string }[]);
    return Array.from({ length: LANES }, (_, i) => shuffle(cards, cards.length * 31 + i));
  }, [cards]);

  // Duration scales with strip length so a 500-card pool doesn't fly by.
  // Staggered per lane by a prime-ish factor so no two lanes ever land at
  // the same phase — otherwise adjacent lanes drift in visible lockstep.
  const durations = useMemo(
    () =>
      lanes.map((laneCards, i) => {
        const base = Math.max(405, laneCards.length * SECONDS_PER_CARD);
        return base + i * 63;
      }),
    [lanes],
  );

  return (
    <div className="marquee" aria-hidden>
      {lanes.map((laneCards, i) => (
        <div
          key={i}
          className={`marquee-lane marquee-lane-${i % 2 === 0 ? "up" : "down"}`}
        >
          <div className="marquee-track" style={{ animationDuration: `${durations[i]}s` }}>
            {[0, 1].map((copy) => (
              <div key={copy} className="marquee-strip">
                {laneCards.map((c, j) => (
                  <img
                    key={`${copy}-${j}-${c.card_id}`}
                    src={`/api/cards/${c.card_id}/image?v=normal`}
                    alt=""
                    className="marquee-img"
                    loading="lazy"
                    draggable={false}
                  />
                ))}
              </div>
            ))}
          </div>
        </div>
      ))}
      <div className="marquee-overlay" />
    </div>
  );
}

// Fisher-Yates with a fixed seed so a rerender with the same input produces
// the same order — no reshuffle churn.
function shuffle<T>(items: T[], seed: number): T[] {
  const out = items.slice();
  let s = seed >>> 0;
  for (let i = out.length - 1; i > 0; i--) {
    s = (s * 1664525 + 1013904223) >>> 0;
    const j = s % (i + 1);
    [out[i], out[j]] = [out[j], out[i]];
  }
  return out;
}
