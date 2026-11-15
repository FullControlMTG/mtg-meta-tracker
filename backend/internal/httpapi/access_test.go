package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/runyanjake/mtg-meta-tracker/backend/internal/appctx"
)

type fakeMembers struct {
	members, owners map[uuid.UUID]bool
	err             error
}

func (f fakeMembers) IsCubeMember(_ context.Context, _, userID uuid.UUID) (bool, error) {
	return f.members[userID], f.err
}

func (f fakeMembers) IsCubeOwner(_ context.Context, _, userID uuid.UUID) (bool, error) {
	return f.owners[userID], f.err
}

var (
	ownerID    = uuid.New()
	memberID   = uuid.New()
	outsiderID = uuid.New()
	adminID    = uuid.New()
)

func accessServer(err error) *Server {
	return &Server{members: fakeMembers{
		members: map[uuid.UUID]bool{ownerID: true, memberID: true},
		owners:  map[uuid.UUID]bool{ownerID: true},
		err:     err,
	}}
}

func asCaller(id uuid.UUID, role appctx.Role, target string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, target, nil)
	return r.WithContext(appctx.With(r.Context(),
		appctx.Caller{Kind: appctx.Authenticated, UserID: id, Role: role}))
}

func TestRequireCubeAccess(t *testing.T) {
	cases := []struct {
		name   string
		caller uuid.UUID
		role   appctx.Role
		ok     bool
		status int
	}{
		{"owner", ownerID, appctx.RoleUser, true, http.StatusOK},
		{"member", memberID, appctx.RoleUser, true, http.StatusOK},
		// 404, not 403: a non-member isn't told the cube exists.
		{"outsider", outsiderID, appctx.RoleUser, false, http.StatusNotFound},
		{"admin non-member", adminID, appctx.RoleAdmin, true, http.StatusOK},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		ok := accessServer(nil).requireCubeAccess(w, asCaller(tc.caller, tc.role, "/"), uuid.New())
		if ok != tc.ok || w.Code != tc.status {
			t.Errorf("%s: ok=%v status=%d, want ok=%v status=%d", tc.name, ok, w.Code, tc.ok, tc.status)
		}
	}
}

func TestRequireCubeOwner(t *testing.T) {
	cases := []struct {
		name   string
		caller uuid.UUID
		role   appctx.Role
		ok     bool
		status int
	}{
		{"owner", ownerID, appctx.RoleUser, true, http.StatusOK},
		{"member", memberID, appctx.RoleUser, false, http.StatusForbidden},
		{"outsider", outsiderID, appctx.RoleUser, false, http.StatusNotFound},
		{"admin non-member", adminID, appctx.RoleAdmin, true, http.StatusOK},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		ok := accessServer(nil).requireCubeOwner(w, asCaller(tc.caller, tc.role, "/"), uuid.New())
		if ok != tc.ok || w.Code != tc.status {
			t.Errorf("%s: ok=%v status=%d, want ok=%v status=%d", tc.name, ok, w.Code, tc.ok, tc.status)
		}
	}
}

// A store error must deny with a 500, never fall through to allowing the request.
func TestAccessHelpersDenyOnStoreError(t *testing.T) {
	s := accessServer(errors.New("db down"))
	for name, check := range map[string]func(http.ResponseWriter, *http.Request, uuid.UUID) bool{
		"access": s.requireCubeAccess,
		"owner":  s.requireCubeOwner,
	} {
		w := httptest.NewRecorder()
		if check(w, asCaller(ownerID, appctx.RoleUser, "/"), uuid.New()) || w.Code != http.StatusInternalServerError {
			t.Errorf("%s: status %d, want a 500 denial", name, w.Code)
		}
	}
}

// Anonymous callers carry the zero user ID, which is in no cube.
func TestRequireCubeAccessRejectsPublicCaller(t *testing.T) {
	w := httptest.NewRecorder()
	if accessServer(nil).requireCubeAccess(w, httptest.NewRequest(http.MethodGet, "/", nil), uuid.New()) {
		t.Fatal("public caller was let in")
	}
}

func TestCubeParamAccess(t *testing.T) {
	cube := uuid.New()
	w := httptest.NewRecorder()
	id, ok := accessServer(nil).cubeParamAccess(w, asCaller(memberID, appctx.RoleUser, "/?cube="+cube.String()))
	if !ok || id != cube {
		t.Fatalf("member: id=%v ok=%v", id, ok)
	}

	w = httptest.NewRecorder()
	if _, ok := accessServer(nil).cubeParamAccess(w, asCaller(memberID, appctx.RoleUser, "/?cube=nope")); ok || w.Code != http.StatusBadRequest {
		t.Fatalf("bad id: ok=%v status=%d, want 400", ok, w.Code)
	}

	w = httptest.NewRecorder()
	if _, ok := accessServer(nil).cubeParamAccess(w, asCaller(outsiderID, appctx.RoleUser, "/?cube="+cube.String())); ok || w.Code != http.StatusNotFound {
		t.Fatalf("outsider: ok=%v status=%d, want 404", ok, w.Code)
	}
}
