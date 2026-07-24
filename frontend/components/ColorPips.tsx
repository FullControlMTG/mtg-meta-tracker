import { identityColors, identityString } from "@/lib/colors";

// Stacked mana pips for a deck's color identity, with the letter code beside.
//
// `splash` are the colors the deck only splashes (see domain.InferDeckColors).
// They are not the deck's colors, so they render smaller and faded, after the real
// ones, and lowercase in the code: a Selesnya deck with two red cards reads "WG r",
// not "WRG".
export function ColorPips({
  bits,
  splash = 0,
  showCode = false,
}: {
  bits: number;
  splash?: number;
  showCode?: boolean;
}) {
  const cs = identityColors(bits);
  const ss = splash === 0 ? [] : identityColors(splash);
  return (
    // Wraps rather than sets a floor: in a narrow table cell the code drops under the
    // pips instead of forcing the column wider than the card it sits in. The pips
    // themselves stay on one line — they are the identity, and half of WUBRG is a lie.
    <span
      style={{ display: "inline-flex", alignItems: "center", flexWrap: "wrap", gap: 4 }}
    >
      <span style={{ display: "inline-flex", alignItems: "center", gap: 3 }}>
        {cs.map((c) => (
          <span
            key={c.code}
            title={c.name}
            style={{
              width: 13,
              height: 13,
              borderRadius: "50%",
              background: c.hex,
              border: "1px solid var(--pip-ring)",
              display: "inline-block",
            }}
          />
        ))}
        {ss.map((c) => (
          <span
            key={c.code}
            title={`${c.name} (splash)`}
            style={{
              width: 9,
              height: 9,
              borderRadius: "50%",
              background: c.hex,
              border: "1px solid var(--pip-ring)",
              display: "inline-block",
              opacity: 0.55,
            }}
          />
        ))}
      </span>
      {showCode && (
        <span style={{ fontSize: "0.85rem", fontVariantNumeric: "tabular-nums" }}>
          {cs.map((c) => c.code).join("")}
          {ss.length > 0 && (
            <span className="muted"> {identityString(splash).toLowerCase()}</span>
          )}
        </span>
      )}
    </span>
  );
}
