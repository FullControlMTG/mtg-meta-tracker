package store

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrComboPieceNotInPool reports a piece that is not one of the cube's active
// cards. Combos are configured by picking from the pool, so this is the caller's
// mistake rather than a server fault.
var ErrComboPieceNotInPool = errors.New("combo piece is not in the cube pool")

// ErrComboNameTaken reports a second combo claiming a name already used in the
// same cube (the UNIQUE (cube_id, name) the schema enforces).
var ErrComboNameTaken = errors.New("a combo with that name already exists")

// comboKeyExpr identifies a card across printings, for a `cards` row aliased `c`.
// A combo's pieces name one printing each and a deck resolves its own — the same
// Demonic Consultation from two sets is two `cards` rows — so matching on
// scryfall_id would miss half the decks that actually run the combo. oracle_id is
// the printing-independent identity; it is nullable, so fall back to the name.
const comboKeyExpr = `coalesce(c.oracle_id::text, lower(c.name))`

// The card columns a combo piece carries. Deliberately the shape the frontend's
// FanCard wants, so a piece renders through the same card components as a deck's
// or a cube's cards.
const comboPieceCols = `c.scryfall_id, c.name, c.slug, c.image_normal,
	c.set_code, c.collector_number, c.color_identity`

// ComboPiece is one card of a combo, at the printing it was configured from.
type ComboPiece struct {
	CardID          uuid.UUID `json:"card_id"`
	CardName        string    `json:"card_name"`
	Slug            string    `json:"slug"`
	ImageNormal     *string   `json:"image_normal,omitempty"`
	SetCode         *string   `json:"set_code,omitempty"`
	CollectorNumber *string   `json:"collector_number,omitempty"`
	ColorIdentity   int       `json:"color_identity"`
}

// ComboView is a named combo and the cards that make it.
type ComboView struct {
	ID          uuid.UUID    `json:"id"`
	CubeID      uuid.UUID    `json:"cube_id"`
	Name        string       `json:"name"`
	Description *string      `json:"description,omitempty"`
	Cards       []ComboPiece `json:"cards"`
}

// ComboInput is a combo as an admin configures it. CardIDs are printings from
// the cube's active pool, in the order they should read.
type ComboInput struct {
	Name        string
	Description *string
	CardIDs     []uuid.UUID
}

// scanCombos folds a combo × piece join into one ComboView per combo. Rows must
// arrive grouped by combo and ordered by piece position.
func scanCombos(rows pgx.Rows) ([]ComboView, error) {
	defer rows.Close()
	out := []ComboView{}
	idx := map[uuid.UUID]int{}
	for rows.Next() {
		var c ComboView
		var p ComboPiece
		if err := rows.Scan(&c.ID, &c.CubeID, &c.Name, &c.Description,
			&p.CardID, &p.CardName, &p.Slug, &p.ImageNormal,
			&p.SetCode, &p.CollectorNumber, &p.ColorIdentity); err != nil {
			return nil, err
		}
		i, ok := idx[c.ID]
		if !ok {
			i = len(out)
			idx[c.ID] = i
			out = append(out, c)
		}
		out[i].Cards = append(out[i].Cards, p)
	}
	return out, rows.Err()
}

// The join every combo read shares. A combo with no pieces would fall out of the
// inner join entirely, which writes prevent: CreateCombo and UpdateCombo both
// insist on at least two, inside the transaction that writes the combo row.
const comboSelect = `
	SELECT co.id, co.cube_id, co.name, co.description, ` + comboPieceCols + `
	FROM combos co
	JOIN combo_cards cc ON cc.combo_id = co.id
	JOIN cards c ON c.scryfall_id = cc.card_id`

const comboOrder = ` ORDER BY lower(co.name), cc.position, lower(c.name)`

// ListCombos returns every combo configured for a cube.
func (s *Store) ListCombos(ctx context.Context, cubeID uuid.UUID) ([]ComboView, error) {
	rows, err := s.pool.Query(ctx, comboSelect+` WHERE co.cube_id=$1`+comboOrder, cubeID)
	if err != nil {
		return nil, err
	}
	return scanCombos(rows)
}

func (s *Store) GetCombo(ctx context.Context, id uuid.UUID) (*ComboView, error) {
	rows, err := s.pool.Query(ctx, comboSelect+` WHERE co.id=$1`+comboOrder, id)
	if err != nil {
		return nil, err
	}
	combos, err := scanCombos(rows)
	if err != nil {
		return nil, err
	}
	if len(combos) == 0 {
		return nil, ErrNotFound
	}
	return &combos[0], nil
}

// MatchCombos returns the cube's combos whose every piece is among the given
// cards — the deck's resolved main board, or a list being previewed before it is
// saved. Computed live rather than snapshotted, like ListDecksWithCard: a deck
// holds forty cards and a cube a handful of combos, so this stays cheap and, more
// to the point, an edit to a combo's definition shows up on every deck at once
// instead of waiting for each to be saved again.
func (s *Store) MatchCombos(ctx context.Context, cubeID uuid.UUID, cardIDs []uuid.UUID) ([]ComboView, error) {
	if len(cardIDs) == 0 {
		return []ComboView{}, nil
	}
	idStrs := make([]string, len(cardIDs))
	for i, id := range cardIDs {
		idStrs[i] = id.String()
	}
	rows, err := s.pool.Query(ctx, `
		WITH deck AS (
			SELECT DISTINCT `+comboKeyExpr+` AS card_key
			FROM cards c WHERE c.scryfall_id = ANY($2::uuid[])
		), matched AS (
			-- A combo is matched when the deck covers every one of its pieces:
			-- count(*) is how many pieces it has, count(d.card_key) how many of
			-- them the deck found. deck's keys are DISTINCT, so the left join
			-- cannot multiply a piece into looking like several.
			SELECT cc.combo_id
			FROM combo_cards cc
			JOIN combos co ON co.id = cc.combo_id AND co.cube_id = $1
			JOIN cards c ON c.scryfall_id = cc.card_id
			LEFT JOIN deck d ON d.card_key = `+comboKeyExpr+`
			GROUP BY cc.combo_id
			HAVING count(*) = count(d.card_key)
		)`+comboSelect+`
		JOIN matched m ON m.combo_id = co.id`+comboOrder, cubeID, idStrs)
	if err != nil {
		return nil, err
	}
	return scanCombos(rows)
}

func (s *Store) CreateCombo(ctx context.Context, cubeID uuid.UUID, in ComboInput) (uuid.UUID, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := checkComboPieces(ctx, tx, cubeID, in.CardIDs); err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO combos (cube_id, name, description) VALUES ($1,$2,$3) RETURNING id`,
		cubeID, in.Name, in.Description).Scan(&id)
	if err != nil {
		return uuid.Nil, comboWriteErr(err)
	}
	if err := insertComboCards(ctx, tx, id, in.CardIDs); err != nil {
		return uuid.Nil, err
	}
	return id, tx.Commit(ctx)
}

// UpdateCombo rewrites a combo and replaces its pieces wholesale. The cube it
// belongs to is fixed at creation: its pieces come from that pool, so moving it
// would be a different combo.
func (s *Store) UpdateCombo(ctx context.Context, id uuid.UUID, in ComboInput) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var cubeID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT cube_id FROM combos WHERE id=$1`, id).Scan(&cubeID); err != nil {
		return normErr(err)
	}
	if err := checkComboPieces(ctx, tx, cubeID, in.CardIDs); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE combos SET name=$2, description=$3, updated_at=now() WHERE id=$1`,
		id, in.Name, in.Description); err != nil {
		return comboWriteErr(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM combo_cards WHERE combo_id=$1`, id); err != nil {
		return err
	}
	if err := insertComboCards(ctx, tx, id, in.CardIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteCombo(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM combos WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func insertComboCards(ctx context.Context, tx pgx.Tx, comboID uuid.UUID, cardIDs []uuid.UUID) error {
	for i, cardID := range cardIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO combo_cards (combo_id, card_id, position) VALUES ($1,$2,$3)`,
			comboID, cardID, i); err != nil {
			return err
		}
	}
	return nil
}

// checkComboPieces insists every piece is an active card of the cube. Without it
// a combo could name a card no deck built from this pool can ever play, and the
// admin would have no way to tell it apart from one that simply never comes up.
func checkComboPieces(ctx context.Context, tx pgx.Tx, cubeID uuid.UUID, cardIDs []uuid.UUID) error {
	idStrs := make([]string, len(cardIDs))
	for i, id := range cardIDs {
		idStrs[i] = id.String()
	}
	var n int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM cube_cards
		WHERE cube_id=$1 AND is_active AND card_id = ANY($2::uuid[])`,
		cubeID, idStrs).Scan(&n); err != nil {
		return err
	}
	if n != len(cardIDs) {
		return ErrComboPieceNotInPool
	}
	return nil
}

// comboWriteErr maps the one constraint an admin can trip from the form — two
// combos in a cube sharing a name — onto an error the handler can report as a
// client error rather than a 500.
func comboWriteErr(err error) error {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) && pgErr.SQLState() == "23505" {
		return ErrComboNameTaken
	}
	return err
}
