// A hoverable "i" that explains a stat. Pure CSS (see .info-hint in globals.css), so
// it works inside the async server components that render the stats tables — no client
// bundle for a tooltip. tabIndex makes it reachable without a pointer; the bubble shows
// on :hover and :focus-visible alike.
export function InfoHint({ text }: { text: string }) {
  return (
    <span className="info-hint" tabIndex={0} role="note" aria-label={text}>
      <span className="info-mark" aria-hidden="true">
        i
      </span>
      <span className="info-bubble">{text}</span>
    </span>
  );
}
