export function pct(v: number | null | undefined, digits = 1): string {
  if (v === null || v === undefined) return "—";
  return (v * 100).toFixed(digits) + "%";
}

export function num(v: number | null | undefined, digits = 2): string {
  if (v === null || v === undefined) return "—";
  return v.toFixed(digits);
}
