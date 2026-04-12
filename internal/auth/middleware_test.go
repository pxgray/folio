package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

func TestRequireAuth_NoCookie(t *testing.T) {
	a, _ := newTestAuth(t)

	// Dashboard path → redirect
	req := httptest.NewRequest(http.MethodGet, "/-/dashboard/", nil)
	w := httptest.NewRecorder()
	auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/-/auth/login" {
		t.Errorf("unexpected redirect location: %q", w.Header().Get("Location"))
	}

	// API path → 401
	req2 := httptest.NewRequest(http.MethodGet, "/-/api/v1/repos", nil)
	w2 := httptest.NewRecorder()
	auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for API path, got %d", w2.Code)
	}
}

func TestRequireAuth_ValidSession(t *testing.T) {
	ctx := context.Background()
	a, store := newTestAuth(t)

	u := &db.User{Email: "hank@example.com", Name: "Hank"}
	_ = store.CreateUser(ctx, u)
	sess, _ := a.NewSession(ctx, u.ID)

	req := httptest.NewRequest(http.MethodGet, "/-/dashboard/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()

	var gotUser *db.User
	auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = auth.UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotUser == nil || gotUser.ID != u.ID {
		t.Errorf("UserFromContext returned wrong user: %v", gotUser)
	}
}

func TestRequireAdmin_NonAdmin(t *testing.T) {
	ctx := context.Background()
	a, store := newTestAuth(t)

	u := &db.User{Email: "ivan@example.com", Name: "Ivan", IsAdmin: false}
	_ = store.CreateUser(ctx, u)
	sess, _ := a.NewSession(ctx, u.ID)

	req := httptest.NewRequest(http.MethodGet, "/-/dashboard/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()

	auth.RequireAdmin(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin, got %d", w.Code)
	}
}

func TestRequireAdmin_Admin(t *testing.T) {
	ctx := context.Background()
	a, store := newTestAuth(t)

	u := &db.User{Email: "judy@example.com", Name: "Judy", IsAdmin: true}
	_ = store.CreateUser(ctx, u)
	sess, _ := a.NewSession(ctx, u.ID)

	req := httptest.NewRequest(http.MethodGet, "/-/dashboard/admin/", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
	w := httptest.NewRecorder()

	auth.RequireAdmin(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for admin, got %d", w.Code)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := context.Background()
	a, store := newTestAuth(t)
	u := &db.User{Email: "ken@example.com", Name: "Ken"}
	_ = store.CreateUser(ctx, u)
	sess, _ := a.NewSession(ctx, u.ID)
	if sess.ExpiresAt.Before(time.Now().Add(29 * 24 * time.Hour)) {
		t.Errorf("session expiry too short: %v", sess.ExpiresAt)
	}
}
