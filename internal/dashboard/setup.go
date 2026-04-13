package dashboard

import (
	"net/http"
	"strings"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

type setupPageData struct {
	Error    string
	Addr     string
	CacheDir string
	Name     string
	Email    string
}

func (s *Server) handleSetupGet(w http.ResponseWriter, r *http.Request) {
	if s.setupComplete {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	// Also check DB in case setup was completed in a prior run.
	complete, err := s.dbStore.IsSetupComplete(r.Context())
	if err == nil && complete {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	s.renderSetup(w, setupPageData{})
}

func (s *Server) handleSetupPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	addr := strings.TrimSpace(r.FormValue("addr"))
	cacheDir := strings.TrimSpace(r.FormValue("cache_dir"))

	if name == "" {
		s.renderSetup(w, setupPageData{
			Error:    "Name is required",
			Addr:     addr,
			CacheDir: cacheDir,
			Email:    email,
		})
		return
	}
	if email == "" {
		s.renderSetup(w, setupPageData{
			Error:    "Email is required",
			Addr:     addr,
			CacheDir: cacheDir,
			Name:     name,
		})
		return
	}
	if len(password) < 8 {
		s.renderSetup(w, setupPageData{
			Error:    "Password must be at least 8 characters (required)",
			Addr:     addr,
			CacheDir: cacheDir,
			Name:     name,
			Email:    email,
		})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := s.dbStore.CreateUser(ctx, &db.User{
		Name:     name,
		Email:    email,
		Password: hash,
		IsAdmin:  true,
	}); err != nil {
		s.renderSetup(w, setupPageData{
			Error:    "Could not create user: " + err.Error(),
			Addr:     addr,
			CacheDir: cacheDir,
			Name:     name,
			Email:    email,
		})
		return
	}

	if addr != "" {
		_ = s.dbStore.UpsertSetting(ctx, "addr", addr)
	}
	if cacheDir != "" {
		_ = s.dbStore.UpsertSetting(ctx, "cache_dir", cacheDir)
	}
	if err := s.dbStore.UpsertSetting(ctx, "setup_complete", "true"); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	s.setupComplete = true

	http.Redirect(w, r, "/-/auth/login", http.StatusSeeOther)
}

func (s *Server) renderSetup(w http.ResponseWriter, data setupPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.setupTmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
