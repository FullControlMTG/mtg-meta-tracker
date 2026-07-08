import Image from "next/image";

const CARD_W = 260;
const CARD_H = 362; // Scryfall "normal" is ~488x680 (ratio 0.717).
const PEEK = 40; // visible sliver (title line) per stacked card

// Minimal shape shared by DecklistCard and CubeCard. is_resolved is optional —
// cube cards are always resolved, so treat a missing flag as resolved.
type FanCard = { card_name: string; image_normal?: string; is_resolved?: boolean };

// Vertical overlaid fan: cards stacked with ~90% overlap so only each card's
// title line peeks; hovering slides one out. Uses Scryfall "normal" images.
export function CardFan({ cards }: { cards: FanCard[] }) {
  const withArt = cards.filter((c) => c.is_resolved !== false && c.image_normal);
  if (withArt.length === 0) return null;

  return (
    <div style={{ width: CARD_W, position: "relative" }}>
      {withArt.map((c, i) => (
        <div
          key={c.card_name}
          style={{ marginTop: i === 0 ? 0 : -(CARD_H - PEEK), position: "relative", zIndex: i }}
        >
          <Image
            className="fan-card"
            src={c.image_normal as string}
            alt={c.card_name}
            width={CARD_W}
            height={CARD_H}
          />
        </div>
      ))}
    </div>
  );
}
