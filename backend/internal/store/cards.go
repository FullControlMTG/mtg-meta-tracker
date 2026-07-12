package store

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

func (s *Store) UpsertCard(ctx context.Context, c *domain.Card) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO cards (scryfall_id, oracle_id, name, mana_cost, cmc, type_line,
			oracle_text, colors, color_identity, rarity, layout,
			image_small, image_normal, image_art_crop, set_code, collector_number, raw, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17, now())
		ON CONFLICT (scryfall_id) DO UPDATE SET
			oracle_id=EXCLUDED.oracle_id, name=EXCLUDED.name, mana_cost=EXCLUDED.mana_cost,
			cmc=EXCLUDED.cmc, type_line=EXCLUDED.type_line, oracle_text=EXCLUDED.oracle_text,
			colors=EXCLUDED.colors, color_identity=EXCLUDED.color_identity, rarity=EXCLUDED.rarity,
			layout=EXCLUDED.layout, image_small=EXCLUDED.image_small, image_normal=EXCLUDED.image_normal,
			image_art_crop=EXCLUDED.image_art_crop, set_code=EXCLUDED.set_code,
			collector_number=EXCLUDED.collector_number, raw=EXCLUDED.raw, updated_at=now()`,
		c.ScryfallID, c.OracleID, c.Name, c.ManaCost, c.CMC, c.TypeLine, c.OracleText,
		c.Colors, c.ColorIdentity, c.Rarity, c.Layout, c.ImageSmall, c.ImageNormal,
		c.ImageArtCrop, c.SetCode, c.CollectorNumber, []byte(c.Raw))
	return err
}

// GetCardImageURL returns the canonical Scryfall image URL for a card and
// variant (one of "small", "normal", "art_crop"). Returns ErrNotFound when the
// card is unknown or that variant has no URL.
func (s *Store) GetCardImageURL(ctx context.Context, id uuid.UUID, variant string) (string, error) {
	var small, normal, artCrop *string
	err := s.pool.QueryRow(ctx,
		`SELECT image_small, image_normal, image_art_crop FROM cards WHERE scryfall_id=$1`, id).
		Scan(&small, &normal, &artCrop)
	if err != nil {
		return "", normErr(err)
	}
	var url *string
	switch variant {
	case "small":
		url = small
	case "art_crop":
		url = artCrop
	default: // "normal"
		url = normal
	}
	if url == nil || *url == "" {
		return "", ErrNotFound
	}
	return *url, nil
}

// LookupCubeCardsByName resolves card names against a cube's active pool,
// case-insensitively.
//
// A card answers to more than one name: a double-faced card is stored under its
// canonical "Front // Back" but decklists write "Front / Back" or just "Front",
// and a Universes Beyond printing is stored under its real name while the list
// writes the flavor name ("White Tower of Ecthelion" for Karakas). Both the SQL
// predicate and the returned map therefore work over every alias, so a caller
// can probe with whatever the list wrote. Keying only on the canonical name is
// what made DFC deck cards miss the pool and re-resolve to a different printing.
func (s *Store) LookupCubeCardsByName(ctx context.Context, cubeID uuid.UUID, names []string) (map[string]domain.Card, error) {
	out := make(map[string]domain.Card)
	if len(names) == 0 {
		return out, nil
	}
	lowered := make([]string, 0, len(names)*2)
	for _, n := range names {
		lowered = append(lowered, strings.ToLower(n), strings.ToLower(FrontFace(n)))
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.scryfall_id, c.name, c.cmc, c.color_identity, c.type_line,
			c.raw->>'flavor_name',`+castColorCol+`
		FROM cards c
		JOIN cube_cards cc ON cc.card_id = c.scryfall_id
		WHERE cc.cube_id = $1 AND cc.is_active AND (
		      lower(c.name) = ANY($2::text[])
		   OR lower(split_part(c.name, ' // ', 1)) = ANY($2::text[])
		   OR lower(c.raw->>'flavor_name') = ANY($2::text[]))`,
		cubeID, lowered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Card
		var flavor *string
		var colors []string
		if err := rows.Scan(&c.ScryfallID, &c.Name, &c.CMC, &c.ColorIdentity, &c.TypeLine,
			&flavor, &colors); err != nil {
			return nil, err
		}
		c.Colors = int(domain.ParseColorIdentity(colors))
		out[strings.ToLower(c.Name)] = c
		out[strings.ToLower(FrontFace(c.Name))] = c
		if flavor != nil && *flavor != "" {
			out[strings.ToLower(*flavor)] = c
			out[strings.ToLower(FrontFace(*flavor))] = c
		}
	}
	return out, rows.Err()
}

// FrontFace reduces "Front // Back" or "Front / Back" to "Front". Decklists and
// Scryfall disagree on how many slashes a double-faced card gets, so every name
// comparison has to be able to normalize both.
func FrontFace(name string) string {
	if i := strings.IndexByte(name, '/'); i >= 0 {
		return strings.TrimSpace(name[:i])
	}
	return name
}

// CubeCardView enriches a cube's active card with cached Scryfall fields for display.
type CubeCardView struct {
	ScryfallID    uuid.UUID `json:"card_id"`
	Name          string    `json:"card_name"`
	Slug          string    `json:"slug"`
	ManaCost      *string   `json:"mana_cost,omitempty"`
	CMC           *float64  `json:"cmc,omitempty"`
	TypeLine      *string   `json:"type_line,omitempty"`
	ColorIdentity int       `json:"color_identity"`
	// The section the card displays under; see domain.GroupColors.
	GroupColors  int     `json:"group_colors"`
	ImageNormal  *string `json:"image_normal,omitempty"`
	ImageArtCrop *string `json:"image_art_crop,omitempty"`
	// The exact printing, as resolved from the cube list. Together they address
	// the card on Scryfall (/card/{set}/{collector}); null on a row synced before
	// printings were resolved.
	SetCode         *string `json:"set_code,omitempty"`
	CollectorNumber *string `json:"collector_number,omitempty"`
}

// ListCubeCards returns a cube's active cards with the Scryfall fields the
// public cube page renders. Ordered so callers can group by color identity.
func (s *Store) ListCubeCards(ctx context.Context, cubeID uuid.UUID) ([]CubeCardView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT c.scryfall_id, c.name, c.slug, c.mana_cost, c.cmc, c.type_line,
			c.color_identity, c.image_normal, c.image_art_crop,
			c.set_code, c.collector_number,`+groupColorCols+`
		FROM cards c
		JOIN cube_cards cc ON cc.card_id = c.scryfall_id
		WHERE cc.cube_id = $1 AND cc.is_active
		ORDER BY c.color_identity, lower(c.name)`, cubeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CubeCardView{}
	for rows.Next() {
		var v CubeCardView
		var g groupColorInputs
		if err := rows.Scan(&v.ScryfallID, &v.Name, &v.Slug, &v.ManaCost, &v.CMC, &v.TypeLine,
			&v.ColorIdentity, &v.ImageNormal, &v.ImageArtCrop,
			&v.SetCode, &v.CollectorNumber,
			&g.colors, &g.oracleText, &g.produced); err != nil {
			return nil, err
		}
		v.GroupColors = int(g.resolve(v.TypeLine, v.ColorIdentity))
		out = append(out, v)
	}
	return out, rows.Err()
}

// The three columns domain.GroupColors needs beyond what a card view already
// carries, as SELECTed by groupColorCols. Kept off the views themselves: the
// oracle text of a whole cube would dwarf the rest of the payload, to say nothing
// of the raw Scryfall blob two of these are extracted from.
type groupColorInputs struct {
	colors     []string
	oracleText *string
	produced   []string
}

// The colors of a card's casting cost, for a `cards` row aliased `c` that may be
// all-NULL (an unresolved deck card LEFT JOINs to nothing).
//
// Read from `raw` rather than the `colors` column because Scryfall omits top-level
// `colors` on a double-faced card — they live per face — so the column is 0 for
// every DFC written before that was handled at ingest, and reading it would file
// them all under Colorless. Fall back to the union of the faces'.
// jsonb_array_elements is strict, so a card with no `card_faces` yields no rows
// there and lands on the empty array.
const castColorCol = `
	coalesce(
		c.raw -> 'colors',
		(SELECT jsonb_agg(col)
		   FROM jsonb_array_elements(c.raw -> 'card_faces') AS f,
		        jsonb_array_elements(f -> 'colors') AS col),
		'[]'::jsonb)`

// The three columns domain.GroupColors needs beyond a card view's own fields.
const groupColorCols = castColorCol + `,
	c.oracle_text,
	coalesce(c.raw -> 'produced_mana', '[]'::jsonb)`

func (g groupColorInputs) resolve(typeLine *string, identity int) domain.ColorIdentity {
	return domain.GroupColors(
		deref(typeLine), deref(g.oracleText),
		domain.ParseColorIdentity(g.colors), domain.ColorIdentity(identity), g.produced)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// CardView is the full public view of a cached card, for the /cards/<slug> page.
type CardView struct {
	ScryfallID    uuid.UUID `json:"card_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	ManaCost      *string   `json:"mana_cost,omitempty"`
	CMC           *float64  `json:"cmc,omitempty"`
	TypeLine      *string   `json:"type_line,omitempty"`
	OracleText    *string   `json:"oracle_text,omitempty"`
	ColorIdentity int       `json:"color_identity"`
	Rarity        *string   `json:"rarity,omitempty"`
	ImageNormal   *string   `json:"image_normal,omitempty"`
	ImageArtCrop  *string   `json:"image_art_crop,omitempty"`
}

// GetCardBySlug resolves a URL slug to a card. Slugs are not unique by
// construction — two printings of a name are two `cards` rows sharing a slug — so
// prefer the printing that is in this cube's active pool, then the most recently
// updated. Also reports whether the chosen card is in that pool.
func (s *Store) GetCardBySlug(ctx context.Context, cubeID uuid.UUID, slug string) (*CardView, bool, error) {
	var c CardView
	var inPool bool
	err := s.pool.QueryRow(ctx, `
		SELECT c.scryfall_id, c.name, c.slug, c.mana_cost, c.cmc, c.type_line, c.oracle_text,
			c.color_identity, c.rarity, c.image_normal, c.image_art_crop,
			cc.card_id IS NOT NULL AS in_pool
		FROM cards c
		LEFT JOIN cube_cards cc
			ON cc.card_id = c.scryfall_id AND cc.cube_id = $1 AND cc.is_active
		WHERE c.slug = $2
		ORDER BY in_pool DESC, c.updated_at DESC
		LIMIT 1`, cubeID, slug).Scan(
		&c.ScryfallID, &c.Name, &c.Slug, &c.ManaCost, &c.CMC, &c.TypeLine, &c.OracleText,
		&c.ColorIdentity, &c.Rarity, &c.ImageNormal, &c.ImageArtCrop, &inPool)
	if err != nil {
		return nil, false, normErr(err)
	}
	return &c, inPool, nil
}

// DeckBrief is a decklist as listed on a card's page.
type DeckBrief struct {
	ID            uuid.UUID `json:"id"`
	Name          string    `json:"name"`
	ColorIdentity int       `json:"color_identity"`
	SplashColors  int       `json:"splash_colors"`
	Quantity      int       `json:"quantity"`
	GamesPlayed   int       `json:"games_played"`
	Wins          int       `json:"wins"`
	Losses        int       `json:"losses"`
	Winrate       *float64  `json:"winrate"`
	Owner         *string   `json:"owner,omitempty"`
}

// ListDecksWithCard returns the analyzed decks in a cube whose main board plays a
// card. Computed live rather than snapshotted: a card sits in a handful of decks,
// so this is cheap and always fresh even between analytics runs.
func (s *Store) ListDecksWithCard(ctx context.Context, cubeID, cardID uuid.UUID) ([]DeckBrief, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT d.id, d.name, d.color_identity, d.splash_colors, dc.quantity,
			d.games_played, d.wins, d.losses, d.winrate, u.username
		FROM decklist_cards dc
		JOIN decklists d ON d.id = dc.decklist_id
		LEFT JOIN users u ON u.id = d.user_id
		WHERE d.cube_id=$1 AND dc.card_id=$2
		  AND d.status IN ('active','archived') AND dc.is_resolved AND dc.board='main'
		ORDER BY d.games_played DESC, d.created_at DESC`, cubeID, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DeckBrief{}
	for rows.Next() {
		var d DeckBrief
		if err := rows.Scan(&d.ID, &d.Name, &d.ColorIdentity, &d.SplashColors, &d.Quantity,
			&d.GamesPlayed, &d.Wins, &d.Losses, &d.Winrate, &d.Owner); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Absent cards are soft-removed (not deleted) so old decklists still resolve.
// Returns the number of active rows the cube actually holds once committed, so
// the caller can check that against what it asked for rather than assuming.
func (s *Store) SyncCubeCards(ctx context.Context, cubeID uuid.UUID, activeIDs []uuid.UUID) (int, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	idStrs := make([]string, len(activeIDs))
	for i, id := range activeIDs {
		idStrs[i] = id.String()
	}

	if _, err := tx.Exec(ctx, `
		UPDATE cube_cards SET is_active=false, removed_at=now()
		WHERE cube_id=$1 AND is_active AND NOT (card_id = ANY($2::uuid[]))`,
		cubeID, idStrs); err != nil {
		return 0, err
	}

	for _, id := range activeIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cube_cards (cube_id, card_id, is_active, added_at, removed_at)
			VALUES ($1,$2,true, now(), NULL)
			ON CONFLICT (cube_id, card_id)
			DO UPDATE SET is_active=true, removed_at=NULL`,
			cubeID, id); err != nil {
			return 0, err
		}
	}

	var active int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM cube_cards WHERE cube_id=$1 AND is_active`, cubeID).
		Scan(&active); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return active, nil
}
