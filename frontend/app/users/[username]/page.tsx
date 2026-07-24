import { notFound } from "next/navigation";
import {
  apiGetOptional,
  type Decklist,
  type DecklistListItem,
  type PublicUser,
} from "@/lib/api";
import { COLORS, colorCount, identityString } from "@/lib/colors";
import { UNDEFEATED_TERMS, deckListHref, quoteTerm } from "@/lib/deckQuery";
import { num, pct } from "@/lib/format";
import { ColorPips } from "@/components/ColorPips";
import { ColorWheelGrid } from "@/components/ColorWheelGrid";
import { DeckTable } from "@/components/DeckTable";
import { RadarChart, type RadarAxis } from "@/components/RadarChart";
import { StatTile } from "@/components/StatTile";

export const revalidate = 3600;

// One player's meta, computed here from their own decklists rather than from the
// analytics snapshots: those are per-cube aggregates over everybody, and a player
// plays across cubes. The list a player's page already loads carries every field
// this needs (colors, splashes, record), so there is nothing to fetch for it.

// Share of the player's decks on an axis. Null with no decks, so pct() renders an
// em dash rather than NaN.
const share = (count: number, total: number) => (total > 0 ? count / total : null);

// One radar axis per WUBRG color, over whichever bitset the caller picks — a deck's
// real colors or the ones it only splashes. Splashes are not a deck's colors, so the
// two are counted separately and never on the same chart.
function colorAxes(
  decks: Decklist[],
  bits: (d: Decklist) => number,
  verb: string,
): RadarAxis[] {
  return COLORS.map((c) => {
    const n = decks.filter((d) => (bits(d) & c.bit) !== 0).length;
    const s = share(n, decks.length);
    return {
      key: c.code,
      label: c.name,
      value: n,
      hex: c.hex,
      share: s,
      note: `${pct(s, 0)} of their decks ${verb} ${c.name}`,
    };
  });
}

interface Combo {
  bits: number;
  decks: number;
  games: number;
  wins: number;
}

// The player's exact color combinations, most-built first — splashes disregarded, so a
// Selesnya deck with two red cards is one more Selesnya deck rather than a Naya one.
function combos(decks: Decklist[]): Combo[] {
  const by = new Map<number, Combo>();
  for (const d of decks) {
    const c = by.get(d.color_identity) ?? {
      bits: d.color_identity,
      decks: 0,
      games: 0,
      wins: 0,
    };
    c.decks++;
    c.games += d.games_played;
    c.wins += d.wins;
    by.set(c.bits, c);
  }
  // Deck count first; a tie goes to the combination with more games behind it, so the
  // one they actually played breaks ahead of the one they only built.
  return [...by.values()].sort((a, b) => b.decks - a.decks || b.games - a.games);
}

export default async function UserPage({ params }: { params: { username: string } }) {
  const user = await apiGetOptional<PublicUser>(`/users/${params.username}`, 3600);
  if (!user) notFound();

  const items =
    (await apiGetOptional<DecklistListItem[]>(`/decklists?user=${user.id}`, 3600)) ?? [];
  const decks = items.map((i) => i.decklist);

  const games = decks.reduce((n, d) => n + d.games_played, 0);
  const wins = decks.reduce((n, d) => n + d.wins, 0);
  const losses = decks.reduce((n, d) => n + d.losses, 0);
  const undefeated = decks.filter((d) => d.games_played > 0 && d.losses === 0).length;
  const avgColors = decks.length
    ? decks.reduce((n, d) => n + colorCount(d.color_identity), 0) / decks.length
    : null;
  const splashing = decks.filter((d) => d.splash_colors !== 0).length;
  const ranked = combos(decks);
  const top = ranked[0];

  return (
    <main className="container">
      <h1>{user.display_name}</h1>
      <p className="muted">
        @{user.username}
        {user.role === "admin" && (
          <span className="pill" style={{ marginLeft: 8 }}>
            admin
          </span>
        )}
      </p>
      {user.bio && <p>{user.bio}</p>}

      {decks.length === 0 ? (
        <p className="muted" style={{ marginTop: "1.5rem" }}>
          No decklists yet.
        </p>
      ) : (
        <>
          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gap: "0.6rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(125px, 1fr))",
            }}
          >
            <StatTile value={String(decks.length)} label="Decks built" />
            <StatTile value={String(games)} label="Matches played" />
            <StatTile value={`${wins}-${losses}`} label="Record (W-L)" />
            <StatTile value={pct(games > 0 ? wins / games : null)} label="Winrate" />
            {/* Reads the same as the cube's tile, so it links the same way — their
                undefeated decks, best record first. */}
            <StatTile
              value={String(undefeated)}
              label="Undefeated decks"
              href={deckListHref(
                [`user:${quoteTerm(user.username)}`, ...UNDEFEATED_TERMS],
                { key: "record", dir: "desc" },
              )}
            />
            <StatTile value={num(avgColors, 1)} label="Avg. colors per deck" />
            {/* The headline answer to "what do they play" — the pips are in the
                combinations card below, where there is room for a legible row. */}
            <StatTile
              value={top ? identityString(top.bits) : "—"}
              label="Most played colors"
            />
            <StatTile
              value={pct(share(splashing, decks.length), 0)}
              label="Decks with a splash"
            />
          </div>

          <div
            className="grid"
            style={{
              marginTop: "1.5rem",
              gridTemplateColumns: "repeat(auto-fit, minmax(320px, 1fr))",
            }}
          >
            <section className="card">
              <h2>Colors Played</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                How often they build on each color.
              </p>
              <RadarChart
                axes={colorAxes(decks, (d) => d.color_identity, "play")}
                caption={`Decks ${user.display_name} has built in each color of the WUBRG pie`}
              />
            </section>

            <section className="card">
              <h2>Colors Splashed</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Which colors they reach for rather than build on.
              </p>
              <RadarChart
                axes={colorAxes(decks, (d) => d.splash_colors, "splash")}
                caption={`Decks ${user.display_name} has splashed each color in`}
              />
            </section>

            <section className="card">
              <h2>Color Combinations</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Every combination they could build.
              </p>
              <ColorWheelGrid bitsets={decks.map((d) => d.color_identity)} />
            </section>

            <section className="card">
              <h2>Combinations</h2>
              <p className="muted" style={{ marginTop: "-0.25rem" }}>
                Top 5 played color combinations
              </p>
              {/* .table-compact spends the outer cells' padding and the number columns'
                  slack on the colors column, which is what gets the four columns of
                  pips, codes and percentages inside the 320px a card gets at the grid's
                  minimum — no sideways scroll, and none needed on a phone either. */}
              <table className="table-compact">
                <thead>
                  <tr>
                    <th>Colors</th>
                    <th>Decks</th>
                    <th>Share</th>
                    <th>Winrate</th>
                  </tr>
                </thead>
                <tbody>
                  {ranked.slice(0, 5).map((c) => (
                    <tr key={c.bits}>
                      <td>
                        <ColorPips bits={c.bits} showCode />
                      </td>
                      <td>{c.decks}</td>
                      <td>{pct(share(c.decks, decks.length), 0)}</td>
                      <td>{pct(c.games > 0 ? c.wins / c.games : null)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </section>
          </div>

          {/* No syncUrl: this page's address is the player, not a deck query. */}
          <DeckTable decks={items} heading={<h2 style={{ marginTop: "1.5rem" }}>Decklists</h2>} />
        </>
      )}
    </main>
  );
}
