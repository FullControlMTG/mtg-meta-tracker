import Link from "next/link";

// A single number with its name under it. These sit in a row above the charts and
// are read at a glance, so they stay short: the value is a headline, not a display
// face, and the tile is only as tall as the two lines need.
//
// With an href the whole tile becomes a link to the rows behind the number — the deck
// list, filtered to the same decks the tile counted. It keeps the tile's own look
// rather than the accent-blue of a link: a row of tiles where one is blue reads as a
// different kind of thing, and it is not.
export function StatTile({
  value,
  label,
  href,
}: {
  value: string;
  label: string;
  href?: string;
}) {
  const body = (
    <>
      <div className="tile-value">{value}</div>
      <div className="tile-label">{label}</div>
    </>
  );
  if (!href) return <div className="card tile">{body}</div>;
  return (
    <Link href={href} className="card tile tile-link">
      {body}
    </Link>
  );
}
