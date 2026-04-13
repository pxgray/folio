package dashboard

import (
	"context"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
)

type repoListData struct {
	Title   string
	Flash   string
	IsAdmin bool
	User    *db.User
	Repos   []*db.Repo
}

type repoFormData struct {
	Title      string
	Flash      string
	IsAdmin    bool
	User       *db.User
	Repo       *db.Repo // nil for new, populated for edit
	Error      string
	WebhookURL string // only populated on edit page
}

func (s *Server) handleRepoList(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	repos, err := s.dbStore.ListReposByOwner(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to list repos: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "dashboard_repos.html", repoListData{
		Title:   "Repos",
		Flash:   getFlash(w, r),
		IsAdmin: user.IsAdmin,
		User:    user,
		Repos:   repos,
	})
}

func (s *Server) handleRepoNew(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
		Title:   "Add Repo",
		IsAdmin: user.IsAdmin,
		User:    user,
	})
}

func (s *Server) handleRepoCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	host := strings.TrimSpace(r.FormValue("host"))
	owner := strings.TrimSpace(r.FormValue("owner"))
	repoName := strings.TrimSpace(r.FormValue("repo_name"))

	if host == "" || owner == "" || repoName == "" {
		w.WriteHeader(http.StatusUnprocessableEntity)
		s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
			Title:   "Add Repo",
			IsAdmin: user.IsAdmin,
			User:    user,
			Error:   "Host, Owner, and Repo Name are required.",
		})
		return
	}

	repo := &db.Repo{
		OwnerID:       user.ID,
		Host:          host,
		RepoOwner:     owner,
		RepoName:      repoName,
		RemoteURL:     strings.TrimSpace(r.FormValue("remote_url")),
		WebhookSecret: strings.TrimSpace(r.FormValue("webhook_secret")),
		TrustedHTML:   r.FormValue("trusted_html") == "on",
		Status:        db.RepoStatusPending,
	}
	if err := s.dbStore.CreateRepo(r.Context(), repo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
			Title:   "Add Repo",
			IsAdmin: user.IsAdmin,
			User:    user,
			Error:   "Could not create repo: " + err.Error(),
		})
		return
	}

	// Trigger background clone if gitStore is available.
	if s.gitStore != nil {
		go func() {
			ctx := context.Background()
			staleTTL := time.Duration(repo.StaleTTLSecs) * time.Second
			err := s.gitStore.AddRepo(ctx, gitstore.RepoEntry{
				Host:      repo.Host,
				Owner:     repo.RepoOwner,
				Name:      repo.RepoName,
				RemoteURL: repo.RemoteURL,
				StaleTTL:  staleTTL,
			})
			if err != nil {
				log.Printf("background clone failed for %s/%s/%s: %v", host, owner, repoName, err)
				_ = s.dbStore.UpdateRepoStatus(context.Background(), repo.ID, db.RepoStatusError, err.Error())
			}
		}()
	}

	setFlash(w, "Repo added — cloning in background.")
	http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
}

func (s *Server) renderTemplate(w http.ResponseWriter, name string, data any) {
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"has": func(slice []string, item string) bool {
			for _, v := range slice {
				if v == item {
					return true
				}
			}
			return false
		},
	}).ParseFS(s.tmplFS,
		"templates/dashboard_base.html",
		"templates/"+name,
	)
	if err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "dashboard_base.html", data); err != nil {
		log.Printf("template execute error: %v", err)
	}
}
