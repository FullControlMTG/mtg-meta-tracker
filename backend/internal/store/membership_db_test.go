package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
)

// testStore connects to TEST_DATABASE_URL and isolates the test in a throwaway
// Postgres schema, so it can share a database with dev data without touching it.
// Unset, the test skips: `make test` stays runnable with no database.
func testStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	name := fmt.Sprintf("test_%d", time.Now().UnixNano())

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+name+" CASCADE")
		admin.Close()
	})

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = name + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	st := New(pool)
	if err := st.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	return st
}

func mustUser(t *testing.T, st *Store, username, role string) *domain.User {
	t.Helper()
	u := &domain.User{Username: username, DisplayName: username, Role: role}
	if err := st.CreateUser(context.Background(), u); err != nil {
		t.Fatalf("create user %s: %v", username, err)
	}
	return u
}

func memberCount(t *testing.T, st *Store, cube *domain.Cube) int {
	t.Helper()
	var n int
	if err := st.pool.QueryRow(context.Background(),
		`SELECT count(*) FROM cube_members WHERE cube_id=$1`, cube.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// A cube whose sole owner is deleted is left with no members. The schema re-runs on
// every boot, and it once seeded every user into such a cube — opening a private
// cube to the whole site on the next deploy. Membership must stay invite-only.
func TestEnsureSchemaDoesNotEnrollUsersInOrphanedCube(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	admin := mustUser(t, st, "admin", domain.RoleAdmin)
	owner := mustUser(t, st, "owner", "user")
	outsider := mustUser(t, st, "outsider", "user")

	cube := &domain.Cube{Name: "private", OwnerID: &owner.ID}
	if err := st.CreateCube(ctx, cube); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser(ctx, owner.ID); err != nil {
		t.Fatal(err)
	}

	// Simulate the next deploy.
	if err := st.EnsureSchema(ctx); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}

	if n := memberCount(t, st, cube); n != 0 {
		t.Fatalf("orphaned cube has %d members after reboot, want 0", n)
	}
	if ok, err := st.IsCubeMember(ctx, cube.ID, outsider.ID); err != nil || ok {
		t.Fatalf("outsider is a member (ok=%v, err=%v); membership must be invite-only", ok, err)
	}
	got, err := st.GetCube(ctx, cube.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerID == nil || *got.OwnerID != admin.ID {
		t.Fatalf("orphaned cube owner = %v, want the first admin %v", got.OwnerID, admin.ID)
	}
}

// Invites remain the one way in: accepting one enrolls exactly the invitee.
func TestAcceptInviteEnrollsOnlyInvitee(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	owner := mustUser(t, st, "owner", "user")
	invitee := mustUser(t, st, "invitee", "user")
	mustUser(t, st, "bystander", "user")

	cube := &domain.Cube{Name: "private", OwnerID: &owner.ID}
	if err := st.CreateCube(ctx, cube); err != nil {
		t.Fatal(err)
	}
	inviteID, err := st.CreateInvite(ctx, cube.ID, invitee.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptInvite(ctx, inviteID); err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureSchema(ctx); err != nil {
		t.Fatalf("re-apply schema: %v", err)
	}

	if n := memberCount(t, st, cube); n != 2 {
		t.Fatalf("cube has %d members, want 2 (owner + invitee)", n)
	}
	if ok, _ := st.IsCubeMember(ctx, cube.ID, invitee.ID); !ok {
		t.Fatal("invitee is not a member after accepting")
	}
}
