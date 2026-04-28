package web

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
)

func TestVerifyGitHubSignature(t *testing.T) {
	secret := "mysecret"
	body := []byte(`{"action":"push"}`)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	validSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyGitHubSignature(secret, body, validSig) {
		t.Error("expected valid signature to pass")
	}
	if verifyGitHubSignature(secret, body, "sha256=badhex") {
		t.Error("expected invalid hex to fail")
	}
	if verifyGitHubSignature(secret, body, "invalid-prefix") {
		t.Error("expected missing prefix to fail")
	}
	if verifyGitHubSignature(secret, body, "") {
		t.Error("expected empty sig to fail")
	}
	if verifyGitHubSignature("wrongsecret", body, validSig) {
		t.Error("expected wrong secret to fail")
	}
}

func TestWebhook_RateLimiting(t *testing.T) {
	dbStore, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer dbStore.Close()

	cacheDir := t.TempDir()
	gitStore := gitstore.New(cacheDir, 5*time.Minute)
	staticFS, _ := fs.Sub(assets.StaticFS, "static")

	srv, err := New(dbStore, gitStore, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatal(err)
	}

	// Create a user and repo with webhook secret
	user := &db.User{Email: "admin@test.com", Name: "Admin", Password: "admin123", IsAdmin: true}
	if err := dbStore.CreateUser(t.Context(), user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	repo := &db.Repo{OwnerID: user.ID, Host: "github.com", RepoOwner: "test", RepoName: "docs", WebhookSecret: "secret"}
	if err := dbStore.CreateRepo(t.Context(), repo); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Pre-create the bare repo directory so AddRepo opens instead of cloning
	bareDir := cacheDir + "/github.com/test/docs"
	if err := initBareRepo(bareDir); err != nil {
		t.Fatalf("initBareRepo: %v", err)
	}

	// Add to gitstore (will open existing bare repo)
	if err := gitStore.AddRepo(t.Context(), gitstore.RepoEntry{
		Host:      "github.com",
		Owner:     "test",
		Name:      "docs",
		RemoteURL: "https://github.com/test/docs.git",
	}); err != nil {
		t.Fatalf("AddRepo: %v", err)
	}

	handler := srv.Handler()

	body := []byte(`{"action":"push"}`)
	mac := hmac.New(sha256.New, []byte("secret"))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	// First request (may fail on fetch, but should not be rate limited)
	req := httptest.NewRequest("POST", "/github.com/test/docs/-/webhook", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code == http.StatusTooManyRequests {
		t.Error("first request should not be rate limited")
	}

	// Immediate second request should be rate limited
	req2 := httptest.NewRequest("POST", "/github.com/test/docs/-/webhook", bytes.NewReader(body))
	req2.Header.Set("X-Hub-Signature-256", sig)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)

	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected 429, got %d", w2.Code)
	}

	// After cooldown, request should not be rate limited again
	time.Sleep(webhookCooldown + 100*time.Millisecond)
	req3 := httptest.NewRequest("POST", "/github.com/test/docs/-/webhook", bytes.NewReader(body))
	req3.Header.Set("X-Hub-Signature-256", sig)
	req3.Header.Set("Content-Type", "application/json")
	w3 := httptest.NewRecorder()
	handler.ServeHTTP(w3, req3)

	if w3.Code == http.StatusTooManyRequests {
		t.Error("request after cooldown should not be rate limited")
	}
}

// initBareRepo initializes a minimal bare git repo at the given path.
func initBareRepo(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path+"/HEAD", []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(path+"/objects", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(path+"/refs/heads", 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(path+"/refs/tags", 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path+"/config", []byte("[core]\n\trepositoryformatversion = 0\n"), 0o644); err != nil {
		return err
	}
	return nil
}
