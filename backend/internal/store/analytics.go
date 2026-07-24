package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/analytics/model"
)

// --- run lifecycle ---

func (s *Store) CreateAnalyticsRun(ctx context.Context, cubeID uuid.UUID, trigger string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.pool.QueryRow(ctx, `
		INSERT INTO analytics_runs (cube_id, trigger, status) VALUES ($1,$2,'running')
		RETURNING id`, cubeID, trigger).Scan(&id)
	return id, err
}

func (s *Store) SetAnalyticsRunFailed(ctx context.Context, runID uuid.UUID) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE analytics_runs SET status='failed', finished_at=now() WHERE id=$1`, runID)
	return err
}

// --- loading source data ---

func (s *Store) LoadDecksForAnalytics(ctx context.Context, cubeID uuid.UUID) ([]model.DeckRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, color_identity, splash_colors, played_at, games_played, wins, losses
		FROM decklists WHERE cube_id=$1 AND status IN ('active','archived')
		ORDER BY played_at`, cubeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DeckRow
	for rows.Next() {
		var d model.DeckRow
		if err := rows.Scan(&d.ID, &d.ColorIdent, &d.SplashIdent, &d.PlayedAt,
			&d.Games, &d.Wins, &d.Losses); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) LoadDeckCardsForAnalytics(ctx context.Context, cubeID uuid.UUID) ([]model.DeckCardRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT dc.decklist_id, dc.card_id, c.name, dc.quantity, c.cmc, c.type_line
		FROM decklist_cards dc
		JOIN decklists d ON d.id = dc.decklist_id
		JOIN cards c ON c.scryfall_id = dc.card_id
		WHERE d.cube_id=$1 AND d.status IN ('active','archived')
		  AND dc.is_resolved AND dc.board='main'`, cubeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.DeckCardRow
	for rows.Next() {
		var r model.DeckCardRow
		if err := rows.Scan(&r.DecklistID, &r.CardID, &r.Name, &r.Quantity, &r.CMC, &r.TypeLine); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- finalize ---

// FinalizeAnalyticsRun writes all stat rows, promotes this run to current, and
// marks it ok — all in one transaction so a run never becomes current partially.
func (s *Store) FinalizeAnalyticsRun(ctx context.Context, runID, cubeID uuid.UUID, r *model.Results) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	m := r.Meta
	if _, err := tx.Exec(ctx, `
		INSERT INTO meta_snapshot (run_id, total_decks, total_games, overall_winrate,
			avg_cmc, avg_color_count, mono_share, multi_share, power9_share, undefeated_decks)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		runID, m.TotalDecks, m.TotalGames, m.OverallWinrate, m.AvgCMC, m.AvgColorCount,
		m.MonoShare, m.MultiShare, m.Power9Share, m.UndefeatedDecks); err != nil {
		return err
	}

	for _, c := range r.ColorStats {
		if _, err := tx.Exec(ctx, `
			INSERT INTO color_stats (run_id, facet, facet_key, deck_count, games, wins, losses, winrate)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			runID, c.Facet, c.FacetKey, c.DeckCount, c.Games, c.Wins, c.Losses, c.Winrate); err != nil {
			return err
		}
	}
	for _, t := range r.ColorTrend {
		if _, err := tx.Exec(ctx, `
			INSERT INTO color_trend_stats (run_id, as_of, color, deck_count, total_decks, share)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			runID, t.AsOf, t.Color, t.DeckCount, t.TotalDecks, t.Share); err != nil {
			return err
		}
	}
	for _, c := range r.CardStats {
		if _, err := tx.Exec(ctx, `
			INSERT INTO card_stats (run_id, card_id, deck_count, inclusion_rate, games, wins, losses, winrate)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			runID, c.CardID, c.DeckCount, c.InclusionRate, c.Games, c.Wins, c.Losses,
			c.Winrate); err != nil {
			return err
		}
	}
	for _, p := range r.PairStats {
		if _, err := tx.Exec(ctx, `
			INSERT INTO card_pair_stats (run_id, card_a_id, card_b_id, co_count, pair_winrate)
			VALUES ($1,$2,$3,$4,$5)`,
			runID, p.CardA, p.CardB, p.CoCount, p.PairWinrate); err != nil {
			return err
		}
	}
	for _, d := range r.DeckMetrics {
		if _, err := tx.Exec(ctx, `
			INSERT INTO deck_metric_stats (run_id, metric, bucket, deck_count, winrate)
			VALUES ($1,$2,$3,$4,$5)`,
			runID, d.Metric, d.Bucket, d.DeckCount, d.Winrate); err != nil {
			return err
		}
	}

	if err := promoteRun(ctx, tx, runID, cubeID, r.DecksIncluded, r.GamesIncluded); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FinalizeEmptyRun promotes a zero-deck run with just a zeroed meta snapshot.
func (s *Store) FinalizeEmptyRun(ctx context.Context, runID, cubeID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO meta_snapshot (run_id, total_decks, total_games) VALUES ($1,0,0)`, runID); err != nil {
		return err
	}
	if err := promoteRun(ctx, tx, runID, cubeID, 0, 0); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func promoteRun(ctx context.Context, tx pgx.Tx, runID, cubeID uuid.UUID, decks, games int) error {
	if _, err := tx.Exec(ctx,
		`UPDATE analytics_runs SET is_current=false WHERE cube_id=$1 AND is_current`, cubeID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
		UPDATE analytics_runs SET status='ok', finished_at=now(),
			decks_included=$2, games_included=$3, is_current=true WHERE id=$1`,
		runID, decks, games)
	return err
}

// --- read views (json-tagged for the API) ---

type RunMeta struct {
	ID            uuid.UUID  `json:"id"`
	CubeID        uuid.UUID  `json:"cube_id"`
	Trigger       string     `json:"trigger"`
	Status        string     `json:"status"`
	DecksIncluded int        `json:"decks_included"`
	GamesIncluded int        `json:"games_included"`
	StartedAt     time.Time  `json:"started_at"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
}

func (s *Store) GetCurrentRun(ctx context.Context, cubeID uuid.UUID) (*RunMeta, error) {
	var r RunMeta
	err := s.pool.QueryRow(ctx, `
		SELECT id, cube_id, trigger, status, decks_included, games_included, started_at, finished_at
		FROM analytics_runs WHERE cube_id=$1 AND is_current`, cubeID).Scan(
		&r.ID, &r.CubeID, &r.Trigger, &r.Status, &r.DecksIncluded, &r.GamesIncluded, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, normErr(err)
	}
	return &r, nil
}

type MetaSnapshot struct {
	TotalDecks     int      `json:"total_decks"`
	TotalGames     int      `json:"total_games"`
	OverallWinrate *float64 `json:"overall_winrate"`
	AvgCMC         *float64 `json:"avg_cmc"`
	AvgColorCount  *float64 `json:"avg_color_count"`
	MonoShare      *float64 `json:"mono_share"`
	MultiShare     *float64 `json:"multi_share"`
	// Share of decks running at least one of the Power Nine, and how many decks
	// have played at least one game without losing any.
	Power9Share     *float64 `json:"power9_share"`
	UndefeatedDecks int      `json:"undefeated_decks"`
}

func (s *Store) GetMetaSnapshot(ctx context.Context, runID uuid.UUID) (*MetaSnapshot, error) {
	var m MetaSnapshot
	err := s.pool.QueryRow(ctx, `
		SELECT total_decks, total_games, overall_winrate, avg_cmc, avg_color_count, mono_share,
			multi_share, power9_share, undefeated_decks
		FROM meta_snapshot WHERE run_id=$1`, runID).Scan(
		&m.TotalDecks, &m.TotalGames, &m.OverallWinrate, &m.AvgCMC, &m.AvgColorCount, &m.MonoShare,
		&m.MultiShare, &m.Power9Share, &m.UndefeatedDecks)
	if err != nil {
		return nil, normErr(err)
	}
	return &m, nil
}

type ColorStat struct {
	Facet     string   `json:"facet"`
	FacetKey  int      `json:"facet_key"`
	DeckCount int      `json:"deck_count"`
	Games     int      `json:"games"`
	Wins      int      `json:"wins"`
	Losses    int      `json:"losses"`
	Winrate   *float64 `json:"winrate"`
}

func (s *Store) ListColorStats(ctx context.Context, runID uuid.UUID, facet string) ([]ColorStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT facet, facet_key, deck_count, games, wins, losses, winrate
		FROM color_stats WHERE run_id=$1 AND ($2='' OR facet=$2)
		ORDER BY facet, facet_key`, runID, facet)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Non-nil so an empty result encodes as [] rather than null — a JSON null here
	// crashes the clients that index straight into the array.
	out := []ColorStat{}
	for rows.Next() {
		var c ColorStat
		if err := rows.Scan(&c.Facet, &c.FacetKey, &c.DeckCount, &c.Games, &c.Wins, &c.Losses,
			&c.Winrate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ColorTrendColor is one color's standing on one day of the trend.
type ColorTrendColor struct {
	Color     int      `json:"color"` // a single WUBRG bit
	DeckCount int      `json:"deck_count"`
	Share     *float64 `json:"share"` // 0..1 of that day's color pie; null on a day with no colored decks
}

// ColorTrendPoint is one day of the color trend: every color, always in WUBRG order,
// so a client can stack them without sorting or filling gaps.
//
// AsOf is a plain "2006-01-02". The column is a DATE and the day is the whole of the
// meaning; serving a timestamp would hand every client west of UTC the previous day
// the moment it parsed it.
type ColorTrendPoint struct {
	AsOf       string            `json:"as_of"`
	TotalDecks int               `json:"total_decks"`
	Colors     []ColorTrendColor `json:"colors"`
}

// ListColorTrend returns the run's color trend, one point per day, oldest first.
func (s *Store) ListColorTrend(ctx context.Context, runID uuid.UUID) ([]ColorTrendPoint, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT as_of, color, deck_count, total_decks, share
		FROM color_trend_stats WHERE run_id=$1
		ORDER BY as_of, color`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// Non-nil so an empty trend encodes as [] rather than null.
	out := []ColorTrendPoint{}
	for rows.Next() {
		var day time.Time
		var c ColorTrendColor
		var total int
		if err := rows.Scan(&day, &c.Color, &c.DeckCount, &total, &c.Share); err != nil {
			return nil, err
		}
		key := day.Format("2006-01-02")
		// Rows arrive grouped by day (ORDER BY as_of), so only the last point can match.
		if n := len(out); n > 0 && out[n-1].AsOf == key {
			out[n-1].Colors = append(out[n-1].Colors, c)
			continue
		}
		out = append(out, ColorTrendPoint{AsOf: key, TotalDecks: total, Colors: []ColorTrendColor{c}})
	}
	return out, rows.Err()
}

type CardStat struct {
	CardID        uuid.UUID `json:"card_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	ImageNormal   *string   `json:"image_normal,omitempty"`
	ImageArtCrop  *string   `json:"image_art_crop,omitempty"`
	ColorIdentity int       `json:"color_identity"`
	DeckCount     int       `json:"deck_count"`
	InclusionRate float64   `json:"inclusion_rate"`
	Games         int       `json:"games"`
	Wins          int       `json:"wins"`
	Winrate       *float64  `json:"winrate"`
}

func (s *Store) ListCardStats(ctx context.Context, runID uuid.UUID, limit int) ([]CardStat, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `
		SELECT cs.card_id, c.name, c.slug, c.image_normal, c.image_art_crop, c.color_identity,
			cs.deck_count, cs.inclusion_rate, cs.games, cs.wins, cs.winrate
		FROM card_stats cs JOIN cards c ON c.scryfall_id = cs.card_id
		WHERE cs.run_id=$1 ORDER BY cs.inclusion_rate DESC NULLS LAST LIMIT $2`, runID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CardStat{}
	for rows.Next() {
		var c CardStat
		if err := rows.Scan(&c.CardID, &c.Name, &c.Slug, &c.ImageNormal, &c.ImageArtCrop, &c.ColorIdentity,
			&c.DeckCount, &c.InclusionRate, &c.Games, &c.Wins, &c.Winrate); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// GetCardStat returns one card's row in a run, or ErrNotFound when the card is
// in no analyzed deck (e.g. it sits in the pool but nobody has drafted it).
func (s *Store) GetCardStat(ctx context.Context, runID, cardID uuid.UUID) (*CardStat, error) {
	var c CardStat
	err := s.pool.QueryRow(ctx, `
		SELECT cs.card_id, c.name, c.slug, c.image_normal, c.image_art_crop, c.color_identity,
			cs.deck_count, cs.inclusion_rate, cs.games, cs.wins, cs.winrate
		FROM card_stats cs JOIN cards c ON c.scryfall_id = cs.card_id
		WHERE cs.run_id=$1 AND cs.card_id=$2`, runID, cardID).Scan(
		&c.CardID, &c.Name, &c.Slug, &c.ImageNormal, &c.ImageArtCrop, &c.ColorIdentity,
		&c.DeckCount, &c.InclusionRate, &c.Games, &c.Wins, &c.Winrate)
	if err != nil {
		return nil, normErr(err)
	}
	return &c, nil
}

// CardInclusionRank ranks a card by popularity within its run: 1-based position
// by inclusion_rate, plus the size of the ranked field.
func (s *Store) CardInclusionRank(ctx context.Context, runID uuid.UUID, inclusionRate float64) (rank, total int, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE inclusion_rate > $2) + 1, count(*)
		FROM card_stats WHERE run_id=$1`, runID, inclusionRate).Scan(&rank, &total)
	return rank, total, err
}

type CardPair struct {
	CardBID     uuid.UUID `json:"card_b_id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	ColorIdent  int       `json:"color_identity"`
	CoCount     int       `json:"co_count"`
	PairWinrate *float64  `json:"pair_winrate"`
}

// ListCardPairs returns the cards most often played alongside cardID, most-shared
// first. Name breaks ties so the list is stable across runs.
func (s *Store) ListCardPairs(ctx context.Context, runID, cardID uuid.UUID, limit int) ([]CardPair, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ps.card_b_id, c.name, c.slug, c.color_identity, ps.co_count, ps.pair_winrate
		FROM card_pair_stats ps JOIN cards c ON c.scryfall_id = ps.card_b_id
		WHERE ps.run_id=$1 AND ps.card_a_id=$2
		ORDER BY ps.co_count DESC, c.name LIMIT $3`, runID, cardID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// A card with no co-occurring pairs is the common case in a young meta. Returning
	// nil here encodes as JSON null and blows up every client that reads .length.
	out := []CardPair{}
	for rows.Next() {
		var p CardPair
		if err := rows.Scan(&p.CardBID, &p.Name, &p.Slug, &p.ColorIdent,
			&p.CoCount, &p.PairWinrate); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
