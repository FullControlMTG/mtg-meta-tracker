export function StatTile({ value, label }: { value: string; label: string }) {
  return (
    <div className="card">
      <div className="tile-value">{value}</div>
      <div className="tile-label">{label}</div>
    </div>
  );
}
