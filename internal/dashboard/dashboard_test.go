package dashboard_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/dashboard"
	"github.com/pxgray/folio/internal/db"
)

// newTestDashboard creates a dashboard server backed by an in-memory DB.
// gitStore and docSrv are nil (setup-only mode).
func newTestDashboard(t *testing.T) (*httptest.Server, db.Store) {
	t.Helper()
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	authn := auth.New(store)
	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, nil, false)
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

func TestHandlerNonNil(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer store.Close()

	authn := auth.New(store)
	srv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, nil, false)
	if srv.Handler() == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestSetupGet_NotComplete_Returns200(t *testing.T) {
	ts, _ := newTestDashboard(t)
	resp, err := http.Get(ts.URL + "/-/setup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestSetupGet_AlreadyComplete_RedirectsToRoot(t *testing.T) {
	ts, store := newTestDashboard(t)
	ctx := context.Background()
	if err := store.UpsertSetting(ctx, "setup_complete", "true"); err != nil {
		t.Fatal(err)
	}

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(ts.URL + "/-/setup")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/-/dashboard/" {
		t.Fatalf("want Location /-/dashboard/, got %q", loc)
	}
}

func TestSetupPost_ValidInput_CreatesUserAndRedirects(t *testing.T) {
	ts, store := newTestDashboard(t)
	ctx := context.Background()

	resp, err := http.PostForm(ts.URL+"/-/setup", url.Values{
		"name":      {"Alice Admin"},
		"email":     {"alice@example.com"},
		"password":  {"securepass1"},
		"addr":      {":9090"},
		"cache_dir": {"/tmp/folio-test"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	complete, err := store.IsSetupComplete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("setup_complete should be true after valid POST")
	}

	addr, err := store.GetSetting(ctx, "addr")
	if err != nil {
		t.Fatal(err)
	}
	if addr != ":9090" {
		t.Fatalf("want addr :9090, got %q", addr)
	}
}

func TestSetupPost_MissingName_ReturnsFormWithError(t *testing.T) {
	ts, _ := newTestDashboard(t)

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.PostForm(ts.URL+"/-/setup", url.Values{
		"name":     {""},
		"email":    {"alice@example.com"},
		"password": {"securepass1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (re-rendered form), got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "required") {
		t.Error("expected error message in re-rendered form")
	}
}

func TestMainSmoke_RedirectsToSetupWhenNotConfigured(t *testing.T) {
	store, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	authn := auth.New(store)
	dashSrv := dashboard.New(store, nil, authn, nil, assets.TemplateFS, nil, false)
	dashHandler := dashSrv.Handler()

	combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/-/setup") ||
			strings.HasPrefix(p, "/-/auth") ||
			strings.HasPrefix(p, "/-/dashboard") ||
			strings.HasPrefix(p, "/-/api") {
			dashHandler.ServeHTTP(w, r)
			return
		}
		// docHandler is nil (setup not complete)
		http.Redirect(w, r, "/-/setup", http.StatusSeeOther)
	})

	combinedTS := httptest.NewServer(combined)
	defer combinedTS.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(combinedTS.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("want 303, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/-/setup" {
		t.Fatalf("want Location /-/setup, got %q", loc)
	}
}
