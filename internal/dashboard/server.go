package dashboard

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
	"github.com/pxgray/folio/internal/web"
)

// Server accumulates dashboard HTTP handlers across phases 3-6.
type Server struct {
	dbStore       db.Store
	gitStore      *gitstore.Store // nil when setup not yet complete
	authn         *auth.Auth
	docSrv        *web.Server // nil when setup not yet complete
	tmplFS        embed.FS
	setupComplete bool

	setupTmpl *template.Template
	// additional templates added in later phases
}

// New creates a dashboard Server. gitStore and docSrv may be nil when
// setupComplete is false (setup-only mode).
func New(
	dbStore db.Store,
	gitStore *gitstore.Store,
	authn *auth.Auth,
	docSrv *web.Server,
	tmplFS embed.FS,
	setupComplete bool,
) *Server {
	s := &Server{
		dbStore:       dbStore,
		gitStore:      gitStore,
		authn:         authn,
		docSrv:        docSrv,
		tmplFS:        tmplFS,
		setupComplete: setupComplete,
	}
	s.setupTmpl = template.Must(
		template.ParseFS(tmplFS, "templates/setup.html"),
	)
	return s
}

// Handler returns a chi router mounting all active dashboard routes.
// When setupComplete is false only /-/setup routes are registered.
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Route("/-/setup", func(r chi.Router) {
		r.Get("/", s.handleSetupGet)
		r.Post("/", s.handleSetupPost)
	})
	// /-/auth, /-/dashboard, /-/api routes are added in later phases
	return r
}
