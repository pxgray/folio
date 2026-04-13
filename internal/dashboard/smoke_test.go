package dashboard_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

func TestAPIRoutesSmoke(t *testing.T) {
	// Set up server with in-memory SQLite, no real git.
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	hash, err := auth.HashPassword("smokepass")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	user := &db.User{Email: "smoke@example.com", Password: hash, Name: "Smoke"}
	if err := store.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	authn := auth.New(store)
	sess, err := authn.NewSession(ctx, user.ID)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	sessionCookie := &http.Cookie{Name: "session", Value: sess.Token}

	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)

	// Use a client that does NOT follow redirects so 401 responses are visible.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	type testCase struct {
		name       string
		method     string
		path       string
		body       string
		authed     bool
		wantStatus int
	}

	cases := []testCase{
		// --- Unauthenticated ---
		{
			name:       "GET /-/auth/login unauthenticated",
			method:     http.MethodGet,
			path:       "/-/auth/login",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST /-/api/v1/auth/login empty body",
			method:     http.MethodPost,
			path:       "/-/api/v1/auth/login",
			body:       `{}`,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:       "GET /-/api/v1/auth/me unauthenticated",
			method:     http.MethodGet,
			path:       "/-/api/v1/auth/me",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "GET /-/api/v1/repos unauthenticated",
			method:     http.MethodGet,
			path:       "/-/api/v1/repos",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "POST /-/api/v1/repos unauthenticated",
			method:     http.MethodPost,
			path:       "/-/api/v1/repos",
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "GET /-/api/v1/repos/1 unauthenticated",
			method:     http.MethodGet,
			path:       "/-/api/v1/repos/1",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "PATCH /-/api/v1/repos/1 unauthenticated",
			method:     http.MethodPatch,
			path:       "/-/api/v1/repos/1",
			body:       `{}`,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "DELETE /-/api/v1/repos/1 unauthenticated",
			method:     http.MethodDelete,
			path:       "/-/api/v1/repos/1",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "POST /-/api/v1/repos/1/sync unauthenticated",
			method:     http.MethodPost,
			path:       "/-/api/v1/repos/1/sync",
			wantStatus: http.StatusUnauthorized,
		},

		// --- Authenticated ---
		{
			name:       "GET /-/api/v1/auth/me authenticated",
			method:     http.MethodGet,
			path:       "/-/api/v1/auth/me",
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /-/api/v1/repos authenticated",
			method:     http.MethodGet,
			path:       "/-/api/v1/repos",
			authed:     true,
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST /-/api/v1/repos authenticated valid body",
			method:     http.MethodPost,
			path:       "/-/api/v1/repos",
			body:       `{"host":"github.com","owner":"acme","repo":"smoke-test"}`,
			authed:     true,
			wantStatus: http.StatusAccepted,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if tc.body != "" {
				bodyReader = strings.NewReader(tc.body)
			} else {
				bodyReader = strings.NewReader("")
			}

			req, err := http.NewRequest(tc.method, ts.URL+tc.path, bodyReader)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			if tc.authed {
				req.AddCookie(sessionCookie)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("Do request: %v", err)
			}
			resp.Body.Close()

			if resp.StatusCode != tc.wantStatus {
				t.Errorf("%s %s: want status %d, got %d",
					tc.method, tc.path, tc.wantStatus, resp.StatusCode)
			}
		})
	}
}
