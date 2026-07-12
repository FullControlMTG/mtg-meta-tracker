package decklist

import (
	"context"
	"log"
	"strings"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/scryfall"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

// Resolver turns parsed card entries into decklist_cards rows, matching names
// first against the cube pool (DB) and then falling back to Scryfall for names
// not in the pool. Names that resolve nowhere are kept unresolved.
type Resolver struct {
	store *store.Store
	scry  *scryfall.Client
}

func NewResolver(s *store.Store, scry *scryfall.Client) *Resolver {
	return &Resolver{store: s, scry: scry}
}

// Resolved is the outcome of resolving a raw list: the rows to persist plus the
// inferred deck colors (the ones it is built on, and the ones it splashes) and the
// names that could not be resolved.
type Resolved struct {
	Cards         []domain.DecklistCard
	ColorIdentity int
	SplashColors  int
	Unresolved    []string
}

// Resolve matches each parsed card. It mutates no state except upserting newly
// fetched Scryfall cards into the card cache.
func (r *Resolver) Resolve(ctx context.Context, cubeID uuid.UUID, parsed []ParsedCard) (*Resolved, error) {
	names := make([]string, len(parsed))
	for i, p := range parsed {
		names[i] = p.Name
	}

	// 1. Cube pool. The returned map is keyed by every alias a pool card answers
	// to, so a "Front / Back" or flavor-named entry still finds it.
	pool, err := r.store.LookupCubeCardsByName(ctx, cubeID, names)
	if err != nil {
		return nil, err
	}

	// card[i] is the card for parsed[i], or nil. Indexed, never keyed by name:
	// Scryfall's canonical name ("Bonecrusher Giant // Stomp", "Karakas") need not
	// equal what the list wrote, and matching the two is how cards that resolved
	// perfectly well used to get thrown away.
	cards := make([]*domain.Card, len(parsed))
	for i, p := range parsed {
		if c, ok := poolLookup(pool, p.Name); ok {
			cards[i] = &c
		}
	}

	// 2. Scryfall fallback for entries not in the pool.
	var (
		queries    []scryfall.Query
		queryIdx   []int // queries[k] came from parsed[queryIdx[k]]
		seenMissed = map[string]int{}
	)
	for i, p := range parsed {
		if cards[i] != nil {
			continue
		}
		// Same card on two boards resolves once; step 2's fan-out gives both the result.
		key := strings.ToLower(p.Name)
		if _, dup := seenMissed[key]; dup {
			continue
		}
		seenMissed[key] = len(queries)
		queries = append(queries, scryfall.Query{
			Name: p.Name, SetCode: p.SetCode, Collector: p.Collector,
		})
		queryIdx = append(queryIdx, i)
	}

	if len(queries) > 0 {
		results, err := r.scry.Resolve(ctx, queries)
		if err != nil {
			return nil, err
		}
		for k, res := range results {
			if res.Card == nil {
				continue
			}
			if err := r.store.UpsertCard(ctx, res.Card); err != nil {
				return nil, err
			}
			cards[queryIdx[k]] = res.Card
		}
		// Fan a resolved card back out to any other entry naming the same card.
		for i, p := range parsed {
			if cards[i] != nil {
				continue
			}
			if k, ok := seenMissed[strings.ToLower(p.Name)]; ok && results[k].Card != nil {
				cards[i] = results[k].Card
			}
		}
	}

	// 3. Build rows + infer deck colors from resolved main-board cards.
	res := &Resolved{Cards: make([]domain.DecklistCard, 0, len(parsed))}
	var colorCards []domain.DeckColorCard
	for i, p := range parsed {
		dc := domain.DecklistCard{
			CardName: p.Name,
			Quantity: p.Quantity,
			Board:    p.Board,
		}
		if c := cards[i]; c != nil {
			id := c.ScryfallID
			dc.CardID = &id
			dc.IsResolved = true
			if p.Board == domain.BoardMain {
				colorCards = append(colorCards, domain.DeckColorCard{
					Colors:   domain.ColorIdentity(c.Colors),
					IsLand:   c.TypeLine != nil && domain.IsLandType(*c.TypeLine),
					Quantity: p.Quantity,
				})
			}
		} else {
			res.Unresolved = append(res.Unresolved, p.Name)
		}
		res.Cards = append(res.Cards, dc)
	}
	if len(res.Unresolved) > 0 {
		log.Printf("decklist: cube %s: %d of %d entries unresolved: %v",
			cubeID, len(res.Unresolved), len(parsed), res.Unresolved)
	}
	dc := domain.InferDeckColors(colorCards)
	res.ColorIdentity = int(dc.Main)
	res.SplashColors = int(dc.Splash)
	return res, nil
}

// poolLookup probes the alias-keyed pool map with the name as written and with
// its front face.
func poolLookup(pool map[string]domain.Card, name string) (domain.Card, bool) {
	if c, ok := pool[strings.ToLower(name)]; ok {
		return c, true
	}
	c, ok := pool[strings.ToLower(store.FrontFace(name))]
	return c, ok
}
