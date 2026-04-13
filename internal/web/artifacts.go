package web

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/gitstore"
)

var artifactNames = []string{"llms.txt", "llms-full.txt", "robots.txt", "sitemap.xml"}

var artifactContentType = map[string]string{
	"llms.txt":      "text/plain; charset=utf-8",
	"llms-full.txt": "text/plain; charset=utf-8",
	"robots.txt":    "text/plain; charset=utf-8",
	"sitemap.xml":   "text/xml; charset=utf-8",
}

func (s *Server) handleRootArtifact(w http.ResponseWriter, r *http.Request) {
	// Root artifacts are not yet supported without a config.Config.
	// The route is registered to reserve the URL namespace.
	httpError(w, http.StatusNotFound, "artifact not found")
}

func (s *Server) handleRepoArtifact(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	name := chi.URLParam(r, "name")
	ref := r.URL.Query().Get("ref")

	gr, err := s.store.Get(host, owner, repo)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotRegistered) {
			httpError(w, http.StatusNotFound, "repo not found")
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	key := host + "/" + owner + "/" + repo
	s.mu.RLock()
	repoCfg := s.repoArtifactConfig[key]
	s.mu.RUnlock()

	gitPath := s.resolveArtifactPath(repoCfg, name)
	if gitPath == "" {
		httpError(w, http.StatusNotFound, "artifact not configured")
		return
	}

	hash, err := gr.ResolveRef(r.Context(), ref)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	blob, err := gr.ReadBlob(hash, gitPath)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "artifact not found in repo")
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ct := artifactContentType[name]
	if ct == "" {
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(blob)
}

func (s *Server) resolveArtifactPath(cfg repoArtifactConfig, name string) string {
	if cfg.artifacts == nil {
		return name
	}

	p, ok := cfg.artifacts[name]
	if !ok {
		return name
	}
	return p
}
