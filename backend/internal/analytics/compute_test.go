package analytics

import (
	"math"
	"testing"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/analytics/model"
)

func cmc(v float64) *float64 { return &v }

// Scenario: two decks over the same two cards A,B. Deck1 (mono-W) wins both
// games; Deck2 (WU) loses both.
func buildScenario() ([]model.DeckRow, []model.DeckCardRow, uuid.UUID, uuid.UUID) {
	d1 := uuid.New()
	d2 := uuid.New()
	cardA := uuid.New()
	cardB := uuid.New()
	decks := []model.DeckRow{
		{ID: d1, ColorIdent: 1, Games: 2, Wins: 2, Losses: 0},
		{ID: d2, ColorIdent: 3, Games: 2, Wins: 0, Losses: 2},
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
	if ab.CoCount != 2 || ba.CoCount != 2 {
		t.Fatalf("co_count should be 2 both ways, got AB=%d BA=%d", ab.CoCount, ba.CoCount)
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
	// exact_identity mono-W is only the deck that won both games.
	if e := facets["exact_identity"][1]; e.DeckCount != 1 || e.Winrate == nil || *e.Winrate != 1 {
		t.Fatalf("exact W: deck_count=%d winrate=%v", e.DeckCount, e.Winrate)
	}
}

func typeLine(v string) *string { return &v }

// A realistic deck is mostly basics by copy count. They must not reach the card
// breakdown (every deck plays Island, so it would top inclusion and co-occur with
// everything), and no land may drag the mana-value average.
func TestAggregateExcludesBasicsAndLands(t *testing.T) {
	deck := uuid.New()
	island, wasteland, bolt, jace := uuid.New(), uuid.New(), uuid.New(), uuid.New()

	decks := []model.DeckRow{{ID: deck, ColorIdent: 2, Games: 2, Wins: 1, Losses: 1}}
	cards := []model.DeckCardRow{
		{DecklistID: deck, CardID: island, Quantity: 17, CMC: cmc(0), TypeLine: typeLine("Basic Land — Island")},
		{DecklistID: deck, CardID: wasteland, Quantity: 1, CMC: cmc(0), TypeLine: typeLine("Land")},
		{DecklistID: deck, CardID: bolt, Quantity: 1, CMC: cmc(1), TypeLine: typeLine("Instant")},
		{DecklistID: deck, CardID: jace, Quantity: 1, CMC: cmc(4), TypeLine: typeLine("Legendary Planeswalker — Jace")},
	}
	r := aggregate(decks, cards)

	byCard := map[uuid.UUID]model.CardStatRow{}
	for _, c := range r.CardStats {
		byCard[c.CardID] = c
	}
	if _, ok := byCard[island]; ok {
		t.Error("basic land must not appear in card_stats")
	}
	// A nonbasic land is a real card choice — it stays in the breakdown.
	if _, ok := byCard[wasteland]; !ok {
		t.Error("nonbasic land should appear in card_stats")
	}
	if len(byCard) != 3 {
		t.Errorf("card_stats has %d cards, want 3 (all but the basic)", len(byCard))
	}
	for _, p := range r.PairStats {
		if p.CardA == island || p.CardB == island {
			t.Error("basic land must not appear in card_pair_stats")
		}
	}

	// Mana value averages the two nonlands only: (1 + 4) / 2 = 2.5. Counting the 18
	// lands at MV 0 would give 5/20 = 0.25.
	if r.Meta.AvgCMC == nil || math.Abs(*r.Meta.AvgCMC-2.5) > 1e-9 {
		t.Fatalf("avg_cmc = %v, want 2.5 (nonlands only)", r.Meta.AvgCMC)
	}
}

func TestAggregateEmpty(t *testing.T) {
	r := aggregate(nil, nil)
	if r.DecksIncluded != 0 || r.Meta.OverallWinrate != nil {
		t.Fatalf("empty aggregate should have nil winrate and 0 decks")
	}
}
