package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/gitstore"
)

const webhookCooldown = 30 * time.Second

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "host")
	owner := chi.URLParam(r, "owner")
	repo := chi.URLParam(r, "repo")

	gr, err := s.store.Get(host, owner, repo)
	if err != nil {
		if errors.Is(err, gitstore.ErrNotRegistered) {
			httpError(w, http.StatusNotFound, "repo not found")
			return
		}
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Read body (needed for HMAC verification and event type).
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read body")
		return
	}

	// Verify HMAC if a secret is configured for this repo.
	key := host + "/" + owner + "/" + repo
	secret := s.repoSecrets[key]
	if secret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifyGitHubSignature(secret, body, sig) {
			httpError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	s.webhookMu.Lock()
	if last, ok := s.webhookLimiter[key]; ok && time.Since(last) < webhookCooldown {
		s.webhookMu.Unlock()
		httpError(w, http.StatusTooManyRequests, "rate limited")
		return
	}
	s.webhookLimiter[key] = time.Now()
	s.webhookMu.Unlock()

	log.Printf("folio: webhook received for %s/%s/%s, fetching...", host, owner, repo)
	if err := gr.FetchNow(r.Context()); err != nil {
		log.Printf("folio: webhook fetch error for %s/%s/%s: %v", host, owner, repo, err)
		httpError(w, http.StatusInternalServerError, "fetch failed")
		return
	}
	log.Printf("folio: webhook fetch complete for %s/%s/%s", host, owner, repo)

	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprintln(w, "ok")
}

// verifyGitHubSignature checks the X-Hub-Signature-256 header value against
// the HMAC-SHA256 of the body with the given secret.
func verifyGitHubSignature(secret string, body []byte, sigHeader string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(sigHeader, prefix) {
		return false
	}
	gotHex := strings.TrimPrefix(sigHeader, prefix)
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(got, expected)
}
