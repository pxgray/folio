package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/config"
	"github.com/pxgray/folio/internal/gitstore"
)

// Server is the Folio HTTP server.
type Server struct {
	store    *gitstore.Store
	tmpl     *template.Template
	staticFS fs.FS
	cfg      *config.Config
}

// New creates a Server. tmplFS should embed templates/*.html and staticFS
// should contain the static web assets (already sub-rooted at "static/").
func New(cfg *config.Config, store *gitstore.Store, tmplFS embed.FS, staticFS fs.FS) (*Server, error) {
	funcMap := template.FuncMap{
		"formatSize": formatSize,
		"not":        func(b bool) bool { return !b },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(tmplFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}

	return &Server{
		store:    store,
		tmpl:     tmpl,
		staticFS: staticFS,
		cfg:      cfg,
	}, nil
}

// Handler returns the http.Handler with all routes registered.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(loggingMiddleware)

	// Static assets under /-/static/ (clear namespace, can't conflict with /{host}/...).
	r.Handle("/-/static/*", http.StripPrefix("/-/static/", http.FileServer(http.FS(s.staticFS))))

	// Root index.
	r.Get("/", s.handleIndex)

	// Repo routes.
	r.Post("/{host}/{owner}/{repo}/-/webhook", s.handleWebhook)
	r.Get("/{host}/{owner}/{repo}/-/raw/*", s.handleRaw)
	r.Get("/{host}/{owner}/{repo}", s.handleDoc)
	r.Get("/{host}/{owner}/{repo}/*", s.handleDoc)

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
	return "?ref=" + ref
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
