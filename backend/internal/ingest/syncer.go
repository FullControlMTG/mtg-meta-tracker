package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
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

	// Record that a sync is now in progress so the admin page can show it. Errors
	// here are best-effort — never let progress bookkeeping fail the actual sync.
	_ = s.store.BeginCubeSyncProgress(ctx, cubeID, "resolving")

	// Parse the pasted list into the unique mainboard entries that make up the pool.
	entries, st := poolEntriesFromList(cube.CardList)
	log.Printf("sync cube %s: parsed %d lines -> %d entries (main=%d side=%d maybe=%d), "+
		"%d annotated, %d merged, %d headers, %d comments",
		cubeID, st.Lines, st.Entries, st.Main, st.Side, st.Maybe,
		st.Annotated, st.Merged, st.Headers, st.Comments)

	// Change detection: fingerprint the entry-set. If it matches the last built
	// list, skip the expensive Scryfall resolve / pool rewrite / analytics
	// recompute and just record that we checked.
	hash := hashEntries(entries)
	if cube.ContentHash != nil && *cube.ContentHash == hash {
		if err := s.store.SetCubeSyncState(ctx, cubeID, hash, time.Now()); err != nil {
			_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "failed", err.Error())
			return err
		}
		// Report the size of the pool we actually built, not len(entries) — the pasted
		// list may contain names that never resolved, and counting those would claim
		// more cards than the cube holds. The previous run's `unresolved` still
		// stands (same list ⇒ same names failed), so BeginCubeSyncProgress leaves it.
		active, err := s.store.CountActiveCubeCards(ctx, cubeID)
		if err != nil {
			active = len(entries) // best-effort; never fail a sync over a progress counter
		}
		_ = s.store.SetCubeSyncResolved(ctx, cubeID, active, 0)
		_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "done", "")
		// Say what the pool actually holds, not just that we skipped: this branch is
		// how a bad pool stays frozen across syncs, so the number has to be visible.
		log.Printf("sync cube %s: list unchanged, skipped resolve; pool holds %d cards (list has %d entries)",
			cubeID, active, len(entries))
		return nil
	}

	queries := make([]scryfall.Query, len(entries))
	for i, e := range entries {
		queries[i] = scryfall.Query{Name: e.Name, SetCode: e.SetCode, Collector: e.Collector}
	}
	results, err := s.scry.Resolve(ctx, queries)
	if err != nil {
		_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "failed", err.Error())
		return fmt.Errorf("scryfall: %w", err)
	}

	// Walk the results once: upsert what resolved, name what didn't. Results come
	// back one-per-query in query order, so a card is bound to its own entry by
	// position — never by matching Scryfall's canonical name against the pasted one.
	var (
		cards      []domain.Card
		pool       []store.PoolCard
		unresolved []string
		seen       = map[uuid.UUID]int{} // scryfall id -> pool index that claimed it
		dupes      int
	)
	for i, r := range results {
		if r.Card == nil {
			unresolved = append(unresolved, describeEntry(entries[i]))
			continue
		}
		if err := s.store.UpsertCard(ctx, r.Card); err != nil {
			_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "failed", err.Error())
			return fmt.Errorf("upsert card %s: %w", r.Card.Name, err)
		}
		qty := entries[i].Quantity
		if qty < 1 {
			qty = 1
		}
		// Two entries resolving to one printing share a cube_cards row — the table is
		// keyed on (cube_id, card_id). Their copies add rather than one entry winning
		// and the other vanishing, which is what "2 Forest" + "3 Forest (LEA)" means.
		if prev, dup := seen[r.Card.ScryfallID]; dup {
			dupes++
			pool[prev].Quantity += qty
			log.Printf("sync cube %s: duplicate printing: %q and %q both resolved to %s; %d copies total",
				cubeID, describeEntry(entries[i]), r.Card.Name, r.Card.ScryfallID, pool[prev].Quantity)
			continue
		}
		seen[r.Card.ScryfallID] = len(pool)
		cards = append(cards, *r.Card)
		pool = append(pool, store.PoolCard{CardID: r.Card.ScryfallID, Quantity: qty})
	}

	// Unresolved names are dropped from the pool, so record them on the progress
	// row for the admin page — logging alone let a typo silently shrink the cube.
	// Written unconditionally so a run that fixes a typo clears the stale list.
	_ = s.store.SetCubeSyncUnresolved(ctx, cubeID, unresolved)
	if len(unresolved) > 0 {
		log.Printf("sync cube %s: %d entries unresolved: %v", cubeID, len(unresolved), unresolved)
	}
	log.Printf("sync cube %s: upsert: %d resolved, %d distinct printings, %d duplicates merged",
		cubeID, len(results)-len(unresolved), len(pool), dupes)

	active, err := s.store.SyncCubeCards(ctx, cubeID, pool)
	if err != nil {
		_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "failed", err.Error())
		return fmt.Errorf("sync cube_cards: %w", err)
	}
	if active != len(pool) {
		log.Printf("sync cube %s: WARNING cube_cards holds %d active rows, expected %d",
			cubeID, active, len(pool))
	}
	log.Printf("sync cube %s: cube_cards: %d active rows (expected %d)", cubeID, active, len(pool))
	if err := s.store.SetCubeSyncState(ctx, cubeID, hash, time.Now()); err != nil {
		_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "failed", err.Error())
		return err
	}
	// Pool changed → refresh analytics for this cube.
	_ = s.store.EnqueueJob(ctx, "recompute_analytics",
		map[string]string{"cube_id": cubeID.String(), "trigger": "cube_synced"},
		"recompute:"+cubeID.String())
	log.Printf("sync cube %s: %d active cards", cubeID, len(pool))

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
// (<scryfall_id>-<variant>, default variant "normal"). It also drives the
// downloading phase of the cube's sync-progress row and marks it done when the
// downloads finish — this goroutine outlives the job, so the progress row (not
// the job status) reflects when images are actually ready.
func (s *Syncer) prefetchImages(ctx context.Context, cubeID uuid.UUID, cards []domain.Card) {
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

	// Enter the downloading phase with known totals before any bytes move.
	_ = s.store.SetCubeSyncResolved(ctx, cubeID, len(cards), len(items))

	if s.images == nil || len(items) == 0 {
		_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "done", "")
		return
	}

	// Throttle progress writes: the callback fires once per downloaded image, but
	// we only persist the running counters ~twice a second to keep DB load low.
	var mu sync.Mutex
	lastWrite := time.Time{}
	failed := s.images.Prefetch(ctx, items, func(done, failedSoFar int) {
		mu.Lock()
		if time.Since(lastWrite) >= 500*time.Millisecond {
			lastWrite = time.Now()
			mu.Unlock()
			_ = s.store.SetCubeSyncImages(ctx, cubeID, done, failedSoFar)
			return
		}
		mu.Unlock()
	})
	// Final flush + mark done so the last few images and the failed count land
	// even if they arrived inside the throttle window.
	_ = s.store.SetCubeSyncImages(ctx, cubeID, len(items)-failed, failed)
	_ = s.store.FinishCubeSyncProgress(ctx, cubeID, "done", "")

	log.Printf("sync cube %s: prefetched %d/%d images (%d failed)",
		cubeID, len(items)-failed, len(items), failed)
}

// poolEntriesFromList parses a raw pasted decklist into the mainboard entries
// that make up the cube pool, plus a per-line accounting of the parse.
// Quantities are ignored (a cube is a set of distinct cards); side/maybe boards
// are excluded. Returns nil for an empty or nil list, which clears the pool.
//
// The returned Stats matter: entries lost here — to a stray section header, or
// to the same-name merge — never reach Scryfall, so they can never show up as
// unresolved names. Without the counters that loss is completely invisible.
func poolEntriesFromList(cardList *string) ([]decklist.ParsedCard, decklist.Stats) {
	if cardList == nil {
		return nil, decklist.Stats{}
	}
	parsed, st := decklist.ParseListStats(*cardList)
	var entries []decklist.ParsedCard
	for _, p := range parsed {
		if p.Board == domain.BoardMain {
			entries = append(entries, p)
		}
	}
	return entries, st
}

// describeEntry renders an entry the way the list wrote it, so an unresolved
// report names the printing that failed and not just the card.
func describeEntry(e decklist.ParsedCard) string {
	if e.SetCode == "" {
		return e.Name
	}
	if e.Collector == "" {
		return fmt.Sprintf("%s (%s)", e.Name, e.SetCode)
	}
	return fmt.Sprintf("%s (%s) %s", e.Name, e.SetCode, e.Collector)
}

// Bumped whenever resolution changes what a given list produces. It is folded
// into the content hash so every stored fingerprint is invalidated on deploy —
// otherwise the unchanged-list short-circuit would skip the resolve and keep
// serving the pool the old resolver built.
//
// 3: the pool carries per-card quantities.
const resolverVersion = 3

// hashEntries produces an order-independent, case-insensitive fingerprint of the
// pool entries, used to detect whether the cube list has changed. The printing
// is part of the fingerprint: re-pointing a card at a different set or collector
// number has to trigger a re-resolve even though the name is the same. So is the
// quantity — "1 Ornithopter" becoming "150 Ornithopter" changes the pool without
// changing which cards are in it, and would otherwise hash the same and be skipped.
func hashEntries(entries []decklist.ParsedCard) string {
	norm := make([]string, len(entries))
	for i, e := range entries {
		norm[i] = strings.ToLower(strings.TrimSpace(e.Name)) + "|" +
			strings.ToLower(e.SetCode) + "|" + strings.ToLower(e.Collector) + "|" +
			strconv.Itoa(e.Quantity)
	}
	sort.Strings(norm)
	joined := fmt.Sprintf("v%d\n%s", resolverVersion, strings.Join(norm, "\n"))
	sum := sha256.Sum256([]byte(joined))
	return hex.EncodeToString(sum[:])
}
