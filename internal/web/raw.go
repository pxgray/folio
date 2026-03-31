package web

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/gitstore"
)

const maxRawSize = 10 * 1024 * 1024

var allowedExtensions = map[string]bool{
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".webp":  true,
	".svg":   true,
	".ico":   true,
	".bmp":   true,
	".tiff":  true,
	".avif":  true,
	".woff":  true,
	".woff2": true,
	".ttf":   true,
	".eot":   true,
	".otf":   true,
	".css":   true,
	".pdf":   true,
	".json":  true,
	".xml":   true,
	".yaml":  true,
	".yml":   true,
	".csv":   true,
	".tsv":   true,
	".mp4":   true,
	".webm":  true,
	".ogg":   true,
	".mp3":   true,
	".wav":   true,
}

func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")
	filePath := strings.TrimPrefix(chi.URLParam(r, "*"), "/")
	ref := r.URL.Query().Get("ref")

	if !isExtensionAllowed(filePath) {
		httpError(w, http.StatusNotFound, "not found")
		return
	}

	if hasBlockedPrefix(filePath) {
		httpError(w, http.StatusNotFound, "not found")
		return
	}

	if strings.Contains(filePath, "..") {
		httpError(w, http.StatusNotFound, "not found")
		return
	}

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

	if len(blob) > maxRawSize {
		httpError(w, http.StatusNotFound, "file too large")
		return
	}

	ct := http.DetectContentType(blob)
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".html", ".htm", ".xhtml", ".svg", ".js", ".mjs":
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(blob)
}

func isExtensionAllowed(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return allowedExtensions[ext]
}

func hasBlockedPrefix(path string) bool {
	parts := strings.Split(path, "/")
	for _, part := range parts {
		if strings.HasPrefix(part, ".") || strings.HasPrefix(part, "_") {
			return true
		}
	}
	return false
}
