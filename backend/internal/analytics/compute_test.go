package analytics

import (
	"math"
	"testing"
	"time"

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

// A splashed color is not one of the deck's colors: it is counted on the splash
// facet and nowhere else, so a WU deck splashing black stays a two-color WU deck
// in the meta rather than becoming a three-color one.
func TestAggregateSplashFacet(t *testing.T) {
	deck := uuid.New()
	decks := []model.DeckRow{
		{ID: deck, ColorIdent: 3, SplashIdent: 4, Games: 2, Wins: 1, Losses: 1},
	}
	r := aggregate(decks, nil)

	facets := map[string]map[int]model.ColorStatRow{}
	for _, c := range r.ColorStats {
		if facets[c.Facet] == nil {
			facets[c.Facet] = map[int]model.ColorStatRow{}
		}
		facets[c.Facet][c.FacetKey] = c
	}
	if b := facets["splash_color"][4]; b.DeckCount != 1 || b.Winrate == nil || *b.Winrate != 0.5 {
		t.Fatalf("splash_color B: deck_count=%d winrate=%v", b.DeckCount, b.Winrate)
	}
	if _, ok := facets["splash_color"][1]; ok {
		t.Fatal("W is one of the deck's colors, not a splash")
	}
	if b, ok := facets["single_color"][4]; ok {
		t.Fatalf("splashed B leaked into single_color: %+v", b)
	}
	if _, ok := facets["exact_identity"][7]; ok {
		t.Fatal("splashed B leaked into exact_identity (WUB)")
	}
	if cc := facets["color_count"][3]; cc.DeckCount != 0 {
		t.Fatalf("splashed B made the deck three colors: %+v", cc)
	}
	if cc := facets["color_count"][2]; cc.DeckCount != 1 {
		t.Fatalf("deck should count as two colors, got %+v", cc)
	}
	if r.Meta.AvgColorCount == nil || *r.Meta.AvgColorCount != 2 {
		t.Fatalf("avg_color_count = %v, want 2", r.Meta.AvgColorCount)
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

// Power Nine is a name match, and undefeated means "played and never lost" — a deck
// with no games recorded has not gone undefeated.
func TestAggregatePowerNineAndUndefeated(t *testing.T) {
	perfect, beaten, unplayed := uuid.New(), uuid.New(), uuid.New()
	decks := []model.DeckRow{
		{ID: perfect, ColorIdent: 2, Games: 3, Wins: 3, Losses: 0},
		{ID: beaten, ColorIdent: 8, Games: 3, Wins: 1, Losses: 2},
		{ID: unplayed, ColorIdent: 1, Games: 0},
	}
	cards := []model.DeckCardRow{
		{DecklistID: perfect, CardID: uuid.New(), Name: "Black Lotus", Quantity: 1, CMC: cmc(0)},
		{DecklistID: perfect, CardID: uuid.New(), Name: "Brainstorm", Quantity: 1, CMC: cmc(1)},
		{DecklistID: beaten, CardID: uuid.New(), Name: "Lightning Bolt", Quantity: 1, CMC: cmc(1)},
		// Case-insensitive: the name comes from Scryfall, but the match must not be
		// the thing that breaks if a printing ever spells it differently.
		{DecklistID: unplayed, CardID: uuid.New(), Name: "mox pearl", Quantity: 1, CMC: cmc(0)},
	}
	r := aggregate(decks, cards)

	if r.Meta.UndefeatedDecks != 1 {
		t.Errorf("undefeated_decks = %d, want 1 (the 3-0; the 0-game deck has not played)",
			r.Meta.UndefeatedDecks)
	}
	if r.Meta.Power9Share == nil || math.Abs(*r.Meta.Power9Share-2.0/3) > 1e-9 {
		t.Errorf("power9_share = %v, want 2/3", r.Meta.Power9Share)
	}
}

func TestAggregateEmpty(t *testing.T) {
	r := aggregate(nil, nil)
	if r.DecksIncluded != 0 || r.Meta.OverallWinrate != nil {
		t.Fatalf("empty aggregate should have nil winrate and 0 decks")
	}
	if len(r.ColorTrend) != 0 {
		t.Errorf("empty aggregate should have no trend points, got %d", len(r.ColorTrend))
	}
}

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// Three decks over two days: a mono-W and a UB on the 1st, then a WU on the 3rd. The
// 2nd is absent — nothing was played, so nothing changed.
func TestColorTrendIsCumulativeAndStacksTo100(t *testing.T) {
	decks := []model.DeckRow{
		{ID: uuid.New(), ColorIdent: 1, PlayedAt: day("2026-07-01")},     // W
		{ID: uuid.New(), ColorIdent: 2 | 4, PlayedAt: day("2026-07-01")}, // UB
		{ID: uuid.New(), ColorIdent: 1 | 2, PlayedAt: day("2026-07-03")}, // WU
	}
	trend := aggregate(decks, nil).ColorTrend

	byDay := map[string]map[int]model.ColorTrendRow{}
	for _, row := range trend {
		k := row.AsOf.Format("2006-01-02")
		if byDay[k] == nil {
			byDay[k] = map[int]model.ColorTrendRow{}
		}
		byDay[k][row.Color] = row
	}
	if len(byDay) != 2 {
		t.Fatalf("got %d days, want 2 (only days a deck was played)", len(byDay))
	}
	// Every color on every day, zeros included: an area band needs a point at each x.
	for k, day := range byDay {
		if len(day) != 5 {
			t.Errorf("%s has %d colors, want all 5", k, len(day))
		}
		var sum float64
		for _, row := range day {
			if row.Share != nil {
				sum += *row.Share
			}
		}
		if math.Abs(sum-1) > 1e-9 {
			t.Errorf("%s shares sum to %v, want 1", k, sum)
		}
	}

	// Day one: W=1, U=1, B=1 of a 3-slice pie; two decks.
	d1 := byDay["2026-07-01"]
	if d1[1].DeckCount != 1 || d1[2].DeckCount != 1 || d1[4].DeckCount != 1 {
		t.Errorf("day 1 counts: W=%d U=%d B=%d, want 1/1/1",
			d1[1].DeckCount, d1[2].DeckCount, d1[4].DeckCount)
	}
	if d1[8].DeckCount != 0 || d1[16].DeckCount != 0 {
		t.Errorf("day 1: R and G should be 0, got %d/%d", d1[8].DeckCount, d1[16].DeckCount)
	}
	if d1[1].TotalDecks != 2 {
		t.Errorf("day 1 total_decks = %d, want 2", d1[1].TotalDecks)
	}
	if s := *d1[1].Share; math.Abs(s-1.0/3) > 1e-9 {
		t.Errorf("day 1 W share = %v, want 1/3", s)
	}

	// Day three carries day one forward and adds the WU: W=2, U=2, B=1 of 5.
	d3 := byDay["2026-07-03"]
	if d3[1].DeckCount != 2 || d3[2].DeckCount != 2 || d3[4].DeckCount != 1 {
		t.Errorf("day 3 counts: W=%d U=%d B=%d, want 2/2/1",
			d3[1].DeckCount, d3[2].DeckCount, d3[4].DeckCount)
	}
	if d3[1].TotalDecks != 3 {
		t.Errorf("day 3 total_decks = %d, want 3", d3[1].TotalDecks)
	}
	if s := *d3[1].Share; math.Abs(s-0.4) > 1e-9 {
		t.Errorf("day 3 W share = %v, want 0.4 (2 of 5 color slots)", s)
	}
}

// A colorless deck counts toward the deck total but has no slice of the pie, and a day
// with nothing but colorless decks has no pie at all.
func TestColorTrendColorlessOnly(t *testing.T) {
	decks := []model.DeckRow{{ID: uuid.New(), ColorIdent: 0, PlayedAt: day("2026-07-01")}}
	trend := aggregate(decks, nil).ColorTrend

	if len(trend) != 5 {
		t.Fatalf("got %d rows, want 5 (one day, five colors)", len(trend))
	}
	for _, row := range trend {
		if row.DeckCount != 0 {
			t.Errorf("color %d: count %d, want 0", row.Color, row.DeckCount)
		}
		if row.Share != nil {
			t.Errorf("color %d: share %v, want null on an empty pie", row.Color, *row.Share)
		}
		if row.TotalDecks != 1 {
			t.Errorf("color %d: total_decks %d, want 1", row.Color, row.TotalDecks)
		}
	}
}

// Splashes are not the deck's colors and must stay out of the trend, exactly as they
// stay out of the single_color facet.
func TestColorTrendExcludesSplashes(t *testing.T) {
	decks := []model.DeckRow{
		{ID: uuid.New(), ColorIdent: 1 | 2, SplashIdent: 16, PlayedAt: day("2026-07-01")},
	}
	for _, row := range aggregate(decks, nil).ColorTrend {
		if row.Color == 16 && row.DeckCount != 0 {
			t.Errorf("splashed green should not appear in the trend, got %d", row.DeckCount)
		}
	}
}
