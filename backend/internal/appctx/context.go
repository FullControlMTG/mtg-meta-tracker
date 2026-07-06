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

type Caller struct {
	Kind   CallerKind
	UserID uuid.UUID // zero value when Public
	Role   Role
}

var PublicCaller = Caller{Kind: Public}

func (c Caller) IsAuthenticated() bool { return c.Kind == Authenticated }
func (c Caller) IsAdmin() bool         { return c.Kind == Authenticated && c.Role == RoleAdmin }

func (c Caller) Owns(ownerID uuid.UUID) bool {
	return c.Kind == Authenticated && c.UserID == ownerID
}

// Owner or admin may update/delete user-owned resources.
func (c Caller) CanMutateOwned(ownerID uuid.UUID) bool {
	return c.IsAdmin() || c.Owns(ownerID)
}

type ctxKey struct{}

func With(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

func From(ctx context.Context) Caller {
	if c, ok := ctx.Value(ctxKey{}).(Caller); ok {
		return c
	}
	return PublicCaller
}
