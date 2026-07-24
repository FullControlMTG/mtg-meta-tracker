# Goals and scope

## What the project does

Tracks the metagame of one local MTG cube playgroup:

- An admin registers a cube and pastes its card list. The list is resolved to
  exact Scryfall printings and stored as the cube's pool.
- Players upload decks built from that pool, and record games played / wins /
  losses per deck.
- The backend infers each deck's colors from the casting costs of its nonland
  cards, splitting off sub-threshold colors as splashes.
- An admin can register named combos — sets of cards that play together — and
  every deck holding all the pieces reports them, naming a sub-archetype the
  colors and the archetype tag cannot.
- An analytics engine recomputes snapshots per cube: color share (four facets),
  color share over time, card popularity and record, card co-occurrence, and
  deck-level metrics.
- The frontend renders those as a dashboard, plus browsable cube pools, deck
  pages, card pages, and user profiles.

## Who it is for

One playgroup, self-hosted. Deployed at `cube.fullcontrolmtg.com` behind Traefik.

Accounts are created by an admin; there is no public signup. Read endpoints accept
an anonymous caller, so the site is publicly readable but only members can write.

Scale is tens of decks and a handful of players. That figure drives design
decisions throughout — see the non-goals below.

## Current priorities

Phases 0–5 of the build plan are complete and Phase 6 (polish) is partly done
(see [IMPLEMENTATION.md](IMPLEMENTATION.md)). Open items, in order of value:

1. **Per-game (or per-match) results.** The highest-value item left. Records are
   currently per deck, so a card's winrate is its deck's winrate. Recording
   results per game is what would make a defensible card-power signal possible at
   all.
2. Test coverage for `store/` and `httpapi/` — currently untested, because all
   existing tests are pure units that need no database.
3. Frontend tests — there are none.
4. Wire up the frontend `lint` script, which is presently a no-op.
5. Google OAuth (OIDC). The `oauth_accounts` table and `GOOGLE_*` config exist; no
   route does.

## Non-goals

**Derived statistics on per-deck data.** Bayesian-shrunk winrates, lift over the
global mean, Wilson lower bounds, and association-rule support/confidence/lift for
card pairs were all implemented and then deliberately removed in commit `b483fbb`.
Do not reintroduce them, and do not reach for them when asked to make the
analytics "more rigorous". The reason is the input, not the math: a card's "games"
are its deck's games attributed wholesale to all ~40 cards, and no estimator
downstream repairs that attribution — it only dresses one soft number up as
several. The fix is better input (priority 1 above).

**Head-to-head / matchup statistics.** Out of scope for the same reason: the data
records a deck's aggregate record, not who it played.

**Fetching cube lists from the Moxfield API.** Moxfield blocks the app. Pools come
from pasted lists. `internal/moxfield` survives only to parse a `publicId` out of
a URL for display.

**Multi-tenancy.** The app supports multiple cubes, but one deployment serves one
playgroup. There is no billing, no org model, and no tenant isolation.

**Hotlinking card images.** Images are downloaded and self-hosted from
`/api/cards/{id}/image`.

**An ORM, a CSS framework, or a chart library.** See CLAUDE.md conventions.
