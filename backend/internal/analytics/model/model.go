// Package model holds the plain data structures shared between the store (which
// loads and persists them) and the analytics engine (which computes them),
// kept in a leaf package to avoid a store<->analytics import cycle.
package model

import "github.com/google/uuid"

// DeckRow is a decklist loaded for analytics (record fields + color identity).
type DeckRow struct {
	ID         uuid.UUID
	ColorIdent int
	Games      int
	Wins       int
	Losses     int
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

type CardStatRow struct {
	CardID        uuid.UUID
	DeckCount     int
	InclusionRate float64
	Games         int
	Wins          int
	Losses        int
	Winrate       *float64
	WinrateShrunk *float64
	WinrateLift   *float64
	WilsonLower   *float64
}

type PairStatRow struct {
	CardA        uuid.UUID
	CardB        uuid.UUID
	CoCount      int
	Support      float64
	ConfidenceAB float64
	Lift         float64
	PairWinrate  *float64
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
	CardStats     []CardStatRow
	PairStats     []PairStatRow
	Meta          MetaSnapshotRow
	DeckMetrics   []DeckMetricRow
	DecksIncluded int
	GamesIncluded int
}
