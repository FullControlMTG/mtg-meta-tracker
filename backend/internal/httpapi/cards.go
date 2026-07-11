package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// CardDetail is everything the /cards/<slug> dashboard renders, for one cube.
type CardDetail struct {
	Card   store.CardView `json:"card"`
	CubeID uuid.UUID      `json:"cube_id"`
	InPool bool           `json:"in_pool"`

	// Stat is nil when the card is in no analyzed deck (in the pool, never drafted),
	// or when the cube has no analytics run yet.
	Stat            *store.CardStat   `json:"stat"`
	RankByInclusion *int              `json:"rank_by_inclusion"`
	TotalRanked     int               `json:"total_ranked"`
	ColorSplit      []store.ColorStat `json:"color_split"`
	ColorCountSplit []store.ColorStat `json:"color_count_split"`
	Pairs           []store.CardPair  `json:"pairs"`
	Decks           []store.DeckBrief `json:"decks"`
}

func (s *Server) handleGetCard(w http.ResponseWriter, r *http.Request) {
	cubeID, ok := cubeParam(w, r)
	if !ok {
		return
	}
	slug := chi.URLParam(r, "slug")
	card, inPool, err := s.store.GetCardBySlug(r.Context(), cubeID, slug)
	if err != nil {
		writeErr(w, statusForStoreErr(err), "card not found")
		return
	}

	decks, err := s.store.ListDecksWithCard(r.Context(), cubeID, card.ScryfallID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not load decks for card")
		return
	}

	out := CardDetail{
		Card:            *card,
		CubeID:          cubeID,
		InPool:          inPool,
		Decks:           decks,
		ColorSplit:      colorSplit(decks),
		ColorCountSplit: colorCountSplit(decks),
		Pairs:           []store.CardPair{},
	}

	// The run-derived stats are best-effort: a cube with no analytics run yet still
	// renders the card, its pool membership, and the live deck list.
	if run, err := s.store.GetCurrentRun(r.Context(), cubeID); err == nil {
		if stat, err := s.store.GetCardStat(r.Context(), run.ID, card.ScryfallID); err == nil {
			out.Stat = stat
			if rank, total, err := s.store.CardInclusionRank(r.Context(), run.ID, stat.InclusionRate); err == nil {
				out.RankByInclusion = &rank
				out.TotalRanked = total
			}
		}
		if pairs, err := s.store.ListCardPairs(r.Context(), run.ID, card.ScryfallID, 10); err == nil {
			out.Pairs = pairs
		}
	}

	writeJSON(w, http.StatusOK, out)
}

var wubrg = []int{
	int(domain.White), int(domain.Blue), int(domain.Black), int(domain.Red), int(domain.Green),
}

// colorSplit buckets the decks playing a card by the colors they contain, in the
// shape of the single_color color_stats facet so the frontend renders it with the
// same chart as the cube-wide breakdown. A 2-color deck counts in both colors.
func colorSplit(decks []store.DeckBrief) []store.ColorStat {
	out := make([]store.ColorStat, 0, len(wubrg))
	for _, bit := range wubrg {
		st := store.ColorStat{Facet: "single_color", FacetKey: bit}
		for _, d := range decks {
			if d.ColorIdentity&bit == 0 {
				continue
			}
			addDeckToColorStat(&st, d)
		}
		out = append(out, st)
	}
	return out
}

// colorCountSplit buckets those same decks by how many colors they play (1–5),
// matching the color_count facet.
func colorCountSplit(decks []store.DeckBrief) []store.ColorStat {
	byCount := map[int]*store.ColorStat{}
	for i := 1; i <= 5; i++ {
		byCount[i] = &store.ColorStat{Facet: "color_count", FacetKey: i}
	}
	for _, d := range decks {
		st, ok := byCount[domain.ColorIdentity(d.ColorIdentity).Count()]
		if !ok { // colorless deck — no bucket
			continue
		}
		addDeckToColorStat(st, d)
	}
	out := make([]store.ColorStat, 0, 5)
	for i := 1; i <= 5; i++ {
		out = append(out, *byCount[i])
	}
	return out
}

func addDeckToColorStat(st *store.ColorStat, d store.DeckBrief) {
	st.DeckCount++
	st.Games += d.GamesPlayed
	st.Wins += d.Wins
	st.Losses += d.Losses
	if st.Games > 0 {
		wr := float64(st.Wins) / float64(st.Games)
		st.Winrate = &wr
	}
}
