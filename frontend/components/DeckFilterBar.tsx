"use client";

import { FIELDS, type CompiledQuery } from "@/lib/deckQuery";

// The panel behind the deck table's Filter button: one text input holding a query in
// the language of lib/deckQuery.ts, plus the reference that makes that language
// discoverable. A query language nobody can see the fields of is a language nobody
// uses, so the field list is generated from FIELDS — a new field documents itself,
// and its example is clickable rather than something to retype.

export function DeckFilterBar({
  query,
  onQuery,
  compiled,
  matched,
  total,
}: {
  query: string;
  onQuery: (q: string) => void;
  compiled: CompiledQuery;
  matched: number;
  total: number;
}) {
  // Append rather than replace: the examples are how a query gets built up a term at
  // a time, and terms are ANDed, so two clicks give a narrower filter and not a
  // clobbered one.
  const append = (term: string) => {
    const cur = query.trim();
    onQuery(cur ? `${cur} ${term}` : term);
  };

  return (
    <div className="filter-panel" id="deck-filter">
      <div className="filter-row">
        <input
          className="search"
          type="search"
          value={query}
          onChange={(e) => onQuery(e.target.value)}
          placeholder="losses:0 games>0 c:ur"
          aria-label="Filter decks by query"
          autoFocus
          spellCheck={false}
        />
        <button
          type="button"
          className="ghost-button"
          onClick={() => onQuery("")}
          disabled={query === ""}
        >
          Clear
        </button>
        <span className="muted" style={{ fontSize: "0.85rem" }}>
          {compiled.active ? `${matched} of ${total} decks` : `${total} decks`}
        </span>
      </div>

      {compiled.errors.length > 0 && (
        // The terms that did parse are still applied — say which ones were not, rather
        // than leaving a filter that quietly does less than it reads like it does.
        <ul className="filter-errors">
          {compiled.errors.map((e) => (
            <li key={e}>{e}</li>
          ))}
        </ul>
      )}

      <details className="filter-help">
        <summary>Fields</summary>
        <p className="muted">
          Terms are combined with AND, and <code>-</code> in front of one excludes it
          (<code>-user:jake</code>). Values with a space need quotes. A bare word with no
          field searches the deck name. Comparisons take <code>:</code> <code>=</code>{" "}
          <code>!=</code> <code>&gt;</code> <code>&gt;=</code> <code>&lt;</code>{" "}
          <code>&lt;=</code>.
        </p>
        <dl className="filter-fields">
          {FIELDS.map((f) => (
            <div key={f.key}>
              <dt>
                <button
                  type="button"
                  className="filter-example"
                  onClick={() => append(f.example)}
                  title={`Add ${f.example} to the filter`}
                >
                  {f.example}
                </button>
                {f.aliases.length > 0 && (
                  <span className="muted"> or {f.aliases.map((a) => a + ":").join(" ")}</span>
                )}
              </dt>
              <dd className="muted">{f.hint}</dd>
            </div>
          ))}
        </dl>
      </details>
    </div>
  );
}
