package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
)

// createRepoReq is the JSON body for POST /-/api/v1/repos.
type createRepoReq struct {
	Host          string `json:"host"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	RemoteURL     string `json:"remote_url"`
	WebhookSecret string `json:"webhook_secret"`
	TrustedHTML   bool   `json:"trusted_html"`
}

// repoResp is the JSON-safe representation of a db.Repo.
type repoResp struct {
	ID            int64     `json:"id"`
	OwnerID       int64     `json:"owner_id"`
	Host          string    `json:"host"`
	RepoOwner     string    `json:"repo_owner"`
	RepoName      string    `json:"repo_name"`
	RemoteURL     string    `json:"remote_url"`
	WebhookSecret string    `json:"webhook_secret"`
	TrustedHTML   bool      `json:"trusted_html"`
	StaleTTLSecs  int64     `json:"stale_ttl_secs"`
	Status        string    `json:"status"`
	StatusMsg     string    `json:"status_msg"`
	CreatedAt     time.Time `json:"created_at"`
}

func toRepoResp(r *db.Repo) repoResp {
	return repoResp{
		ID:            r.ID,
		OwnerID:       r.OwnerID,
		Host:          r.Host,
		RepoOwner:     r.RepoOwner,
		RepoName:      r.RepoName,
		RemoteURL:     r.RemoteURL,
		WebhookSecret: r.WebhookSecret,
		TrustedHTML:   r.TrustedHTML,
		StaleTTLSecs:  r.StaleTTLSecs,
		Status:        r.Status,
		StatusMsg:     r.StatusMsg,
		CreatedAt:     r.CreatedAt,
	}
}

// effectiveRemoteURL returns the explicit RemoteURL if set, otherwise constructs
// a default GitHub-style HTTPS clone URL from Host/RepoOwner/RepoName.
func effectiveRemoteURL(r *db.Repo) string {
	if r.RemoteURL != "" {
		return r.RemoteURL
	}
	return "https://" + r.Host + "/" + r.RepoOwner + "/" + r.RepoName + ".git"
}

// handleAPIListRepos returns all repos owned by the authenticated user.
//
// GET /-/api/v1/repos
func (s *Server) handleAPIListRepos(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	ctx := r.Context()

	repos, err := s.dbStore.ListReposByOwner(ctx, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	out := make([]repoResp, 0, len(repos))
	for _, repo := range repos {
		out = append(out, toRepoResp(repo))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleAPICreateRepo creates a new repo record and kicks off a background clone.
//
// POST /-/api/v1/repos
func (s *Server) handleAPICreateRepo(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	ctx := r.Context()

	var req createRepoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Host == "" || req.Owner == "" || req.Repo == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "host, owner, and repo are required"})
		return
	}

	repo := db.Repo{
		OwnerID:       user.ID,
		Host:          req.Host,
		RepoOwner:     req.Owner,
		RepoName:      req.Repo,
		RemoteURL:     req.RemoteURL,
		WebhookSecret: req.WebhookSecret,
		TrustedHTML:   req.TrustedHTML,
		Status:        db.RepoStatusPending,
	}

	if err := s.dbStore.CreateRepo(ctx, &repo); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, toRepoResp(&repo))

	if s.gitStore != nil {
		go func(id int64, host, repoOwner, repoName, remoteURL string) {
			bgCtx := context.Background()
			entry := gitstore.RepoEntry{Host: host, Owner: repoOwner, Name: repoName, RemoteURL: remoteURL}
			if err := s.gitStore.AddRepo(bgCtx, entry); err != nil {
				_ = s.dbStore.UpdateRepoStatus(bgCtx, id, db.RepoStatusError, err.Error())
				return
			}
			_ = s.dbStore.UpdateRepoStatus(bgCtx, id, db.RepoStatusReady, "")
			if s.docSrv != nil {
				s.docSrv.Reload(bgCtx)
			}
		}(repo.ID, repo.Host, repo.RepoOwner, repo.RepoName, effectiveRemoteURL(&repo))
	}
}

// parseRepoID parses the "id" URL param as int64, writing a 400 on error.
// Returns (id, true) on success or (0, false) if parsing failed and a response
// has already been written.
func parseRepoID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid repo id"})
		return 0, false
	}
	return id, true
}

// handleAPIGetRepo returns a single repo owned by (or visible to) the current user.
//
// GET /-/api/v1/repos/{id}
func (s *Server) handleAPIGetRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRepoID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := auth.UserFromContext(ctx)

	repo, err := s.dbStore.GetRepo(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if repo.OwnerID != user.ID && !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, toRepoResp(repo))
}

// updateRepoReq is the JSON body for PATCH /-/api/v1/repos/{id}.
// All fields are pointers so only provided fields are updated.
type updateRepoReq struct {
	WebhookSecret *string `json:"webhook_secret"`
	TrustedHTML   *bool   `json:"trusted_html"`
	StaleTTLSecs  *int64  `json:"stale_ttl_secs"`
	RemoteURL     *string `json:"remote_url"`
}

// handleAPIUpdateRepo applies a partial update to a repo.
//
// PATCH /-/api/v1/repos/{id}
func (s *Server) handleAPIUpdateRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRepoID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := auth.UserFromContext(ctx)

	repo, err := s.dbStore.GetRepo(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if repo.OwnerID != user.ID && !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	var req updateRepoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request body"})
		return
	}

	if req.WebhookSecret != nil {
		repo.WebhookSecret = *req.WebhookSecret
	}
	if req.TrustedHTML != nil {
		repo.TrustedHTML = *req.TrustedHTML
	}
	if req.StaleTTLSecs != nil {
		repo.StaleTTLSecs = *req.StaleTTLSecs
	}
	if req.RemoteURL != nil {
		repo.RemoteURL = *req.RemoteURL
	}

	if err := s.dbStore.UpdateRepo(ctx, repo); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if s.docSrv != nil {
		_ = s.docSrv.Reload(ctx)
	}

	writeJSON(w, http.StatusOK, toRepoResp(repo))
}

// handleAPIDeleteRepo removes a repo record and unregisters it from the git store.
//
// DELETE /-/api/v1/repos/{id}
func (s *Server) handleAPIDeleteRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRepoID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := auth.UserFromContext(ctx)

	repo, err := s.dbStore.GetRepo(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if repo.OwnerID != user.ID && !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if err := s.dbStore.DeleteRepo(ctx, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if s.gitStore != nil {
		s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
	}

	if s.docSrv != nil {
		_ = s.docSrv.Reload(ctx)
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleAPIRepoSync triggers an immediate fetch for a repo.
//
// POST /-/api/v1/repos/{id}/sync
func (s *Server) handleAPIRepoSync(w http.ResponseWriter, r *http.Request) {
	id, ok := parseRepoID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	user := auth.UserFromContext(ctx)

	repo, err := s.dbStore.GetRepo(ctx, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	if repo.OwnerID != user.ID && !user.IsAdmin {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
		return
	}

	if s.gitStore == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "git store not available"})
		return
	}

	gitRepo, err := s.gitStore.Get(repo.Host, repo.RepoOwner, repo.RepoName)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	if err := gitRepo.FetchNow(ctx); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
