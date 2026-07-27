"use client";

import { useEffect, useMemo, useState } from "react";
import { apiGet, type CubeView, type CubeCard } from "@/lib/api";

// Five vertical lanes drift the pool of card images behind the login form.
// Each lane gets an independent shuffle of every card across every cube, so
// the four columns are visually unrelated. Two identical strips per lane,
// with translateY(-50%) between them — see the CSS for why that is exactly
// one strip's height and therefore seamless.
const LANES = 5;

// Base seconds-per-card for the drift speed. Multiplied by the strip length,
// so a large pool loops slowly and a small one loops quickly rather than the
// same number of cards per second showing at every pool size (which would be
// a wall of blur for a big cube).
const SECONDS_PER_CARD = 4.5;

interface Loaded {
  card_id: string;
}

// Card art the login page drifts behind the form. The images are the cached
// full-card pngs for every card across every cube — same URL the stats-row
// preview hits, so browser cache is reused. The background degrades silently:
// no cubes, no cards, or an unreachable backend leaves an empty layer and the
// login form still works.
export function CardMarqueeBackground() {
  const [cards, setCards] = useState<Loaded[]>([]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const cubes = await apiGet<CubeView[]>("/cubes", 300);
        if (cubes.length === 0) return;
        const results = await Promise.all(
          cubes.map((c) =>
            apiGet<CubeCard[]>(`/cubes/${c.cube.id}/cards`, 300).catch(() => []),
          ),
        );
        // One card_id can live in more than one cube; the marquee shows the
        // picture, not the copies, so dedupe.
        const seen = new Set<string>();
        const flat: Loaded[] = [];
        for (const list of results) {
          for (const c of list) {
            if (!c.card_id || seen.has(c.card_id)) continue;
            seen.add(c.card_id);
            flat.push({ card_id: c.card_id });
          }
        }
        if (!cancelled) setCards(flat);
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
  // the same layout (no shuffle churn on state changes elsewhere in login).
  const lanes = useMemo(() => {
    if (cards.length === 0) return Array.from({ length: LANES }, () => [] as Loaded[]);
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
