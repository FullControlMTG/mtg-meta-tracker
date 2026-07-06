// Package appctx defines the Caller carried on every request context.
//
// Every backend operation runs under either a Public (anonymous) caller or an
// Authenticated one that carries the user id + role, so authorization can be
// decided in one place. See docs/DESIGN.md §2.
package appctx

import (
	"context"

	"github.com/google/uuid"
)

type CallerKind int

const (
	Public CallerKind = iota
	Authenticated
)

type Role int

const (
	RoleUser Role = iota
	RoleAdmin
)

// Caller identifies who is making a backend call.
type Caller struct {
	Kind   CallerKind
	UserID uuid.UUID // zero value when Public
	Role   Role
}

// PublicCaller is the anonymous, read-only caller.
var PublicCaller = Caller{Kind: Public}

func (c Caller) IsAuthenticated() bool { return c.Kind == Authenticated }
func (c Caller) IsAdmin() bool         { return c.Kind == Authenticated && c.Role == RoleAdmin }

// Owns reports whether this caller owns the resource created by ownerID.
func (c Caller) Owns(ownerID uuid.UUID) bool {
	return c.Kind == Authenticated && c.UserID == ownerID
}

// CanMutateOwned is the standard rule for user-owned resources (decklists,
// profiles): the owner or an admin may update/delete.
func (c Caller) CanMutateOwned(ownerID uuid.UUID) bool {
	return c.IsAdmin() || c.Owns(ownerID)
}

// --- context plumbing ---

type ctxKey struct{}

// With returns a context carrying the given caller.
func With(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// From extracts the caller; absent one, the caller is Public.
func From(ctx context.Context) Caller {
	if c, ok := ctx.Value(ctxKey{}).(Caller); ok {
		return c
	}
	return PublicCaller
}
