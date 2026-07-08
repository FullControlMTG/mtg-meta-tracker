package analytics

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/analytics/model"
)

func cmc(v float64) *float64 { return &v }
func placement(v int) *int   { return &v }

// Scenario: two decks over the same two cards A,B. Deck1 (mono-W) wins both
// games; Deck2 (WU) loses both.
func buildScenario() ([]model.DeckRow, []model.DeckCardRow, uuid.UUID, uuid.UUID) {
	d1 := uuid.New()
	d2 := uuid.New()
	cardA := uuid.New()
	cardB := uuid.New()
	decks := []model.DeckRow{
		{ID: d1, ColorIdent: 1, Games: 2, Wins: 2, Losses: 0, Draws: 0, Placement: placement(1)},
		{ID: d2, ColorIdent: 3, Games: 2, Wins: 0, Losses: 2, Draws: 0, Placement: placement(2)},
	}
	cards := []model.DeckCardRow{
		{DecklistID: d1, CardID: cardA, Quantity: 1, CMC: cmc(1)},
		{DecklistID: d1, CardID: cardB, Quantity: 1, CMC: cmc(3)},
		{DecklistID: d2, CardID: cardA, Quantity: 1, CMC: cmc(1)},
		{DecklistID: d2, CardID: cardB, Quantity: 1, CMC: cmc(3)},
	}
	return decks, cards, cardA, cardB
}

func TestAggregateMeta(t *testing.T) {
	decks, cards, _, _ := buildScenario()
	r := aggregate(decks, cards)

	if r.DecksIncluded != 2 || r.GamesIncluded != 4 {
		t.Fatalf("counts: decks=%d games=%d", r.DecksIncluded, r.GamesIncluded)
	}
	if r.Meta.OverallWinrate == nil || *r.Meta.OverallWinrate != 0.5 {
		t.Fatalf("overall winrate = %v, want 0.5", r.Meta.OverallWinrate)
	}
	if r.Meta.AvgCMC == nil || math.Abs(*r.Meta.AvgCMC-2.0) > 1e-9 {
		t.Fatalf("avg_cmc = %v, want 2.0", r.Meta.AvgCMC)
	}
	if r.Meta.MonoShare == nil || *r.Meta.MonoShare != 0.5 {
		t.Fatalf("mono_share = %v, want 0.5", r.Meta.MonoShare)
	}
	if r.Meta.MultiShare == nil || *r.Meta.MultiShare != 0.5 {
		t.Fatalf("multi_share = %v, want 0.5", r.Meta.MultiShare)
	}
	if r.Meta.AvgColorCount == nil || *r.Meta.AvgColorCount != 1.5 {
		t.Fatalf("avg_color_count = %v, want 1.5", r.Meta.AvgColorCount)
	}
}

func TestAggregateCardStats(t *testing.T) {
	decks, cards, cardA, _ := buildScenario()
	r := aggregate(decks, cards)

	var a *model.CardStatRow
	for i := range r.CardStats {
		if r.CardStats[i].CardID == cardA {
			a = &r.CardStats[i]
		}
	}
	if a == nil {
		t.Fatal("card A missing from card_stats")
	}
	if a.DeckCount != 2 || a.InclusionRate != 1.0 {
		t.Fatalf("card A deck_count=%d inclusion=%v", a.DeckCount, a.InclusionRate)
	}
	if a.Winrate == nil || *a.Winrate != 0.5 {
		t.Fatalf("card A winrate=%v want 0.5", a.Winrate)
	}
	// At the global mean, shrunk winrate == mean and lift == 0.
	if a.WinrateShrunk == nil || math.Abs(*a.WinrateShrunk-0.5) > 1e-9 {
		t.Fatalf("card A shrunk=%v want 0.5", a.WinrateShrunk)
	}
	if a.WinrateLift == nil || math.Abs(*a.WinrateLift) > 1e-9 {
		t.Fatalf("card A lift=%v want 0", a.WinrateLift)
	}
	if a.WilsonLower == nil {
		t.Fatal("card A wilson_lower should be set")
	}
}

func TestAggregatePairsBothDirections(t *testing.T) {
	decks, cards, cardA, cardB := buildScenario()
	r := aggregate(decks, cards)

	if len(r.PairStats) != 2 {
		t.Fatalf("expected 2 directional pair rows, got %d", len(r.PairStats))
	}
	var ab, ba *model.PairStatRow
	for i := range r.PairStats {
		p := &r.PairStats[i]
		if p.CardA == cardA && p.CardB == cardB {
			ab = p
		}
		if p.CardA == cardB && p.CardB == cardA {
			ba = p
		}
	}
	if ab == nil || ba == nil {
		t.Fatal("both pair directions must be present")
	}
	if ab.CoCount != 2 || ab.Support != 1.0 || ab.Lift != 1.0 {
		t.Fatalf("pair AB: co=%d support=%v lift=%v", ab.CoCount, ab.Support, ab.Lift)
	}
	if ab.ConfidenceAB != 1.0 || ba.ConfidenceAB != 1.0 {
		t.Fatalf("confidence should be 1.0 both ways")
	}
	if ab.PairWinrate == nil || *ab.PairWinrate != 0.5 {
		t.Fatalf("pair winrate=%v want 0.5", ab.PairWinrate)
	}
}

func TestAggregateColorFacets(t *testing.T) {
	decks, cards, _, _ := buildScenario()
	r := aggregate(decks, cards)

	facets := map[string]map[int]model.ColorStatRow{}
	for _, c := range r.ColorStats {
		if facets[c.Facet] == nil {
			facets[c.Facet] = map[int]model.ColorStatRow{}
		}
		facets[c.Facet][c.FacetKey] = c
	}
	// single_color W (bit 1) appears in both decks.
	if w := facets["single_color"][1]; w.DeckCount != 2 || w.Winrate == nil || *w.Winrate != 0.5 {
		t.Fatalf("single_color W: deck_count=%d winrate=%v", w.DeckCount, w.Winrate)
	}
	// single_color U (bit 2) only in the WU deck (0 wins).
	if u := facets["single_color"][2]; u.DeckCount != 1 || u.Winrate == nil || *u.Winrate != 0 {
		t.Fatalf("single_color U: deck_count=%d winrate=%v", u.DeckCount, u.Winrate)
	}
	// exact_identity mono-W deck has avg placement 1.
	if e := facets["exact_identity"][1]; e.AvgPlacement == nil || *e.AvgPlacement != 1 {
		t.Fatalf("exact W avg_placement=%v want 1", e.AvgPlacement)
	}
}

func TestAggregateEmpty(t *testing.T) {
	r := aggregate(nil, nil)
	if r.DecksIncluded != 0 || r.Meta.OverallWinrate != nil {
		t.Fatalf("empty aggregate should have nil winrate and 0 decks")
	}
}
