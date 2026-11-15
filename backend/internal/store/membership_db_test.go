package store

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/domain"
	"github.com/runyanjake/mtg-meta-tracker/backend/internal/testdb"
)

// testPool is a migration-free, per-test Postgres schema; see testdb.Pool.
func testPool(t *testing.T) *pgxpool.Pool { return testdb.Pool(t) }

// testStore is testPool migrated to the latest schema.
func testStore(t *testing.T) *Store {
	t.Helper()
	st := New(testPool(t))
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
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

// A cube whose sole owner is deleted is left with no members. The schema used to
// re-run on every boot, and it once seeded every user into such a cube — opening a
// private cube to the whole site on the next deploy. Membership must stay invite-only.
func TestMigrateDoesNotEnrollUsersInOrphanedCube(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	mustUser(t, st, "admin", domain.RoleAdmin)
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
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}

	if n := memberCount(t, st, cube); n != 0 {
		t.Fatalf("orphaned cube has %d members after reboot, want 0", n)
	}
	if ok, err := st.IsCubeMember(ctx, cube.ID, outsider.ID); err != nil || ok {
		t.Fatalf("outsider is a member (ok=%v, err=%v); membership must be invite-only", ok, err)
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
	if err := st.Migrate(ctx); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}

	if n := memberCount(t, st, cube); n != 2 {
		t.Fatalf("cube has %d members, want 2 (owner + invitee)", n)
	}
	if ok, _ := st.IsCubeMember(ctx, cube.ID, invitee.ID); !ok {
		t.Fatal("invitee is not a member after accepting")
	}
}

// The cube boundary end to end: the owner administers, a member reads, an outsider
// sees nothing, an admin sees everything, and a declined invite lets no one in.
func TestCubeAccessBoundary(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	admin := mustUser(t, st, "admin", domain.RoleAdmin)
	owner := mustUser(t, st, "owner", "user")
	member := mustUser(t, st, "member", "user")
	decliner := mustUser(t, st, "decliner", "user")
	outsider := mustUser(t, st, "outsider", "user")

	cube := &domain.Cube{Name: "private", OwnerID: &owner.ID}
	if err := st.CreateCube(ctx, cube); err != nil {
		t.Fatal(err)
	}
	accept, err := st.CreateInvite(ctx, cube.ID, member.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptInvite(ctx, accept); err != nil {
		t.Fatal(err)
	}
	// An invite is spent once answered.
	if err := st.AcceptInvite(ctx, accept); err != ErrNotFound {
		t.Fatalf("re-accepting an invite: err=%v, want ErrNotFound", err)
	}
	decline, err := st.CreateInvite(ctx, cube.ID, decliner.ID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeclineInvite(ctx, decline); err != nil {
		t.Fatal(err)
	}
	if err := st.AcceptInvite(ctx, decline); err != ErrNotFound {
		t.Fatalf("accepting a declined invite: err=%v, want ErrNotFound", err)
	}

	for _, tc := range []struct {
		user          *domain.User
		member, owner bool
	}{
		{owner, true, true},
		{member, true, false},
		{decliner, false, false},
		{outsider, false, false},
		// Admins bypass membership in the HTTP layer; the store must not enroll them.
		{admin, false, false},
	} {
		isMember, err := st.IsCubeMember(ctx, cube.ID, tc.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		isOwner, err := st.IsCubeOwner(ctx, cube.ID, tc.user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if isMember != tc.member || isOwner != tc.owner {
			t.Errorf("%s: member=%v owner=%v, want member=%v owner=%v",
				tc.user.Username, isMember, isOwner, tc.member, tc.owner)
		}
	}

	visible := func(u *domain.User, isAdmin bool) int {
		cubes, err := st.ListCubesForUser(ctx, u.ID, isAdmin)
		if err != nil {
			t.Fatal(err)
		}
		return len(cubes)
	}
	if n := visible(member, false); n != 1 {
		t.Errorf("member sees %d cubes, want 1", n)
	}
	if n := visible(outsider, false); n != 0 {
		t.Errorf("outsider sees %d cubes, want 0", n)
	}
	if n := visible(admin, true); n != 1 {
		t.Errorf("admin sees %d cubes, want 1", n)
	}

	// Ownership is transferred, never abandoned.
	if err := st.RemoveCubeMember(ctx, cube.ID, owner.ID); err != ErrConflict {
		t.Fatalf("removing the owner: err=%v, want ErrConflict", err)
	}
	if err := st.RemoveCubeMember(ctx, cube.ID, member.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := st.IsCubeMember(ctx, cube.ID, member.ID); ok {
		t.Fatal("removed member still has access")
	}
}
