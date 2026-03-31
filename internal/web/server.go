package web

import (
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
	"github.com/pxgray/folio/internal/config"
	"github.com/pxgray/folio/internal/gitstore"
)

// repoArtifactConfig holds per-repo artifact configuration.
type repoArtifactConfig struct {
	artifacts map[string]string
}

// Server is the Folio HTTP server.
type Server struct {
	store     *gitstore.Store
	docTmpl   *template.Template // base.html + doc.html
	indexTmpl *template.Template // base.html + index.html
	staticFS  fs.FS
	cfg       *config.Config

	repoTrusted        map[string]bool
	localTrusted       map[string]bool
	repoSecrets        map[string]string
	repoArtifactConfig map[string]repoArtifactConfig
	webhookLimiter     map[string]time.Time
	webhookMu          sync.Mutex

	rootArtifactDir   string
	rootArtifactFiles map[string]string
}

// New creates a Server. tmplFS should embed templates/*.html and staticFS
// should contain the static web assets (already sub-rooted at "static/").
func New(cfg *config.Config, store *gitstore.Store, tmplFS embed.FS, staticFS fs.FS) (*Server, error) {
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

	repoTrusted := make(map[string]bool, len(cfg.Repos))
	repoSecrets := make(map[string]string, len(cfg.Repos))
	repoArtifacts := make(map[string]repoArtifactConfig, len(cfg.Repos))
	for _, rc := range cfg.Repos {
		repoTrusted[rc.Key()] = rc.TrustedHTML
		repoSecrets[rc.Key()] = rc.WebhookSecret
		repoArtifacts[rc.Key()] = repoArtifactConfig{artifacts: rc.WebArtifacts}
	}

	localTrusted := make(map[string]bool, len(cfg.Locals))
	for _, lc := range cfg.Locals {
		localTrusted[lc.Label] = lc.TrustedHTML
	}

	return &Server{
		store:              store,
		docTmpl:            docTmpl,
		indexTmpl:          indexTmpl,
		staticFS:           staticFS,
		cfg:                cfg,
		repoTrusted:        repoTrusted,
		localTrusted:       localTrusted,
		repoSecrets:        repoSecrets,
		repoArtifactConfig: repoArtifacts,
		webhookLimiter:     make(map[string]time.Time),
		rootArtifactDir:    cfg.RootArtifacts.Dir,
		rootArtifactFiles:  cfg.RootArtifacts.Files,
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
