package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// A database at 0001 links each deck to its cube by decklists.cube_id. 0002 must
// carry that link into the deck's tags before it drops the column, or every deck
// would fall out of its cube on the deploy that ships it.
func TestMigrateFilesExistingDecksUnderTheirCube(t *testing.T) {
	ctx := context.Background()
	ms, err := loadMigrations(migrationFiles, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	st := New(testPool(t))
	if err := migrate(ctx, st.pool, ms[:1]); err != nil {
		t.Fatalf("migrate to 0001: %v", err)
	}
	if _, err := st.pool.Exec(ctx, `
		INSERT INTO users (username, display_name) VALUES ('owner', 'Owner');
		INSERT INTO cubes (name, owner_id) SELECT 'vintage', id FROM users;
		INSERT INTO cubes (name) VALUES ('other');
		INSERT INTO decklists (cube_id, user_id, name, decklist_raw)
			SELECT c.id, u.id, 'Burn', '4 Lightning Bolt' FROM cubes c, users u WHERE c.name = 'vintage';`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("migrate to latest: %v", err)
	}

	var cube domain.Cube
	if err := st.pool.QueryRow(ctx, `SELECT id, deck_tag FROM cubes WHERE name = 'vintage'`).
		Scan(&cube.ID, &cube.DeckTag); err != nil {
		t.Fatal(err)
	}
	if cube.DeckTag != "cube:"+cube.ID.String() {
		t.Errorf("deck_tag = %q, want cube:<id>", cube.DeckTag)
	}
	decks, err := st.ListDecklists(ctx, DecklistFilter{CubeID: &cube.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(decks) != 1 {
		t.Fatalf("cube lists %d decks after the upgrade, want 1", len(decks))
	}
	d := decks[0]
	if !reflect.DeepEqual(d.Tags, []string{cube.DeckTag}) {
		t.Errorf("tags = %v, want [%s]", d.Tags, cube.DeckTag)
	}
	if d.Format == nil || *d.Format != domain.FormatCube {
		t.Errorf("format = %v, want %q", d.Format, domain.FormatCube)
	}
	if d.CubeID == nil || *d.CubeID != cube.ID {
		t.Errorf("derived cube_id = %v, want %s", d.CubeID, cube.ID)
	}
}

// A cube does not own its decks; it selects them by tag. The same deck can sit in
// two cubes, and deleting a cube leaves its decks standing.
func TestCubesSelectDecksByTag(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	owner := mustUser(t, st, "owner", "user")
	a := &domain.Cube{Name: "a", OwnerID: &owner.ID}
	b := &domain.Cube{Name: "b", OwnerID: &owner.ID}
	for _, c := range []*domain.Cube{a, b} {
		if err := st.CreateCube(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	format := domain.FormatCube
	d := &domain.Decklist{CubeID: &a.ID, Format: &format, Tags: []string{"event:fnm"},
		UserID: owner.ID, Name: "Burn", DecklistRaw: "4 Lightning Bolt", Status: domain.StatusActive}
	if err := st.CreateDecklist(ctx, d, nil); err != nil {
		t.Fatal(err)
	}
	if want := []string{"event:fnm", a.DeckTag}; !reflect.DeepEqual(d.Tags, want) {
		t.Fatalf("created with tags %v, want %v", d.Tags, want)
	}

	count := func(c *domain.Cube) int {
		t.Helper()
		ids, err := st.ListCubeDecklistIDs(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		return len(ids)
	}
	if count(a) != 1 || count(b) != 0 {
		t.Fatalf("a lists %d, b lists %d; want 1, 0", count(a), count(b))
	}

	if _, err := st.pool.Exec(ctx, `UPDATE decklists SET tags = tags || $2 WHERE id = $1`,
		d.ID, []string{b.DeckTag}); err != nil {
		t.Fatal(err)
	}
	if count(a) != 1 || count(b) != 1 {
		t.Fatalf("tagged into both: a lists %d, b lists %d; want 1, 1", count(a), count(b))
	}

	if err := st.DeleteCube(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetDecklist(ctx, d.ID)
	if err != nil {
		t.Fatalf("deck did not survive its cube: %v", err)
	}
	if got.CubeID == nil || *got.CubeID != b.ID {
		t.Errorf("cube_id after deleting a = %v, want b (%s)", got.CubeID, b.ID)
	}

	if err := st.DeleteCube(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if got, err = st.GetDecklist(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if got.CubeID != nil {
		t.Errorf("cube_id with every cube gone = %s, want nil", *got.CubeID)
	}
}
