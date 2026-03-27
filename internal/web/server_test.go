package web_test

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
	_ "github.com/go-git/go-git/v5/plumbing/transport/file" // register file:// transport
	"github.com/pxgray/folio/internal/assets"
	"github.com/pxgray/folio/internal/config"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/web"
)

// makeTestBareRepo creates a temp bare repo with a README.md and docs/index.md.
func makeTestBareRepo(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()

	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := work.Worktree()

	writeTestFile(t, filepath.Join(workDir, "README.md"), "# Hello\n\nWelcome to Folio.\n")
	writeTestFile(t, filepath.Join(workDir, "docs/index.md"), "# Docs\n\n[Setup](setup.md)\n")
	writeTestFile(t, filepath.Join(workDir, "docs/setup.md"), "# Setup\n\nRun `folio config.toml`.\n")
	writeTestFile(t, filepath.Join(workDir, "static/logo.png"), "\x89PNG\r\n\x1a\n")

	_ = wt.AddGlob(".")
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	git.PlainInit(bareDir, true)
	work.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{bareDir}})
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	return bareDir
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func makeTestServer(t *testing.T, bareDir string) *httptest.Server {
	t.Helper()
	cacheDir := t.TempDir()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Cache:  config.CacheConfig{Dir: cacheDir},
		Repos: []config.RepoConfig{
			{
				Host:   "example.com",
				Owner:  "testuser",
				Repo:   "testrepo",
				Remote: "file://" + bareDir,
			},
		},
	}

	store := gitstore.New(cfg)
	if err := store.EnsureCloned(t.Context()); err != nil {
		t.Fatalf("EnsureCloned: %v", err)
	}

	staticFS, _ := fs.Sub(assets.StaticFS, "static")
	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	return httptest.NewServer(srv.Handler())
}

func TestHandleDoc_MarkdownRender(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/testrepo/README.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}

	// Verify the actual Markdown content is rendered, not an empty/wrong template.
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, "Welcome to Folio") {
		t.Errorf("rendered page missing expected content; got %d bytes, body snippet: %q",
			len(bodyStr), bodyStr[:min(300, len(bodyStr))])
	}
	// The repo-list content from index.html must NOT appear on a doc page.
	if strings.Contains(bodyStr, "repo-list") {
		t.Errorf("doc page is incorrectly rendering index.html content block")
	}
}

func TestHandleDoc_DirectoryListing(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/testrepo")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHandleRaw(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/testrepo/-/raw/static/logo.png")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/png") {
		t.Errorf("Content-Type = %q, want image/png", ct)
	}
}

func TestHandleDoc_NotFound(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/testrepo/nonexistent.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleDoc_RepoNotRegistered(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/nobody/norepo/README.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleIndex(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHandleWebhook_NoSecret(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/example.com/testuser/testrepo/-/webhook", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

// makeTestBareRepoWithNav creates a temp bare repo that includes a folio.yml nav
// and a docs/guide.md file, so the active nav indicator can be tested.
func makeTestBareRepoWithNav(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()

	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := work.Worktree()

	writeTestFile(t, filepath.Join(workDir, "docs/guide.md"), "# Guide\n\nSome guidance.\n")
	writeTestFile(t, filepath.Join(workDir, "folio.yml"),
		"title: Test\nnav:\n  - Guide: docs/guide.md\n")

	_ = wt.AddGlob(".")
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	git.PlainInit(bareDir, true)
	work.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{bareDir}})
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	return bareDir
}

func makeTestServerForNav(t *testing.T, bareDir string) *httptest.Server {
	t.Helper()
	cacheDir := t.TempDir()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Cache:  config.CacheConfig{Dir: cacheDir},
		Repos: []config.RepoConfig{
			{
				Host:   "example.com",
				Owner:  "testuser",
				Repo:   "navrepo",
				Remote: "file://" + bareDir,
			},
		},
	}

	store := gitstore.New(cfg)
	if err := store.EnsureCloned(t.Context()); err != nil {
		t.Fatalf("EnsureCloned: %v", err)
	}

	staticFS, _ := fs.Sub(assets.StaticFS, "static")
	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	return httptest.NewServer(srv.Handler())
}

func TestHandleDoc_ActiveNavItem(t *testing.T) {
	bareDir := makeTestBareRepoWithNav(t)
	ts := makeTestServerForNav(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/navrepo/docs/guide.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	wantActive := `href="/example.com/testuser/navrepo/docs/guide.md" class="active"`
	if !strings.Contains(bodyStr, wantActive) {
		t.Errorf("rendered page missing active nav class on expected link; body snippet: %q",
			bodyStr[:min(500, len(bodyStr))])
	}
}

func makeTestLocalDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "README.md"), "# Local Hello\n\nThis is a local repo.\n")
	writeTestFile(t, filepath.Join(dir, "docs", "guide.md"), "# Guide\n\nSome content.\n")
	return dir
}

func makeTestServerWithLocal(t *testing.T, localDir string) *httptest.Server {
	t.Helper()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Cache:  config.CacheConfig{Dir: t.TempDir()},
		Locals: []config.LocalConfig{
			{Label: "testlocal", Path: localDir},
		},
	}

	store := gitstore.New(cfg)
	if err := store.OpenLocals(); err != nil {
		t.Fatalf("OpenLocals: %v", err)
	}

	staticFS, _ := fs.Sub(assets.StaticFS, "static")
	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	return httptest.NewServer(srv.Handler())
}

func TestHandleLocalDoc_MarkdownRender(t *testing.T) {
	localDir := makeTestLocalDir(t)
	ts := makeTestServerWithLocal(t, localDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/local/testlocal/README.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "This is a local repo") {
		t.Errorf("body missing expected content; got: %q", string(body)[:min(300, len(body))])
	}
}

func TestHandleLocalDoc_DirectoryListing(t *testing.T) {
	localDir := makeTestLocalDir(t)
	ts := makeTestServerWithLocal(t, localDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/local/testlocal")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestHandleLocalDoc_NotFound(t *testing.T) {
	localDir := makeTestLocalDir(t)
	ts := makeTestServerWithLocal(t, localDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/local/testlocal/nonexistent.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestHandleLocalDoc_LabelNotRegistered(t *testing.T) {
	localDir := makeTestLocalDir(t)
	ts := makeTestServerWithLocal(t, localDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/local/nolabel/README.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	bareDir := makeTestBareRepo(t)
	ts := makeTestServer(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/testrepo/README.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	want := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Content-Security-Policy": "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'",
	}
	for header, wantVal := range want {
		if got := resp.Header.Get(header); got != wantVal {
			t.Errorf("header %s = %q, want %q", header, got, wantVal)
		}
	}
}

func makeTestBareRepoWithHTML(t *testing.T) string {
	t.Helper()
	workDir := t.TempDir()
	bareDir := t.TempDir()

	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := work.Worktree()

	writeTestFile(t, filepath.Join(workDir, "README.md"), "# Hello\n")
	writeTestFile(t, filepath.Join(workDir, "page.html"), "<html><body><script>alert(1)</script></body></html>")

	_ = wt.AddGlob(".")
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	git.PlainInit(bareDir, true)
	work.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{bareDir}})
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}
	return bareDir
}

func makeTestServerForHTML(t *testing.T, bareDir string) *httptest.Server {
	t.Helper()
	cacheDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Cache:  config.CacheConfig{Dir: cacheDir},
		Repos: []config.RepoConfig{
			{
				Host:   "example.com",
				Owner:  "testuser",
				Repo:   "htmlrepo",
				Remote: "file://" + bareDir,
			},
		},
	}
	store := gitstore.New(cfg)
	if err := store.EnsureCloned(t.Context()); err != nil {
		t.Fatalf("EnsureCloned: %v", err)
	}
	staticFS, _ := fs.Sub(assets.StaticFS, "static")
	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return httptest.NewServer(srv.Handler())
}

func TestHandleRaw_HTMLServedAsPlainText(t *testing.T) {
	bareDir := makeTestBareRepoWithHTML(t)
	ts := makeTestServerForHTML(t, bareDir)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/htmlrepo/-/raw/page.html")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain for .html files", ct)
	}
}

func TestHandleDoc_XSSStripped_Untrusted(t *testing.T) {
	workDir := t.TempDir()
	bareDir := t.TempDir()

	work, err := git.PlainInit(workDir, false)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := work.Worktree()
	writeTestFile(t, filepath.Join(workDir, "xss.md"),
		"# Danger\n\n<script>alert('xss')</script>\n")
	_ = wt.AddGlob(".")
	_, err = wt.Commit("init", &git.CommitOptions{
		Author: &object.Signature{Name: "t", Email: "t@t.com"},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	git.PlainInit(bareDir, true)
	work.CreateRemote(&gitconfig.RemoteConfig{Name: "origin", URLs: []string{bareDir}})
	if err := work.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("push: %v", err)
	}

	cacheDir := t.TempDir()
	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Cache:  config.CacheConfig{Dir: cacheDir},
		Repos: []config.RepoConfig{{
			Host: "example.com", Owner: "testuser", Repo: "xssrepo",
			Remote: "file://" + bareDir,
			// TrustedHTML omitted → defaults to false
		}},
	}
	store := gitstore.New(cfg)
	if err := store.EnsureCloned(t.Context()); err != nil {
		t.Fatalf("EnsureCloned: %v", err)
	}
	staticFS, _ := fs.Sub(assets.StaticFS, "static")
	srv, err := web.New(cfg, store, assets.TemplateFS, staticFS)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/example.com/testuser/xssrepo/xss.md")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	// Check the specific XSS payload — not just any <script> (the page template has one for theming).
	if strings.Contains(string(body), "<script>alert") {
		t.Errorf("XSS script tag not escaped in untrusted mode, body snippet: %q",
			string(body)[:min(500, len(body))])
	}
}
