package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/decklist"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/images"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/scryfall"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

type Syncer struct {
	store  *store.Store
	scry   *scryfall.Client
	images *images.Cache
}

func NewSyncer(s *store.Store, scry *scryfall.Client, imgs *images.Cache) *Syncer {
	return &Syncer{store: s, scry: scry, images: imgs}
}

// SyncCube rebuilds a cube's card pool from its stored card_list (a raw pasted
// decklist in standard format). The Moxfield URL, if any, is retained only as
// display metadata — it is no longer fetched (Moxfield's API blocks us).
func (s *Syncer) SyncCube(ctx context.Context, cubeID uuid.UUID) error {
	cube, err := s.store.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}

	// Parse the pasted list into the set of unique mainboard card names.
	names := poolNamesFromList(cube.CardList)

	// Change detection: fingerprint the name-set. If it matches the last built
	// list, skip the expensive Scryfall resolve / pool rewrite / analytics
	// recompute and just record that we checked.
	hash := hashNames(names)
	if cube.ContentHash != nil && *cube.ContentHash == hash {
		if err := s.store.SetCubeSyncState(ctx, cubeID, hash, time.Now()); err != nil {
			return err
		}
		log.Printf("sync cube %s: list unchanged (%d cards), skipped", cubeID, len(names))
		return nil
	}

	cards, notFound, err := s.scry.ResolveByNames(ctx, names)
	if err != nil {
		return fmt.Errorf("scryfall: %w", err)
	}
	if len(notFound) > 0 {
		log.Printf("sync cube %s: %d names unresolved: %v", cubeID, len(notFound), notFound)
	}

	activeIDs := make([]uuid.UUID, 0, len(cards))
	for i := range cards {
		if err := s.store.UpsertCard(ctx, &cards[i]); err != nil {
			return fmt.Errorf("upsert card %s: %w", cards[i].Name, err)
		}
		activeIDs = append(activeIDs, cards[i].ScryfallID)
	}

	if err := s.store.SyncCubeCards(ctx, cubeID, activeIDs); err != nil {
		return fmt.Errorf("sync cube_cards: %w", err)
	}
	if err := s.store.SetCubeSyncState(ctx, cubeID, hash, time.Now()); err != nil {
		return err
	}
	// Pool changed → refresh analytics for this cube.
	_ = s.store.EnqueueJob(ctx, "recompute_analytics",
		map[string]string{"cube_id": cubeID.String(), "trigger": "cube_synced"},
		"recompute:"+cubeID.String())
	log.Printf("sync cube %s: %d active cards", cubeID, len(activeIDs))

	// Best-effort: warm the on-disk image cache for this pool so the cube page
	// serves self-hosted images immediately instead of downloading on first view.
	// Already-cached images are skipped; failures are logged, not fatal (the lazy
	// on-demand fetch still covers any that miss here). Detached from the job so it
	// doesn't hold the single-threaded worker for the full download — the pool is
	// already committed and analytics/revalidation can proceed. ctx is the
	// long-lived worker context (cancelled only on shutdown), so it's safe here.
	go s.prefetchImages(ctx, cubeID, cards)
	return nil
}

// prefetchImages downloads the "normal" image for each resolved card into the
// shared image cache. The key format mirrors the image endpoint's
// (<scryfall_id>-<variant>, default variant "normal").
func (s *Syncer) prefetchImages(ctx context.Context, cubeID uuid.UUID, cards []domain.Card) {
	if s.images == nil {
		return
	}
	items := make([]images.PrefetchItem, 0, len(cards))
	for i := range cards {
		if cards[i].ImageNormal == nil || *cards[i].ImageNormal == "" {
			continue
		}
		items = append(items, images.PrefetchItem{
			Key: cards[i].ScryfallID.String() + "-normal",
			URL: *cards[i].ImageNormal,
		})
	}
	if len(items) == 0 {
		return
	}
	failed := s.images.Prefetch(ctx, items)
	log.Printf("sync cube %s: prefetched %d/%d images (%d failed)",
		cubeID, len(items)-failed, len(items), failed)
}

// poolNamesFromList parses a raw pasted decklist into the set of mainboard card
// names that make up the cube pool. Quantities are ignored (a cube is a set of
// distinct cards); side/maybe boards are excluded. Returns nil for an empty or
// nil list, which clears the pool.
func poolNamesFromList(cardList *string) []string {
	if cardList == nil {
		return nil
	}
	var names []string
	for _, p := range decklist.ParseList(*cardList) {
		if p.Board == domain.BoardMain {
			names = append(names, p.Name)
		}
	}
	return names
}

// hashNames produces an order-independent, case-insensitive fingerprint of a
// card-name set, used to detect whether the cube list has changed.
func hashNames(names []string) string {
	norm := make([]string, len(names))
	for i, n := range names {
		norm[i] = strings.ToLower(strings.TrimSpace(n))
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:])
}
