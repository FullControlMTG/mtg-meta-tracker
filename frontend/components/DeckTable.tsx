"use client";

import Link from "next/link";
import { useDeferredValue, useEffect, useMemo, useState, type ReactNode } from "react";
import { ColorPips } from "@/components/ColorPips";
import { DeckFilterBar } from "@/components/DeckFilterBar";
import { SortHeader } from "@/components/SortHeader";
import type { DecklistListItem } from "@/lib/api";
import { compareIdentity } from "@/lib/colors";
import { compileQuery, filterDecks } from "@/lib/deckQuery";
import { fmtDate, isoDay, pct } from "@/lib/format";
import { useTableSort, type Sort, type SortColumn } from "@/lib/tableSort";

const byName = (a: DecklistListItem, b: DecklistListItem) =>
  a.decklist.name.localeCompare(b.decklist.name, undefined, { sensitivity: "base" });

// See lib/tableSort.ts for the rules these obey: comparators ascending, blanks last,
// numeric columns descending on the first click.
const COLUMNS: SortColumn<DecklistListItem>[] = [
  { key: "name", label: "Deck", compare: byName },
  {
    key: "colors",
    label: "Colors",
    compare: (a, b) => compareIdentity(a.decklist.color_identity, b.decklist.color_identity),
  },
  {
    key: "played",
    label: "Date",
    // Not a number, but read newest-first like one, so the first click points down.
    // ISO days sort correctly as plain strings, which is why they are compared as such.
    descFirst: true,
    compare: (a, b) => isoDay(a.decklist.played_at).localeCompare(isoDay(b.decklist.played_at)),
  },
  {
    key: "record",
    label: "Record",
    num: true,
    descFirst: true,
    // Fewest wins first, and among equal wins the most losses first — so reversed (the
    // default click) this reads most wins, then fewest losses. Draws are not collected,
    // so the chain ends there.
    compare: (a, b) =>
      a.decklist.wins - b.decklist.wins || b.decklist.losses - a.decklist.losses,
  },
  {
    key: "winrate",
    label: "Winrate",
    num: true,
    descFirst: true,
    // A deck that has played no games has no winrate to rank — `unknown` sinks it, so the
    // ?? 0 here is never reached with a blank.
    unknown: (d) => d.decklist.winrate == null,
    compare: (a, b) => (a.decklist.winrate ?? 0) - (b.decklist.winrate ?? 0),
  },
];

// A sort named in a URL (`?sort=record&dir=desc`), which is how a stat elsewhere in the
// app links to the view that explains it. An unknown column is dropped rather than
// honoured as a dead sort key: the server's own order is the honest fallback.
//
// The parsing happens here, on the client side of the boundary, so COLUMNS stays the
// only list of what is sortable. A page hands over the raw params — a server component
// cannot call a function exported from a "use client" module anyway.
function parseSort(key?: string, dir?: string): Sort | null {
  const col = COLUMNS.find((c) => c.key === key);
  if (!col) return null;
  // No direction named: take the one a first click on that header would give.
  const fallback = col.descFirst ? "desc" : "asc";
  return { key: col.key, dir: dir === "asc" || dir === "desc" ? dir : fallback };
}

export function DeckTable({
  decks,
  showArchetype = false,
  heading,
  actions,
  filterable = true,
  initialQuery = "",
  initialSort,
  syncUrl = false,
  emptyMessage = "No decklists yet.",
}: {
  decks: DecklistListItem[];
  showArchetype?: boolean;
  // The page's own title and buttons, hosted in the table's toolbar so the Filter
  // button costs no vertical space of its own — it joins a row that already exists.
  heading?: ReactNode;
  actions?: ReactNode;
  filterable?: boolean;
  // A query and a sort the page was linked with. A non-empty query opens the panel:
  // a list that arrives already filtered has to show what filtered it.
  initialQuery?: string;
  // The ?sort / ?dir params, unvalidated — see parseSort.
  initialSort?: { key?: string; dir?: string };
  // Mirror the live query and sort back into the address bar, so the view a filter
  // produced is a link. Off where the URL is not the deck list's own (a profile page).
  syncUrl?: boolean;
  emptyMessage?: string;
}) {
  const [query, setQuery] = useState(initialQuery);
  const [open, setOpen] = useState(initialQuery.trim() !== "");
  // Typing stays responsive while the table re-sorts behind it.
  const deferred = useDeferredValue(query);

  const compiled = useMemo(() => compileQuery(deferred), [deferred]);
  const matches = useMemo(
    () => (filterable ? filterDecks(decks, compiled) : decks),
    [decks, compiled, filterable],
  );

  // No initial sort unless one was asked for: the server returns most-recently-played
  // first, which is the default view — and matches what the Date column would give on
  // one click.
  const { rows, sort, toggle } = useTableSort(matches, COLUMNS, {
    initial: parseSort(initialSort?.key, initialSort?.dir),
    tiebreak: byName,
  });

  // replaceState rather than a router push: the query is client-side state over a list
  // the page already has, so re-running the server component would fetch the same
  // payload again to render the same rows.
  useEffect(() => {
    if (!syncUrl) return;
    const url = new URL(window.location.href);
    const q = deferred.trim();
    if (q) url.searchParams.set("q", q);
    else url.searchParams.delete("q");
    if (sort) {
      url.searchParams.set("sort", sort.key);
      url.searchParams.set("dir", sort.dir);
    } else {
      url.searchParams.delete("sort");
      url.searchParams.delete("dir");
    }
    window.history.replaceState(null, "", url);
  }, [syncUrl, deferred, sort]);

  // Nothing to narrow: a Filter button over an empty list is a button that can only
  // disappoint. The toolbar stays, because its heading and buttons are the page's.
  const showFilter = filterable && decks.length > 0;

  const toolbar = (heading || actions || showFilter) && (
    <div className="table-toolbar">
      {heading}
      <div className="toolbar-actions">
        {showFilter && (
          <button
            type="button"
            className={`ghost-button${compiled.active ? " active" : ""}`}
            aria-expanded={open}
            aria-controls="deck-filter"
            onClick={() => setOpen((v) => !v)}
          >
            {/* A funnel, drawn rather than borrowed: there is no icon set here, and a
                glyph that renders as a box on one platform is worse than 40 bytes. */}
            <svg width="11" height="11" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
              <path d="M1 2h14L9.5 8.4V14L6.5 12.2V8.4z" fill="currentColor" />
            </svg>
            Filter
            {compiled.active && <span className="ghost-count">{matches.length}</span>}
          </button>
        )}
        {actions}
      </div>
    </div>
  );

  return (
    <>
      {toolbar}
      {showFilter && open && (
        <DeckFilterBar
          query={query}
          onQuery={setQuery}
          compiled={compiled}
          matched={matches.length}
          total={decks.length}
        />
      )}

      {rows.length === 0 ? (
        // An empty list and an empty result are different facts. Say which one it is,
        // so a filter that matches nothing does not read as a playgroup with no decks.
        <p className="muted">{compiled.active ? "No decks match this filter." : emptyMessage}</p>
      ) : (
        <div className="card" style={{ marginTop: "1rem", overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                {COLUMNS.map((col) => (
                  <SortHeader key={col.key} col={col} sort={sort} onSort={toggle} />
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map(({ decklist: d }) => (
                <tr key={d.id}>
                  <td>
                    <Link href={`/decks/${d.id}`}>{d.name}</Link>
                    {showArchetype && d.archetype && (
                      <span className="muted" style={{ marginLeft: 6, fontSize: "0.85rem" }}>
                        {d.archetype}
                      </span>
                    )}
                  </td>
                  <td>
                    <ColorPips bits={d.color_identity} splash={d.splash_colors} showCode />
                  </td>
                  <td style={{ whiteSpace: "nowrap" }}>{fmtDate(d.played_at)}</td>
                  <td className="num">{d.games_played > 0 ? `${d.wins}-${d.losses}` : "—"}</td>
                  <td className="num">{pct(d.winrate)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}
