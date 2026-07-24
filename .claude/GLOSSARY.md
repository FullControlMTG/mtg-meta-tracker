# Glossary

## Magic: The Gathering terms

**Cube** — a hand-curated pool of cards, usually singleton, that a group drafts or
builds decks from repeatedly. This app tracks one or more cubes; a cube's pool is
the set of cards its decks may use.

**WUBRG** — the five colors of Magic, in their canonical order: White, Blue (U),
Black, Red, Green. The app's bitset, its display order, and its color-pie sorting
all follow it.

**Color identity** — Scryfall's field counting every colored mana symbol a card
*produces or references*, including in rules text. **Not** what this app uses for
deck colors: a Mox Sapphire has a blue color identity and would make a Selesnya
deck read as blue.

**Casting cost colors** — Scryfall's `colors` field: the colors in a card's mana
cost. This *is* what deck colors are derived from.

**Splash** — a color a deck plays only incidentally. Here: a color appearing on
fewer than 10% of a deck's nonland cards (`domain.SplashThreshold`).

**Mana value / CMC** — the total cost of a card, ignoring color. Scryfall's `cmc`.

**DFC (double-faced card)** — a card with two faces. Subtypes matter to color
inference:
- *transform* — the back has no mana cost and is turned up, not cast. Its colors
  do **not** count toward a deck's colors.
- *modal DFC* — both faces have mana costs and either can be cast. Both count.
- *adventure*, *split* — two castable halves on one face. Both count.

**Mainboard / sideboard / maybeboard** — a decklist's sections. Only the mainboard
(`board = 'main'`) feeds color inference and analytics.

**Basic land** — Plains, Island, Swamp, Mountain, Forest. Excluded from card stats
and pair stats, because every deck plays them.

**Archetype** — a deck's strategic category. Constrained to a fixed enum here:
`aggro`, `control`, `midrange`, `tempo`, `combo`.

**Printing** — one specific publication of a card, addressed by set code plus
collector number. The app resolves lists to exact printings, so two printings of
one name are two `cards` rows.

**Scryfall** — the third-party card database this app resolves names and images
against. Not affiliated with the project.

**Moxfield** — a deck-building site. Its API blocks this app, so cube lists are
pasted rather than fetched; only a `publicId` is kept, for a display link.

## Project-specific terms

**Cast colors** — the app's name for the color bitset derived from casting costs.
Implemented twice: `scryfall.castColors` (Go, at ingest) and `store.castColorCol`
(SQL, for recompute). The two must agree.

**Group colors** — `domain.GroupColors`: the colors a card is *displayed* under,
which is a different question from its cast colors. A land groups by every color
it relates to — what it taps for, the basic types it has or fetches — because the
colors a land cares about are rarely in its cost.

**Facet** — a slicing of `color_stats`. Four exist: `exact_identity` (the 0–31
bitset), `single_color` (one color bit; decks *containing* it), `color_count`
(0–5), and `splash_color` (one color bit; decks *splashing* it). Splashes appear
in the last one and nowhere else.

**Run** — one execution of the analytics engine, a row in `analytics_runs`. Every
stat row is keyed by `run_id`. At most one run per cube is `is_current`; pages
read that one.

**Recompute** — the job (`recompute_analytics`) that re-derives deck colors for a
cube, aggregates every snapshot, writes a run, and promotes it.

**Content hash** — `cubes.content_hash`, a fingerprint of a cube's pasted list
(names, printings, quantities, plus `resolverVersion`). An unchanged hash skips
re-resolution entirely.

**Resolver version** — a constant in `internal/ingest` folded into the content
hash. Bumping it invalidates every stored fingerprint, forcing a re-resolve after
a change to how lists resolve.

**Unresolved** — a pasted name Scryfall could not match. Dropped from the pool but
recorded in `cube_sync_progress.unresolved` and shown on the admin page, so a typo
cannot silently shrink a cube.

**Caller** — `appctx.Caller`, carried on every request `context.Context`: whether
the caller is public or authenticated, their user id, and their role. Authorization
predicates like `CanMutateOwned` read it.

**Bootstrap admin** — the first account, created from the `BOOTSTRAP_ADMIN_*` env
vars. They take effect only while the `users` table is empty. There is no public
signup; that admin creates everyone else.

**Revalidation webhook** — `POST /api/revalidate` on the frontend, called by the
backend with a shared secret after a recompute, triggering `revalidatePath` for
affected pages.

**Card fan** — the frontend's overlaid card-image layout (`CardFan`), used by both
cube pools and deck pages: images stacked with ~81% overlap so each title strip
peeks out, lifting on hover.

**Pip ring** — the `--pip-ring` CSS custom property, an outline applied to any
fill drawn in the MTG palette. White is a near-white and black a near-black, so
without it they disappear into one of the two surfaces.

**Splash threshold** — `domain.SplashThreshold`, 0.10.

**Deck query** — the `field:value` filter language the deck table reads, defined in
`frontend/lib/deckQuery.ts`. Terms are ANDed, `-` negates one, a bare word searches
the deck name. Its `FIELDS` table is the list of what a deck can be filtered by, and
the panel's reference is generated from it. Any page can link into a filtered list
with `/decks?q=…&sort=…&dir=…` (see `deckListHref`).

## Deployment terms

**Traefik** — the reverse proxy fronting production. The frontend container
carries its routing labels.

**Dev compose vs prod compose** — `docker-compose.dev.yml` is the local database
only (published port, named volume). `docker-compose.yml` is the production
deployment (Traefik, a NAS bind mount, no published DB port) and is not usable
locally. The dev file is deliberately not named `docker-compose.override.yml`,
which would silently merge into the production `docker compose up`.
