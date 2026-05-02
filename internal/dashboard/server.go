package dashboard

import (
	"embed"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"

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
	staticFS      fs.FS
	setupComplete bool

	setupTmpl *template.Template
	loginTmpl *template.Template
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
	staticFS fs.FS,
	setupComplete bool,
) *Server {
	s := &Server{
		dbStore:       dbStore,
		gitStore:      gitStore,
		authn:         authn,
		docSrv:        docSrv,
		tmplFS:        tmplFS,
		staticFS:      staticFS,
		setupComplete: setupComplete,
	}
	s.setupTmpl = template.Must(
		template.ParseFS(tmplFS, "templates/setup.html"),
	)
	s.loginTmpl = template.Must(
		template.ParseFS(tmplFS, "templates/login.html"),
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
	r.Get("/-/auth/login", s.handleLoginGet)
	r.Post("/-/auth/login", s.handleFormLogin)
	r.Post("/-/auth/logout", s.handleFormLogout)
	r.Get("/-/auth/github", s.handleGitHubOAuth)
	r.Get("/-/auth/github/callback", s.handleGitHubCallback)
	r.Get("/-/auth/google", s.handleGoogleOAuth)
	r.Get("/-/auth/google/callback", s.handleGoogleCallback)
	r.Post("/-/api/v1/auth/login", s.handleAPILogin)
	r.Post("/-/api/v1/auth/logout", s.handleAPILogout)
	r.Get("/-/api/v1/auth/me", auth.RequireAuth(s.authn)(http.HandlerFunc(s.handleAPIMe)).ServeHTTP)
	r.Route("/-/api/v1/repos", func(r chi.Router) {
		r.Use(auth.RequireAuth(s.authn))
		r.Get("/", s.handleAPIListRepos)
		r.Post("/", s.handleAPICreateRepo)
		r.Get("/{id}", s.handleAPIGetRepo)
		r.Patch("/{id}", s.handleAPIUpdateRepo)
		r.Delete("/{id}", s.handleAPIDeleteRepo)
		r.Post("/{id}/sync", s.handleAPIRepoSync)
	})
	r.Route("/-/api/v1/admin", func(r chi.Router) {
		r.Use(auth.RequireAdmin(s.authn))
		r.Get("/users", s.handleAdminListUsers)
		r.Patch("/users/{id}", s.handleAdminUpdateUser)
		r.Delete("/users/{id}", s.handleAdminDeleteUser)
		r.Get("/settings", s.handleAdminGetSettings)
		r.Patch("/settings", s.handleAdminPatchSettings)
	})
	r.Route("/-/dashboard", func(r chi.Router) {
		r.Use(auth.RequireAuth(s.authn), withCSRFToken, requireCSRF)
		r.Get("/", s.handleRepoList)
		r.Get("/repos/new", s.handleRepoNew)
		r.Post("/repos/new", s.handleRepoCreate)
		r.Get("/repos/{id}", s.handleRepoEdit)
		r.Post("/repos/{id}", s.handleRepoUpdate)
		r.Post("/repos/{id}/delete", s.handleRepoDelete)
		r.Post("/repos/{id}/sync", s.handleRepoSync)
		r.Get("/settings", s.handleSettingsGet)
		r.Post("/settings", s.handleSettingsPost)
		r.Post("/settings/unlink/{provider}", s.handleOAuthUnlink)
		r.Route("/admin", func(r chi.Router) {
			r.Use(auth.RequireAdmin(s.authn))
			r.Get("/", s.handleAdminUsersPage)
			r.Get("/users/{id}", s.handleAdminUserEditPage)
			r.Post("/users/{id}", s.handleAdminUserEditPost)
			r.Post("/users/{id}/delete", s.handleAdminUserDeletePost)
			r.Post("/users/{id}/toggle-admin", s.handleAdminToggleAdmin)
			r.Get("/settings", s.handleAdminSettingsPage)
			r.Post("/settings", s.handleAdminSettingsPost)
		})
	})
	return r
}

// setFlash stores a one-time flash message in a short-lived cookie.
func setFlash(w http.ResponseWriter, msg string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "_flash",
		Value:    url.QueryEscape(msg),
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

// getFlash reads and clears the flash cookie, returning the message (or "").
func getFlash(w http.ResponseWriter, r *http.Request) string {
	c, err := r.Cookie("_flash")
	if err != nil {
		return ""
	}
	http.SetCookie(w, &http.Cookie{Name: "_flash", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode})
	v, _ := url.QueryUnescape(c.Value)
	return v
}
