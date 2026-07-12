"use client";

import { useMemo, useState } from "react";

// The one sort behind every sortable table (the decklists, the cube's cards). A table
// declares its columns and hands them to useTableSort; SortHeader renders them. Adding a
// column is a line in that array, and the rules below — how ties break, where the blanks
// go, which way the first click points — are then the same table to table for free.

export type SortDir = "asc" | "desc";

export interface Sort {
  key: string;
  dir: SortDir;
}

export interface SortColumn<T> {
  key: string;
  label: string;
  num?: boolean; // a right-aligned numeric column
  hint?: string; // an InfoHint (i) beside the label

  // Always written ascending — "worst"/"first" to "best"/"last". A descending sort negates
  // it, so encoding the descent here as well would cancel out and quietly sort the column
  // backwards.
  compare: (a: T, b: T) => number;

  // Numeric columns are read best-first, so their first click should sort descending.
  descFirst?: boolean;

  // A row whose value is *unknown* rather than low — a deck that has played no games has
  // no winrate — ranks nowhere, and sinks to the bottom whichever way the column points.
  // Hence it is resolved outside `sign`, above the comparator.
  unknown?: (row: T) => boolean;
}

export function useTableSort<T>(
  rows: T[],
  columns: SortColumn<T>[],
  {
    initial = null,
    tiebreak,
  }: {
    // Null leaves the server's own order alone until the first click.
    initial?: Sort | null;
    // Breaks every tie, so equal rows hold one predictable order and re-sorting a column
    // never reshuffles them.
    tiebreak: (a: T, b: T) => number;
  },
) {
  const [sort, setSort] = useState<Sort | null>(initial);

  const sorted = useMemo(() => {
    const col = sort && columns.find((c) => c.key === sort.key);
    if (!col || !sort) return rows;
    const sign = sort.dir === "asc" ? 1 : -1;
    // A copy: the caller's array is a server-fetched payload, not ours to sort in place.
    return [...rows].sort((a, b) => {
      if (col.unknown) {
        const au = col.unknown(a);
        const bu = col.unknown(b);
        if (au || bu) return au && bu ? tiebreak(a, b) : au ? 1 : -1;
      }
      return sign * col.compare(a, b) || tiebreak(a, b);
    });
  }, [rows, columns, sort, tiebreak]);

  const toggle = (key: string) =>
    setSort((cur) =>
      cur?.key === key
        ? { key, dir: cur.dir === "asc" ? "desc" : "asc" }
        : { key, dir: columns.find((c) => c.key === key)?.descFirst ? "desc" : "asc" },
    );

  return { rows: sorted, sort, toggle };
}
