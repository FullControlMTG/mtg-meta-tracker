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

// LookupCubeCardsByName resolves card names against a cube's active pool,
// case-insensitively. The returned map is keyed by lower(name).
func (s *Store) LookupCubeCardsByName(ctx context.Context, cubeID uuid.UUID, names []string) (map[string]domain.Card, error) {
	out := make(map[string]domain.Card)
	if len(names) == 0 {
		return out, nil
	}
	lowered := make([]string, len(names))
	for i, n := range names {
		lowered[i] = strings.ToLower(n)
	}
	rows, err := s.pool.Query(ctx, `
		SELECT c.scryfall_id, c.name, c.cmc, c.color_identity
		FROM cards c
		JOIN cube_cards cc ON cc.card_id = c.scryfall_id
		WHERE cc.cube_id = $1 AND cc.is_active AND lower(c.name) = ANY($2::text[])`,
		cubeID, lowered)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Card
		if err := rows.Scan(&c.ScryfallID, &c.Name, &c.CMC, &c.ColorIdentity); err != nil {
			return nil, err
		}
		out[strings.ToLower(c.Name)] = c
	}
	return out, rows.Err()
}

// Absent cards are soft-removed (not deleted) so old decklists still resolve.
func (s *Store) SyncCubeCards(ctx context.Context, cubeID uuid.UUID, activeIDs []uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
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
		return err
	}

	for _, id := range activeIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO cube_cards (cube_id, card_id, is_active, added_at, removed_at)
			VALUES ($1,$2,true, now(), NULL)
			ON CONFLICT (cube_id, card_id)
			DO UPDATE SET is_active=true, removed_at=NULL`,
			cubeID, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
