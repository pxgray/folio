package web

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/gitstore"
)

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	filePath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
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

	hash, err := gr.ResolveRef(r.Context(), ref)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "ref not found: "+ref)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	blob, err := gr.ReadBlob(hash, filePath)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotFound) {
			httpError(w, http.StatusNotFound, "not found: "+filePath)
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Detect content type from first 512 bytes.
	ct := http.DetectContentType(blob)
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}
