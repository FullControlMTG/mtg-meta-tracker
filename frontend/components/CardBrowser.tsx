"use client";

import { useDeferredValue, useMemo, useState } from "react";
import { CardFan, type FanCard } from "@/components/CardFan";
import { CARD_SORTS, sortCardsBy, type CardSortKey, type SortableCard } from "@/lib/colors";
import { matchesQuery } from "@/lib/search";
import type { SortDir } from "@/lib/tableSort";

// The sections a cube pool (colors) or a decklist (boards) is read in, plus one filter
// across all of them: a section whose cards all filter out drops away, so what is left on
// screen is only ever the matches. Both pages group server-side; the order within a
// section is the reader's pick, from the sort control here.
//
// Color sections (`colorSections`) are themselves a sort by color, so they only stand
// while that is the sort: pick mana value or name and the pool becomes one list, since a
// "by mana value" order that restarts at every color is not the order that was asked for.
// A deck's boards are not an order but a fact about the deck, and stay whatever the sort.
//
// The search is opt-in (`searchable`). A cube pool is hundreds of cards and unreadable
// without it; a decklist is forty you can already see, so there it is off and the bar —
// input and running count both — does not render at all.

type BrowserCard = FanCard & SortableCard;

export interface CardSection {
  key: string;
  label: string;
  cards: BrowserCard[];
}

// A deck counts a card as played (a 17× basic is 17 cards); a cube pool is singleton.
function count(cards: BrowserCard[], countQuantity: boolean): number {
  if (!countQuantity) return cards.length;
  return cards.reduce((n, c) => n + (c.quantity ?? 1), 0);
}

export function CardBrowser({
  sections,
  maxCols,
  countQuantity = false,
  searchable = true,
  colorSections = false,
  placeholder = "Search cards…",
}: {
  sections: CardSection[];
  maxCols?: number;
  countQuantity?: boolean;
  searchable?: boolean;
  colorSections?: boolean;
  placeholder?: string;
}) {
  const [query, setQuery] = useState("");
  const [sortKey, setSortKey] = useState<CardSortKey>("color");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  // Typing stays responsive while a few hundred card images re-flow behind it.
  const deferred = useDeferredValue(query);
  const filtering = searchable && deferred.trim() !== "";

  const sorted = useMemo(() => {
    let grouped = sections;
    if (colorSections && sortKey !== "color") {
      grouped = [{ key: "all", label: "All cards", cards: sections.flatMap((s) => s.cards) }];
    } else if (colorSections && sortDir === "desc") {
      // The sections are the color order's first key, so they turn around with it.
      grouped = [...sections].reverse();
    }
    return grouped.map((s) => ({ ...s, cards: sortCardsBy(s.cards, sortKey, sortDir) }));
  }, [sections, colorSections, sortKey, sortDir]);

  const shown = useMemo(
    () =>
      !filtering
        ? sorted.filter((s) => s.cards.length > 0)
        : sorted
            .map((s) => ({ ...s, cards: s.cards.filter((c) => matchesQuery(c.card_name, deferred)) }))
            .filter((s) => s.cards.length > 0),
    [sorted, deferred, filtering],
  );

  const total = sections.reduce((n, s) => n + count(s.cards, countQuantity), 0);
  const matched = shown.reduce((n, s) => n + count(s.cards, countQuantity), 0);

  return (
    <>
      <div className="search-bar">
        {searchable && (
          <>
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
          </>
        )}
        <div className="card-sort">
          <select
            value={sortKey}
            onChange={(e) => setSortKey(e.target.value as CardSortKey)}
            aria-label="Sort cards by"
          >
            {CARD_SORTS.map((o) => (
              <option key={o.key} value={o.key}>
                Sort: {o.label}
              </option>
            ))}
          </select>
          <button
            type="button"
            className="ghost-button"
            onClick={() => setSortDir((d) => (d === "asc" ? "desc" : "asc"))}
            aria-label={`Sort ${sortDir === "asc" ? "ascending" : "descending"}; switch direction`}
          >
            <span aria-hidden="true">{sortDir === "asc" ? "▲" : "▼"}</span>
            {sortDir === "asc" ? "Ascending" : "Descending"}
          </button>
        </div>
      </div>

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
