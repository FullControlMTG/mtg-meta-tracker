package analytics

import (
	"bytes"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/analytics/model"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// Scryfall type lines: "Land — Island", "Basic Land — Forest", "Basic Snow Land — Forest".
func isLand(typeLine *string) bool {
	return typeLine != nil && strings.Contains(*typeLine, "Land")
}

// Basics are in every deck, so they'd top the inclusion-rate list and co-occur
// with everything. They are excluded from card_stats and card_pair_stats.
func isBasicLand(typeLine *string) bool {
	return isLand(typeLine) && strings.Contains(*typeLine, "Basic")
}

// acc accumulates a win/loss record over a group of decks.
type acc struct {
	decks  int
	games  int
	wins   int
	losses int
}

func (a *acc) add(d model.DeckRow) {
	a.decks++
	a.games += d.Games
	a.wins += d.Wins
	a.losses += d.Losses
}

type pairAcc struct {
	coCount int
	games   int
	wins    int
}

// deckCards is one deck's main-board contents, pre-filtered for the two things
// that read it: `set` (nonbasic presence) drives card_stats and card_pair_stats;
// cmcSum/qtySum are accumulated over nonlands only, so avg mana value is the
// conventional nonland average rather than being dragged to ~1.2 by 17 basics.
type deckCards struct {
	set    map[uuid.UUID]struct{}
	cmcSum float64
	qtySum int
}

var singleColorBits = []int{
	int(domain.White), int(domain.Blue), int(domain.Black), int(domain.Red), int(domain.Green),
}

// aggregate computes every analytics snapshot in a single pass. Pure function.
func aggregate(decks []model.DeckRow, cards []model.DeckCardRow) *model.Results {
	res := &model.Results{}

	// Attach cards to decks.
	perDeck := make(map[uuid.UUID]*deckCards, len(decks))
	for _, dc := range cards {
		d := perDeck[dc.DecklistID]
		if d == nil {
			d = &deckCards{set: map[uuid.UUID]struct{}{}}
			perDeck[dc.DecklistID] = d
		}
		if !isBasicLand(dc.TypeLine) {
			d.set[dc.CardID] = struct{}{}
		}
		if dc.CMC != nil && !isLand(dc.TypeLine) {
			d.cmcSum += *dc.CMC * float64(dc.Quantity)
			d.qtySum += dc.Quantity
		}
	}

	var (
		totalDecks    int
		totalGames    int
		totalWins     int
		totalLosses   int
		colorCountSum int
		monoCount     int
		multiCount    int
		cmcDeckSum    float64
		cmcDeckN      int
	)

	exact := map[int]*acc{}
	single := map[int]*acc{}
	countFacet := map[int]*acc{}
	splash := map[int]*acc{}
	cardAgg := map[uuid.UUID]*acc{}
	pairAggs := map[[2]uuid.UUID]*pairAcc{}
	cmcBucket := map[string]*acc{}

	for _, d := range decks {
		totalDecks++
		totalGames += d.Games
		totalWins += d.Wins
		totalLosses += d.Losses

		cc := domain.ColorIdentity(d.ColorIdent).Count()
		colorCountSum += cc
		switch {
		case cc == 1:
			monoCount++
		case cc >= 2:
			multiCount++
		}

		getAcc(exact, d.ColorIdent).add(d)
		for _, bit := range singleColorBits {
			if d.ColorIdent&bit != 0 {
				getAcc(single, bit).add(d)
			}
			// A splashed color is not one of the deck's colors, so it stays out of
			// every facet above and is only counted here.
			if d.SplashIdent&bit != 0 {
				getAcc(splash, bit).add(d)
			}
		}
		getAcc(countFacet, cc).add(d)

		dc := perDeck[d.ID]
		if dc != nil && dc.qtySum > 0 {
			deckAvg := dc.cmcSum / float64(dc.qtySum)
			cmcDeckSum += deckAvg
			cmcDeckN++
			getAcc2(cmcBucket, cmcRange(deckAvg)).add(d)
		}

		if dc != nil && len(dc.set) > 0 {
			ids := sortedIDs(dc.set)
			for _, id := range ids {
				a := cardAgg[id]
				if a == nil {
					a = &acc{}
					cardAgg[id] = a
				}
				a.add(d)
			}
			for i := 0; i < len(ids); i++ {
				for j := i + 1; j < len(ids); j++ {
					key := [2]uuid.UUID{ids[i], ids[j]}
					p := pairAggs[key]
					if p == nil {
						p = &pairAcc{}
						pairAggs[key] = p
					}
					p.coCount++
					p.games += d.Games
					p.wins += d.Wins
				}
			}
		}
	}

	res.DecksIncluded = totalDecks
	res.GamesIncluded = totalGames

	var mu *float64
	if totalGames > 0 {
		m := float64(totalWins) / float64(totalGames)
		mu = &m
	}

	// color_stats
	for facet, m := range map[string]map[int]*acc{
		"exact_identity": exact,
		"single_color":   single,
		"color_count":    countFacet,
		"splash_color":   splash,
	} {
		for key, a := range m {
			res.ColorStats = append(res.ColorStats, model.ColorStatRow{
				Facet: facet, FacetKey: key, DeckCount: a.decks,
				Games: a.games, Wins: a.wins, Losses: a.losses,
				Winrate: ratePtr(a.wins, a.games),
			})
		}
	}

	// card_stats
	for id, a := range cardAgg {
		res.CardStats = append(res.CardStats, model.CardStatRow{
			CardID: id, DeckCount: a.decks,
			InclusionRate: float64(a.decks) / float64(totalDecks),
			Games:         a.games, Wins: a.wins, Losses: a.losses,
			Winrate: ratePtr(a.wins, a.games),
		})
	}

	// card_pair_stats (both directions per co-occurring pair with co_count >= 2)
	for key, p := range pairAggs {
		if p.coCount < 2 {
			continue
		}
		a, b := key[0], key[1]
		pw := ratePtr(p.wins, p.games)
		res.PairStats = append(res.PairStats,
			model.PairStatRow{CardA: a, CardB: b, CoCount: p.coCount, PairWinrate: pw},
			model.PairStatRow{CardA: b, CardB: a, CoCount: p.coCount, PairWinrate: pw},
		)
	}

	// meta_snapshot
	meta := model.MetaSnapshotRow{TotalDecks: totalDecks, TotalGames: totalGames, OverallWinrate: mu}
	if totalDecks > 0 {
		acc := float64(colorCountSum) / float64(totalDecks)
		meta.AvgColorCount = &acc
		mono := float64(monoCount) / float64(totalDecks)
		multi := float64(multiCount) / float64(totalDecks)
		meta.MonoShare = &mono
		meta.MultiShare = &multi
	}
	if cmcDeckN > 0 {
		avg := cmcDeckSum / float64(cmcDeckN)
		meta.AvgCMC = &avg
	}
	res.Meta = meta

	// deck_metric_stats
	for cc, a := range countFacet {
		res.DeckMetrics = append(res.DeckMetrics, model.DeckMetricRow{
			Metric: "color_count", Bucket: strconv.Itoa(cc), DeckCount: a.decks, Winrate: ratePtr(a.wins, a.games),
		})
	}
	for bucket, a := range cmcBucket {
		res.DeckMetrics = append(res.DeckMetrics, model.DeckMetricRow{
			Metric: "avg_cmc", Bucket: bucket, DeckCount: a.decks, Winrate: ratePtr(a.wins, a.games),
		})
	}

	return res
}

// ratePtr returns wins/games as a pointer, or nil when games == 0.
func ratePtr(wins, games int) *float64 {
	if games == 0 {
		return nil
	}
	r := float64(wins) / float64(games)
	return &r
}

func getAcc(m map[int]*acc, key int) *acc {
	a := m[key]
	if a == nil {
		a = &acc{}
		m[key] = a
	}
	return a
}

func getAcc2(m map[string]*acc, key string) *acc {
	a := m[key]
	if a == nil {
		a = &acc{}
		m[key] = a
	}
	return a
}

func sortedIDs(set map[uuid.UUID]struct{}) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})
	return ids
}

func cmcRange(avg float64) string {
	switch {
	case avg < 2:
		return "<2"
	case avg < 2.5:
		return "2-2.5"
	case avg < 3:
		return "2.5-3"
	case avg < 3.5:
		return "3-3.5"
	default:
		return "3.5+"
	}
}
