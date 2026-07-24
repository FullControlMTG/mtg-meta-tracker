"use client";

import { useDeferredValue, useMemo, useState } from "react";
import { CardFan, type FanCard } from "@/components/CardFan";
import { matchesQuery } from "@/lib/search";

// The sections a cube pool (colors) or a decklist (boards) is read in, plus one filter
// across all of them: a section whose cards all filter out drops away, so what is left on
// screen is only ever the matches. Both pages sort and group server-side and hand the
// result here — this component decides nothing about order, only about what is shown.
//
// The search is opt-in (`searchable`). A cube pool is hundreds of cards and unreadable
// without it; a decklist is forty you can already see, so there it is off and the bar —
// input and running count both — does not render at all.

export interface CardSection {
  key: string;
  label: string;
  cards: FanCard[];
}

// A deck counts a card as played (a 17× basic is 17 cards); a cube pool is singleton.
function count(cards: FanCard[], countQuantity: boolean): number {
  if (!countQuantity) return cards.length;
  return cards.reduce((n, c) => n + (c.quantity ?? 1), 0);
}

export function CardBrowser({
  sections,
  maxCols,
  countQuantity = false,
  searchable = true,
  placeholder = "Search cards…",
}: {
  sections: CardSection[];
  maxCols?: number;
  countQuantity?: boolean;
  searchable?: boolean;
  placeholder?: string;
}) {
  const [query, setQuery] = useState("");
  // Typing stays responsive while a few hundred card images re-flow behind it.
  const deferred = useDeferredValue(query);
  const filtering = searchable && deferred.trim() !== "";

  const shown = useMemo(
    () =>
      !filtering
        ? sections.filter((s) => s.cards.length > 0)
        : sections
            .map((s) => ({ ...s, cards: s.cards.filter((c) => matchesQuery(c.card_name, deferred)) }))
            .filter((s) => s.cards.length > 0),
    [sections, deferred, filtering],
  );

  const total = sections.reduce((n, s) => n + count(s.cards, countQuantity), 0);
  const matched = shown.reduce((n, s) => n + count(s.cards, countQuantity), 0);

  return (
    <>
      {searchable && (
        <div className="search-bar">
          <input
            className="search"
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={placeholder}
            aria-label="Search cards by name"
          />
          <span className="muted" style={{ fontSize: "0.85rem" }}>
            {filtering ? `${matched} of ${total}` : `${total} cards`}
          </span>
        </div>
      )}

      {shown.length === 0 ? (
        // Empty with the search off is not a failed search — say the true thing.
        <p className="muted">
          {filtering ? `No cards match “${deferred.trim()}”.` : "No cards."}
        </p>
      ) : (
        <div className="card-sections">
          {shown.map((s) => (
            <section key={s.key}>
              <h2 className="card-section-title">
                {s.label}{" "}
                <span className="muted" style={{ fontWeight: 400 }}>
                  · {count(s.cards, countQuantity)}
                </span>
              </h2>
              <CardFan cards={s.cards} maxCols={maxCols} />
            </section>
          ))}
        </div>
      )}
    </>
  );
}
