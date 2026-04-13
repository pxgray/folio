package dashboard_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

// newAPITestServer creates a dashboard httptest.Server with one pre-created user.
// Returns the server, the store, and the created user.
func newAPITestServer(t *testing.T) (*httptest.Server, db.Store, *db.User) {
	t.Helper()

	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	hash, err := auth.HashPassword("secret")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ctx := context.Background()
	user := &db.User{
		Email:    "test@example.com",
		Password: hash,
		Name:     "Test",
	}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	authn := auth.New(store)
	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	return ts, store, user
}

func postLoginJSON(t *testing.T, ts *httptest.Server, email, password string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	resp, err := http.Post(ts.URL+"/-/api/v1/auth/login", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST login: %v", err)
	}
	return resp
}

func TestAPILoginValidCreds(t *testing.T) {
	ts, _, _ := newAPITestServer(t)

	resp := postLoginJSON(t, ts, "test@example.com", "secret")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "email") {
		t.Errorf("response body missing 'email' field: %s", bodyStr)
	}

	setCookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "session=") {
		t.Errorf("expected Set-Cookie to contain 'session=', got %q", setCookie)
	}
}

func TestAPILoginBadPassword(t *testing.T) {
	ts, _, _ := newAPITestServer(t)

	resp := postLoginJSON(t, ts, "test@example.com", "wrongpassword")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(bodyBytes), "error") {
		t.Errorf("expected body to contain 'error', got %s", string(bodyBytes))
	}
}

func TestAPILoginUnknownEmail(t *testing.T) {
	ts, _, _ := newAPITestServer(t)

	resp := postLoginJSON(t, ts, "nobody@example.com", "secret")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", resp.StatusCode)
	}
}

func TestAPIMeWithSession(t *testing.T) {
	ts, store, user := newAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/-/api/v1/auth/me", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.Header.Set("Cookie", "session="+sess.Token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(bodyBytes), user.Email) {
		t.Errorf("expected body to contain %q, got %s", user.Email, string(bodyBytes))
	}
}

func TestAPIMeWithoutSession(t *testing.T) {
	ts, _, _ := newAPITestServer(t)

	resp, err := http.Get(ts.URL + "/-/api/v1/auth/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func TestAPILogoutClearsCookie(t *testing.T) {
	ts, store, user := newAPITestServer(t)

	ctx := context.Background()
	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/-/api/v1/auth/logout", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /logout: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	setCookie := resp.Header.Get("Set-Cookie")
	if !strings.Contains(setCookie, "session=") {
		t.Errorf("expected Set-Cookie to contain 'session=', got %q", setCookie)
	}
	if !strings.Contains(setCookie, "Max-Age=0") && !strings.Contains(setCookie, "Max-Age=-1") {
		t.Errorf("expected Set-Cookie to contain Max-Age=0 or Max-Age=-1, got %q", setCookie)
	}
}
