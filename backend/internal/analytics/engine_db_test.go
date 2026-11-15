package analytics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/config"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/store"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/testdb"
)

func str(s string) *string { return &s }

// A card as the Scryfall client would cache it; raw carries the casting colors
// RecomputeDeckColors reads back.
func seedCard(t *testing.T, st *store.Store, name, cost, typeLine string, colors []string) uuid.UUID {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"colors": colors})
	c := &domain.Card{
		ScryfallID: uuid.New(), Name: name, ManaCost: str(cost), CMC: 1, TypeLine: str(typeLine),
		Colors: int(domain.ParseColorIdentity(colors)), ColorIdentity: int(domain.ParseColorIdentity(colors)),
		Raw: raw,
	}
	if err := st.UpsertCard(context.Background(), c); err != nil {
		t.Fatalf("upsert %s: %v", name, err)
	}
	return c.ScryfallID
}

// The path every deck takes into the meta: create it, record its games, recompute.
// The run must land as current with the deck's colors re-derived and its record
// counted.
func TestDecklistToAnalyticsRun(t *testing.T) {
	ctx := context.Background()
	st := store.New(testdb.Pool(t))
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user := &domain.User{Username: "player", DisplayName: "Player", Role: domain.RoleUser}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	cube := &domain.Cube{Name: "cube", OwnerID: &user.ID}
	if err := st.CreateCube(ctx, cube); err != nil {
		t.Fatal(err)
	}
	bolt := seedCard(t, st, "Lightning Bolt", "{R}", "Instant", []string{"R"})
	mountain := seedCard(t, st, "Mountain", "", "Basic Land — Mountain", []string{})
	if _, err := st.SyncCubeCards(ctx, cube.ID, []store.PoolCard{{CardID: bolt}, {CardID: mountain}}); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(st, config.Config{})

	// No decks yet: still a current, empty run, so the dashboard has something to show.
	if err := eng.Recompute(ctx, cube.ID, "test"); err != nil {
		t.Fatalf("empty recompute: %v", err)
	}
	empty, err := st.GetCurrentRun(ctx, cube.ID)
	if err != nil || empty.Status != "ok" || empty.DecksIncluded != 0 {
		t.Fatalf("empty run = %+v, err=%v", empty, err)
	}

	// Saved with no colors, as a deck from before color inference would be.
	deck := &domain.Decklist{
		CubeID: &cube.ID, UserID: user.ID, Name: "Burn", DecklistRaw: "4 Lightning Bolt\n16 Mountain",
		CardCount: 20, Status: domain.StatusActive, PlayedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
	}
	cards := []domain.DecklistCard{
		{CardID: &bolt, CardName: "Lightning Bolt", Quantity: 4, IsResolved: true, Board: domain.BoardMain},
		{CardID: &mountain, CardName: "Mountain", Quantity: 16, IsResolved: true, Board: domain.BoardMain},
	}
	if err := st.CreateDecklist(ctx, deck, cards); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateDecklistRecord(ctx, deck.ID, store.DecklistRecord{GamesPlayed: 3, Wins: 2, Losses: 1}); err != nil {
		t.Fatal(err)
	}

	if err := eng.Recompute(ctx, cube.ID, "test"); err != nil {
		t.Fatalf("recompute: %v", err)
	}

	run, err := st.GetCurrentRun(ctx, cube.ID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == empty.ID || run.Status != "ok" || run.DecksIncluded != 1 || run.GamesIncluded != 3 {
		t.Fatalf("run = %+v; want a new ok run with 1 deck and 3 games", run)
	}
	snap, err := st.GetMetaSnapshot(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalDecks != 1 || snap.TotalGames != 3 {
		t.Fatalf("snapshot = %+v", snap)
	}
	stats, err := st.ListCardStats(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var boltStat *store.CardStat
	for i := range stats {
		if stats[i].CardID == bolt {
			boltStat = &stats[i]
		}
	}
	if boltStat == nil || boltStat.DeckCount != 1 || boltStat.Wins != 2 {
		t.Fatalf("Lightning Bolt stat = %+v in %+v", boltStat, stats)
	}

	got, err := st.GetDecklist(ctx, deck.ID)
	if err != nil {
		t.Fatal(err)
	}
	if domain.ColorIdentity(got.ColorIdentity) != domain.ParseColorIdentity([]string{"R"}) {
		t.Errorf("deck colors = %d after recompute, want red", got.ColorIdentity)
	}
	if got.Winrate == nil {
		t.Error("winrate not generated from the recorded games")
	}
}
