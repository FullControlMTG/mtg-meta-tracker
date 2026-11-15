"use client";

import { useDeferredValue, useMemo, useState } from "react";
import { CardFan, type FanCard } from "@/components/CardFan";
import {
  CARD_GROUPS,
  CARD_SORTS,
  GROUP_SORT,
  groupCardsBy,
  sortCardsBy,
  type CardGroupKey,
  type CardSortKey,
  type GroupableCard,
} from "@/lib/colors";
import { matchesQuery } from "@/lib/search";
import type { SortDir } from "@/lib/tableSort";

// A cube pool or a decklist, split into sections by one grouping and ordered within each
// by one sort, plus one filter across all of them: a section whose cards all filter out
// drops away, so what is left on screen is only ever the matches.
//
// Grouping and sorting are both done here, on the client, over the cards the page was
// handed whole. The pages only fetch; that keeps the API's responses the same whatever
// the reader picks, which is what lets them be cached.
//
// `groupings` lists the groupings this browser offers, the first being its default: a
// pool opens by color, a deck by board, since a deck's boards are a fact about the deck.
// When the grouping and the sort share a key (color and color, say), the sections are
// that sort's first key and turn around with its direction; otherwise they stay put.
//
// The search is opt-in (`searchable`). A cube pool is hundreds of cards and unreadable
// without it; a decklist is forty you can already see, so there it is off and the bar —
// input and running count both — does not render at all.

type BrowserCard = FanCard & GroupableCard;

// A deck counts a card as played (a 17× basic is 17 cards); a cube pool is singleton.
function count(cards: BrowserCard[], countQuantity: boolean): number {
  if (!countQuantity) return cards.length;
  return cards.reduce((n, c) => n + (c.quantity ?? 1), 0);
}

export function CardBrowser({
  cards,
  groupings,
  maxCols,
  countQuantity = false,
  searchable = true,
  placeholder = "Search cards…",
}: {
  cards: BrowserCard[];
  groupings: CardGroupKey[];
  maxCols?: number;
  countQuantity?: boolean;
  searchable?: boolean;
  placeholder?: string;
}) {
  const [query, setQuery] = useState("");
  const [groupKey, setGroupKey] = useState<CardGroupKey>(groupings[0] ?? "none");
  const [sortKey, setSortKey] = useState<CardSortKey>("color");
  const [sortDir, setSortDir] = useState<SortDir>("asc");
  // Typing stays responsive while a few hundred card images re-flow behind it.
  const deferred = useDeferredValue(query);
  const filtering = searchable && deferred.trim() !== "";

  const sorted = useMemo(() => {
    let grouped = groupCardsBy(cards, groupKey);
    if (GROUP_SORT[groupKey] === sortKey && sortDir === "desc") grouped = grouped.reverse();
    return grouped.map((s) => ({ ...s, cards: sortCardsBy(s.cards, sortKey, sortDir) }));
  }, [cards, groupKey, sortKey, sortDir]);

  const shown = useMemo(
    () =>
      !filtering
        ? sorted
        : sorted
            .map((s) => ({ ...s, cards: s.cards.filter((c) => matchesQuery(c.card_name, deferred)) }))
            .filter((s) => s.cards.length > 0),
    [sorted, deferred, filtering],
  );

  const total = count(cards, countQuantity);
  const matched = shown.reduce((n, s) => n + count(s.cards, countQuantity), 0);
  const groupOptions = CARD_GROUPS.filter((o) => groupings.includes(o.key));

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
          {groupOptions.length > 1 && (
            <select
              value={groupKey}
              onChange={(e) => setGroupKey(e.target.value as CardGroupKey)}
              aria-label="Group cards by"
            >
              {groupOptions.map((o) => (
                <option key={o.key} value={o.key}>
                  Group: {o.label}
                </option>
              ))}
            </select>
          )}
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
