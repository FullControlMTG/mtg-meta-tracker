export function pct(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return "—";
  return (v * 100).toFixed(digits) + "%";
}

export function num(v: number | null | undefined, digits = 2): string {
  if (v === null || v === undefined) return "—";
  return v.toFixed(digits);
}

const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];

// The calendar day out of an API date, which arrives as "2026-07-24T00:00:00Z" — a DATE
// column, so the midnight is filler and only the day means anything.
//
// Read off the string rather than through `new Date`: that midnight is UTC, so a browser
// west of Greenwich renders the day before, and the month names are fixed rather than
// locale-dependent so the server and the client agree and the markup rehydrates.
export function isoDay(v: string | null | undefined): string {
  return v ? v.slice(0, 10) : "";
}

export function fmtDate(v: string | null | undefined): string {
  const day = isoDay(v);
  const [y, m, d] = day.split("-");
  const month = MONTHS[Number(m) - 1];
  if (!month || !y || !d) return "—";
  return `${month} ${Number(d)}, ${y}`;
}
