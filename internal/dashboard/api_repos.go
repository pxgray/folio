package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

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
