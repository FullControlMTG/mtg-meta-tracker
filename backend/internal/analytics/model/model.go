// Package model holds the plain data structures shared between the store (which
// loads and persists them) and the analytics engine (which computes them),
// kept in a leaf package to avoid a store<->analytics import cycle.
package model

import (
	"time"

	"github.com/google/uuid"
)

// DeckRow is a decklist loaded for analytics (record fields + colors). ColorIdent
// is the deck's real colors and SplashIdent the ones it only splashes; they are
// disjoint, and only the former feeds the color facets. PlayedAt is the day the
// deck was played, and the only time axis the analytics have.
type DeckRow struct {
	ID          uuid.UUID
	ColorIdent  int
	SplashIdent int
	PlayedAt    time.Time
	Games       int
	Wins        int
	Losses      int
}

// DeckCardRow is one resolved main-board card belonging to a deck. TypeLine is
// carried so the engine can exclude lands from mana-value averages and basics
// from the card breakdown; the filtering lives in aggregate, not in SQL.
type DeckCardRow struct {
	DecklistID uuid.UUID
	CardID     uuid.UUID
	Quantity   int
	CMC        *float64
	TypeLine   *string
}

type ColorStatRow struct {
	Facet     string
	FacetKey  int
	DeckCount int
	Games     int
	Wins      int
	Losses    int
	Winrate   *float64
}

// ColorTrendRow is one color's standing on one day: how many decks played it as of
// then, out of how many decks existed, and its slice of that day's color pie.
type ColorTrendRow struct {
	AsOf       time.Time
	Color      int
	DeckCount  int
	TotalDecks int
	Share      *float64
}

type CardStatRow struct {
	CardID        uuid.UUID
	DeckCount     int
	InclusionRate float64
	Games         int
	Wins          int
	Losses        int
	Winrate       *float64
}

type PairStatRow struct {
	CardA       uuid.UUID
	CardB       uuid.UUID
	CoCount     int
	PairWinrate *float64
}

type MetaSnapshotRow struct {
	TotalDecks     int
	TotalGames     int
	OverallWinrate *float64
	AvgCMC         *float64
	AvgColorCount  *float64
	MonoShare      *float64
	MultiShare     *float64
}

type DeckMetricRow struct {
	Metric    string
	Bucket    string
	DeckCount int
	Winrate   *float64
}

// Results is the full output of one recompute, ready to persist.
type Results struct {
	ColorStats    []ColorStatRow
	ColorTrend    []ColorTrendRow
	CardStats     []CardStatRow
	PairStats     []PairStatRow
	Meta          MetaSnapshotRow
	DeckMetrics   []DeckMetricRow
	DecksIncluded int
	GamesIncluded int
}
