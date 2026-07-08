import { identityColors } from "@/lib/colors";

// Stacked mana pips for a deck's color identity, with the letter code beside.
export function ColorPips({ bits, showCode = false }: { bits: number; showCode?: boolean }) {
  const cs = identityColors(bits);
  return (
    <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
      <span style={{ display: "inline-flex", gap: 3 }}>
        {cs.map((c) => (
          <span
            key={c.code}
            title={c.name}
            style={{
              width: 13,
              height: 13,
              borderRadius: "50%",
              background: c.hex,
              border: "1px solid rgba(0,0,0,0.25)",
              display: "inline-block",
            }}
          />
        ))}
      </span>
      {showCode && (
        <span style={{ fontSize: "0.85rem", fontVariantNumeric: "tabular-nums" }}>
          {cs.map((c) => c.code).join("")}
        </span>
      )}
    </span>
  );
}
