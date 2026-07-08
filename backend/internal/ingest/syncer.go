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

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/moxfield"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/scryfall"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
)

type Syncer struct {
	store *store.Store
	scry  *scryfall.Client
	mox   *moxfield.Client
}

func NewSyncer(s *store.Store, scry *scryfall.Client, mox *moxfield.Client) *Syncer {
	return &Syncer{store: s, scry: scry, mox: mox}
}

func (s *Syncer) SyncCube(ctx context.Context, cubeID uuid.UUID) error {
	cube, err := s.store.GetCube(ctx, cubeID)
	if err != nil {
		return err
	}
	if cube.MoxfieldPublicID == nil || *cube.MoxfieldPublicID == "" {
		return fmt.Errorf("cube %s has no moxfield source", cubeID)
	}

	names, err := s.mox.FetchCubeCardNames(ctx, *cube.MoxfieldPublicID)
	if err != nil {
		return fmt.Errorf("moxfield: %w", err)
	}

	// Change detection: fingerprint the returned name-set. If it matches the
	// last synced list, skip the expensive Scryfall resolve / pool rewrite /
	// analytics recompute and just record that we checked.
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
	return nil
}

// hashNames produces an order-independent, case-insensitive fingerprint of a
// card-name set, used to detect whether the Moxfield list has changed.
func hashNames(names []string) string {
	norm := make([]string, len(names))
	for i, n := range names {
		norm[i] = strings.ToLower(strings.TrimSpace(n))
	}
	sort.Strings(norm)
	sum := sha256.Sum256([]byte(strings.Join(norm, "\n")))
	return hex.EncodeToString(sum[:])
}
