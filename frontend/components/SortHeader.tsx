"use client";

import { InfoHint } from "@/components/InfoHint";
import type { Sort, SortColumn } from "@/lib/tableSort";

// One sortable column heading, for any table built on useTableSort.
export function SortHeader<T>({
  col,
  sort,
  onSort,
}: {
  col: SortColumn<T>;
  sort: Sort | null;
  onSort: (key: string) => void;
}) {
  const active = sort?.key === col.key;
  // Idle, the caret previews the direction a first click would give (see .sort-caret).
  const dir = active ? sort.dir : col.descFirst ? "desc" : "asc";
  return (
    <th
      scope="col"
      className={[col.num ? "num" : "", "sortable", active ? "active" : ""]
        .filter(Boolean)
        .join(" ")}
      aria-sort={active ? (dir === "asc" ? "ascending" : "descending") : "none"}
    >
      {/* A real button: a click handler on a bare th is not keyboard-operable. */}
      <button type="button" className="th-sort" onClick={() => onSort(col.key)}>
        {col.label}
        <span className="sort-caret" aria-hidden="true">
          {dir === "asc" ? "▲" : "▼"}
        </span>
      </button>
      {/* Outside the button: InfoHint is itself focusable, and a focusable inside a
          button is invalid HTML. */}
      {col.hint && <InfoHint text={col.hint} />}
    </th>
  );
}
