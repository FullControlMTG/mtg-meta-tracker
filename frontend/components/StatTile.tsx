// A single number with its name under it. These sit in a row above the charts and
// are read at a glance, so they stay short: the value is a headline, not a display
// face, and the tile is only as tall as the two lines need.
export function StatTile({ value, label }: { value: string; label: string }) {
  return (
    <div className="card tile">
      <div className="tile-value">{value}</div>
      <div className="tile-label">{label}</div>
    </div>
  );
}
