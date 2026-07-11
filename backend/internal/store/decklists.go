package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

const decklistCols = `id, cube_id, user_id, name, description, color_identity, archetype,
	source_url, decklist_raw, card_count, status,
	games_played, wins, losses, draws, placement, event_name, played_at, record_updated_at,
	winrate, created_at, updated_at`

func scanDecklist(row pgx.Row) (*domain.Decklist, error) {
	var d domain.Decklist
	err := row.Scan(&d.ID, &d.CubeID, &d.UserID, &d.Name, &d.Description, &d.ColorIdentity,
		&d.Archetype, &d.SourceURL, &d.DecklistRaw, &d.CardCount, &d.Status,
		&d.GamesPlayed, &d.Wins, &d.Losses, &d.Draws, &d.Placement, &d.EventName,
		&d.PlayedAt, &d.RecordUpdatedAt, &d.Winrate, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &d, nil
}

// DecklistFilter narrows a decklist listing. Zero-value fields are ignored.
type DecklistFilter struct {
	CubeID *uuid.UUID
	UserID *uuid.UUID
}

// ListCubeDecklistIDs returns the IDs of every decklist in a cube. Used to
// target on-demand revalidation of the affected deck detail pages.
func (s *Store) ListCubeDecklistIDs(ctx context.Context, cubeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM decklists WHERE cube_id=$1`, cubeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CreateDecklist inserts the decklist and its cards in one transaction.
func (s *Store) CreateDecklist(ctx context.Context, d *domain.Decklist, cards []domain.DecklistCard) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO decklists (cube_id, user_id, name, description, color_identity, archetype,
			source_url, decklist_raw, card_count, status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, winrate, created_at, updated_at`,
		d.CubeID, d.UserID, d.Name, d.Description, d.ColorIdentity, d.Archetype,
		d.SourceURL, d.DecklistRaw, d.CardCount, d.Status,
	).Scan(&d.ID, &d.Winrate, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return err
	}
	if err := insertDecklistCards(ctx, tx, d.ID, cards); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertDecklistCards(ctx context.Context, tx pgx.Tx, decklistID uuid.UUID, cards []domain.DecklistCard) error {
	for _, c := range cards {
		if _, err := tx.Exec(ctx, `
			INSERT INTO decklist_cards (decklist_id, card_id, card_name, quantity, is_resolved, board)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (decklist_id, card_name, board)
			DO UPDATE SET card_id=EXCLUDED.card_id, quantity=EXCLUDED.quantity, is_resolved=EXCLUDED.is_resolved`,
			decklistID, c.CardID, c.CardName, c.Quantity, c.IsResolved, c.Board); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetDecklist(ctx context.Context, id uuid.UUID) (*domain.Decklist, error) {
	return scanDecklist(s.pool.QueryRow(ctx, `SELECT `+decklistCols+` FROM decklists WHERE id=$1`, id))
}

func (s *Store) ListDecklists(ctx context.Context, f DecklistFilter) ([]domain.Decklist, error) {
	q := `SELECT ` + decklistCols + ` FROM decklists WHERE ($1::uuid IS NULL OR cube_id=$1)
		AND ($2::uuid IS NULL OR user_id=$2) ORDER BY created_at DESC`
	rows, err := s.pool.Query(ctx, q, f.CubeID, f.UserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Decklist
	for rows.Next() {
		d, err := scanDecklist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// DecklistCardView enriches a deck card with cached Scryfall fields for display.
// Slug is empty for an unresolved card (there is no `cards` row to link to).
type DecklistCardView struct {
	domain.DecklistCard
	Slug         *string  `json:"slug,omitempty"`
	ImageArtCrop *string  `json:"image_art_crop,omitempty"`
	ImageNormal  *string  `json:"image_normal,omitempty"`
	CMC          *float64 `json:"cmc,omitempty"`
	TypeLine     *string  `json:"type_line,omitempty"`
}

func (s *Store) GetDecklistCardsView(ctx context.Context, decklistID uuid.UUID) ([]DecklistCardView, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dc.decklist_id, dc.card_id, dc.card_name, dc.quantity, dc.is_resolved, dc.board,
			c.slug, c.image_art_crop, c.image_normal, c.cmc, c.type_line
		FROM decklist_cards dc
		LEFT JOIN cards c ON c.scryfall_id = dc.card_id
		WHERE dc.decklist_id=$1 ORDER BY dc.board, dc.card_name`, decklistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DecklistCardView{}
	for rows.Next() {
		var v DecklistCardView
		if err := rows.Scan(&v.DecklistID, &v.CardID, &v.CardName, &v.Quantity, &v.IsResolved, &v.Board,
			&v.Slug, &v.ImageArtCrop, &v.ImageNormal, &v.CMC, &v.TypeLine); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetDecklistCards(ctx context.Context, decklistID uuid.UUID) ([]domain.DecklistCard, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT decklist_id, card_id, card_name, quantity, is_resolved, board
		FROM decklist_cards WHERE decklist_id=$1 ORDER BY board, card_name`, decklistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DecklistCard
	for rows.Next() {
		var c domain.DecklistCard
		if err := rows.Scan(&c.DecklistID, &c.CardID, &c.CardName, &c.Quantity, &c.IsResolved, &c.Board); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// UpdateDecklist rewrites metadata; when cards is non-nil the card rows are
// fully replaced (used when the raw list changed).
func (s *Store) UpdateDecklist(ctx context.Context, d *domain.Decklist, cards []domain.DecklistCard) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ct, err := tx.Exec(ctx, `
		UPDATE decklists SET name=$2, description=$3, color_identity=$4, archetype=$5,
			source_url=$6, decklist_raw=$7, card_count=$8, status=$9, updated_at=now()
		WHERE id=$1`,
		d.ID, d.Name, d.Description, d.ColorIdentity, d.Archetype,
		d.SourceURL, d.DecklistRaw, d.CardCount, d.Status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	if cards != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM decklist_cards WHERE decklist_id=$1`, d.ID); err != nil {
			return err
		}
		if err := insertDecklistCards(ctx, tx, d.ID, cards); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// DecklistRecord is the win/loss record patch payload.
type DecklistRecord struct {
	GamesPlayed int
	Wins        int
	Losses      int
	Draws       int
	Placement   *int
	EventName   *string
	PlayedAt    *time.Time
}

func (s *Store) UpdateDecklistRecord(ctx context.Context, id uuid.UUID, rec DecklistRecord) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE decklists SET games_played=$2, wins=$3, losses=$4, draws=$5,
			placement=$6, event_name=$7, played_at=$8, record_updated_at=now(), updated_at=now()
		WHERE id=$1`,
		id, rec.GamesPlayed, rec.Wins, rec.Losses, rec.Draws,
		rec.Placement, rec.EventName, rec.PlayedAt)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteDecklist(ctx context.Context, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM decklists WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
