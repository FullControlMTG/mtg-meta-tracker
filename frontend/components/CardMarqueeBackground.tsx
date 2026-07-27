"use client";

import { useEffect, useMemo, useState } from "react";
import { apiGet, type CubeView, type CubeCard } from "@/lib/api";

// One horizontal band of art. The lane's own container animates a translateX
// loop from 0 to -50%; the images inside are duplicated end-to-end, so the
// second copy scrolls into the seam left by the first and there is no jump.
const LANES = 4;

// Card art the login page drifts across in the background. The images are the
// cached art crops for cards in the first configured cube — the same URL a stats
// row's hover preview would hit, so the browser cache does double duty. The
// background degrades silently: no cube, no cards, or an unreachable backend
// leaves an empty layer and the login form still works.
export function CardMarqueeBackground() {
  const [cards, setCards] = useState<CubeCard[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const cubes = await apiGet<CubeView[]>("/cubes", 300);
        const first = cubes[0];
        if (!first) return;
        const list = await apiGet<CubeCard[]>(`/cubes/${first.cube.id}/cards`, 300);
        if (!cancelled) setCards(list.filter((c) => c.card_id));
      } catch {
        // The background is decoration; a failed fetch is not worth surfacing.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  // Split the pool across the lanes with a deterministic shuffle per mount, so
  // the four rows aren't the alphabet in stripes but also aren't different on
  // every rerender. Empty until the fetch resolves.
  const lanes = useMemo(() => buildLanes(cards, LANES), [cards]);

  return (
    <div className="marquee" aria-hidden>
      {lanes.map((laneCards, i) => (
        <div
          key={i}
          className={`marquee-lane marquee-lane-${i % 2 === 0 ? "left" : "right"}`}
          style={{ animationDuration: `${45 + i * 8}s` }}
        >
          <div className="marquee-track">
            {[0, 1].map((copy) => (
              <div key={copy} className="marquee-row">
                {laneCards.map((c, j) => (
                  <img
                    key={`${copy}-${j}-${c.card_id}`}
                    src={`/api/cards/${c.card_id}/image?v=art_crop`}
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

function buildLanes(cards: CubeCard[], laneCount: number): CubeCard[][] {
  if (cards.length === 0) return Array.from({ length: laneCount }, () => []);
  // Fisher-Yates with a fixed seed derived from the pool size, so a rerender
  // that hands in the same list produces the same order — no reshuffle churn.
  const shuffled = [...cards];
  let seed = cards.length * 2654435761;
  for (let i = shuffled.length - 1; i > 0; i--) {
    seed = (seed * 1664525 + 1013904223) >>> 0;
    const j = seed % (i + 1);
    [shuffled[i], shuffled[j]] = [shuffled[j], shuffled[i]];
  }
  // Each lane gets every card, sliced from a rotated starting point — so a
  // small cube still fills every lane, and no two lanes start on the same art.
  const lanes: CubeCard[][] = [];
  const rotation = Math.max(1, Math.floor(shuffled.length / laneCount));
  for (let l = 0; l < laneCount; l++) {
    const start = (l * rotation) % shuffled.length;
    lanes.push([...shuffled.slice(start), ...shuffled.slice(0, start)]);
  }
  return lanes;
}
