package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
)

// repoArtifactConfig holds per-repo artifact configuration.
type repoArtifactConfig struct {
	artifacts map[string]string
}

// Server is the Folio HTTP server.
type Server struct {
	store     *gitstore.Store
	dbStore   db.Store // for per-repo settings lookup
	docTmpl   *template.Template
	indexTmpl *template.Template
	staticFS  fs.FS

	mu                 sync.RWMutex
	repoTrusted        map[string]bool
	repoSecrets        map[string]string
	repoArtifactConfig map[string]repoArtifactConfig

	webhookLimiter map[string]time.Time
	webhookMu      sync.Mutex
}

// New creates a Server. tmplFS should embed templates/*.html and staticFS
// should contain the static web assets (already sub-rooted at "static/").
func New(dbStore db.Store, gitStore *gitstore.Store, tmplFS embed.FS, staticFS fs.FS) (*Server, error) {
	funcMap := template.FuncMap{
		"formatSize": formatSize,
		"not":        func(b bool) bool { return !b },
	}

	docTmpl, err := template.New("").Funcs(funcMap).ParseFS(tmplFS, "templates/base.html", "templates/doc.html")
	if err != nil {
		return nil, fmt.Errorf("parse doc template: %w", err)
	}
	indexTmpl, err := template.New("").Funcs(funcMap).ParseFS(tmplFS, "templates/base.html", "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("parse index template: %w", err)
	}

	ctx := context.Background()
	allRepos, err := dbStore.ListAllRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("web.New: list repos: %w", err)
	}

	repoTrusted := make(map[string]bool, len(allRepos))
	repoSecrets := make(map[string]string, len(allRepos))
	repoArtifacts := make(map[string]repoArtifactConfig, len(allRepos))
	for _, r := range allRepos {
		key := r.Host + "/" + r.RepoOwner + "/" + r.RepoName
		repoTrusted[key] = r.TrustedHTML
		repoSecrets[key] = r.WebhookSecret
		repoArtifacts[key] = repoArtifactConfig{artifacts: nil}
	}

	return &Server{
		store:              gitStore,
		dbStore:            dbStore,
		docTmpl:            docTmpl,
		indexTmpl:          indexTmpl,
		staticFS:           staticFS,
		repoTrusted:        repoTrusted,
		repoSecrets:        repoSecrets,
		repoArtifactConfig: repoArtifacts,
		webhookLimiter:     make(map[string]time.Time),
	}, nil
}

// Handler returns the http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(securityHeadersMiddleware)
	r.Use(loggingMiddleware)

	r.Handle("/-/static/*", http.StripPrefix("/-/static/", http.FileServer(http.FS(s.staticFS))))

	r.Get("/", s.handleIndex)

	for _, name := range artifactNames {
		r.Get("/"+name, s.handleRootArtifact)
	}

	r.Post("/{host}/{owner}/{repo}/-/webhook", s.handleWebhook)
	r.Get("/{host}/{owner}/{repo}/-/raw/*", s.handleRaw)

	for _, name := range artifactNames {
		r.Get("/{host}/{owner}/{repo}/"+name, s.handleRepoArtifact)
	}

	r.Get("/{host}/{owner}/{repo}", s.handleDoc)
	r.Get("/{host}/{owner}/{repo}/*", s.handleDoc)

	r.Get("/local/{label}", s.handleLocalDoc)
	r.Get("/local/{label}/*", s.handleLocalDoc)

	return r
}

// breadcrumb is a single crumb in the page nav trail.
type breadcrumb struct {
	Name string
	URL  string
}

// buildBreadcrumbs constructs breadcrumbs for a given repo path.
func buildBreadcrumbs(repoBase, filePath, ref string) []breadcrumb {
	crumbs := []breadcrumb{{Name: repoBase[1:], URL: repoBase + refQuery(ref)}}
	if filePath == "" {
		return crumbs
	}
	parts := strings.Split(filePath, "/")
	for i, part := range parts {
		url := repoBase + "/" + strings.Join(parts[:i+1], "/") + refQuery(ref)
		crumbs = append(crumbs, breadcrumb{Name: part, URL: url})
	}
	return crumbs
}

func refQuery(ref string) string {
	if ref == "" {
		return ""
	}
	return "?ref=" + url.QueryEscape(ref)
}

func formatSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'")
		next.ServeHTTP(w, r)
	})
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func httpError(w http.ResponseWriter, code int, msg string) {
	http.Error(w, msg, code)
}
