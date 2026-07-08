package decklist

import (
	"context"
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
// inferred deck color identity and the names that could not be resolved.
type Resolved struct {
	Cards         []domain.DecklistCard
	ColorIdentity int
	Unresolved    []string
}

// Resolve matches each parsed card. It mutates no state except upserting newly
// fetched Scryfall cards into the card cache.
func (r *Resolver) Resolve(ctx context.Context, cubeID uuid.UUID, parsed []ParsedCard) (*Resolved, error) {
	names := make([]string, len(parsed))
	for i, p := range parsed {
		names[i] = p.Name
	}

	// 1. Cube pool.
	byName, err := r.store.LookupCubeCardsByName(ctx, cubeID, names)
	if err != nil {
		return nil, err
	}

	// 2. Scryfall fallback for names not found in the pool.
	var missing []string
	seenMissing := map[string]struct{}{}
	for _, p := range parsed {
		key := strings.ToLower(p.Name)
		if _, ok := byName[key]; ok {
			continue
		}
		if _, dup := seenMissing[key]; dup {
			continue
		}
		seenMissing[key] = struct{}{}
		missing = append(missing, p.Name)
	}
	if len(missing) > 0 {
		fetched, _, err := r.scry.ResolveByNames(ctx, missing)
		if err != nil {
			return nil, err
		}
		for i := range fetched {
			c := fetched[i]
			if err := r.store.UpsertCard(ctx, &c); err != nil {
				return nil, err
			}
			byName[strings.ToLower(c.Name)] = c
		}
	}

	// 3. Build rows + infer deck identity from resolved main-board cards.
	res := &Resolved{Cards: make([]domain.DecklistCard, 0, len(parsed))}
	var identities []domain.ColorIdentity
	for _, p := range parsed {
		dc := domain.DecklistCard{
			CardName: p.Name,
			Quantity: p.Quantity,
			Board:    p.Board,
		}
		if c, ok := byName[strings.ToLower(p.Name)]; ok {
			id := c.ScryfallID
			dc.CardID = &id
			dc.IsResolved = true
			if p.Board == domain.BoardMain {
				identities = append(identities, domain.ColorIdentity(c.ColorIdentity))
			}
		} else {
			res.Unresolved = append(res.Unresolved, p.Name)
		}
		res.Cards = append(res.Cards, dc)
	}
	res.ColorIdentity = int(domain.InferDeckIdentity(identities))
	return res, nil
}
