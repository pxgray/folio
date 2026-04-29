package dashboard

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
	"github.com/pxgray/folio/internal/gitstore"
)

type repoListData struct {
	Title     string
	Flash     string
	IsAdmin   bool
	User      *db.User
	Repos     []*db.Repo
	CSRFToken string
}

type repoFormData struct {
	Title      string
	Flash      string
	IsAdmin    bool
	User       *db.User
	Repo       *db.Repo // nil for new, populated for edit
	Error      string
	WebhookURL string // only populated on edit page
	CSRFToken  string
	RepoType   string // "remote" or "local" — defaults to "remote"
}

func (s *Server) handleRepoList(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	repos, err := s.dbStore.ListReposByOwner(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "failed to list repos: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "dashboard_repos.html", repoListData{
		Title:     "Repos",
		Flash:     getFlash(w, r),
		IsAdmin:   user.IsAdmin,
		User:      user,
		Repos:     repos,
		CSRFToken: csrfTokenFromContext(r),
	})
}

func (s *Server) handleRepoNew(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
		Title:     "Add Repo",
		IsAdmin:   user.IsAdmin,
		User:      user,
		RepoType:  "remote",
		CSRFToken: csrfTokenFromContext(r),
	})
}

func (s *Server) handleRepoCreate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	repoType := strings.TrimSpace(r.FormValue("repo_type"))
	if repoType == "" {
		repoType = "remote"
	}

	var repo *db.Repo

	if repoType == "local" {
		label := strings.TrimSpace(r.FormValue("label"))
		path := strings.TrimSpace(r.FormValue("path"))

		if label == "" || path == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
				Title:     "Add Repo",
				IsAdmin:   user.IsAdmin,
				User:      user,
				Error:     "Label and Path are required.",
				RepoType:  "local",
				CSRFToken: csrfTokenFromContext(r),
			})
			return
		}

		repo = &db.Repo{
			OwnerID:     user.ID,
			RepoType:    "local",
			Label:       label,
			Path:        path,
			TrustedHTML: r.FormValue("trusted_html") == "on",
			Status:      db.RepoStatusReady,
		}
	} else {
		host := strings.TrimSpace(r.FormValue("host"))
		owner := strings.TrimSpace(r.FormValue("owner"))
		repoName := strings.TrimSpace(r.FormValue("repo_name"))

		if host == "" || owner == "" || repoName == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
				Title:     "Add Repo",
				IsAdmin:   user.IsAdmin,
				User:      user,
				Error:     "Host, Owner, and Repo Name are required.",
				RepoType:  "remote",
				CSRFToken: csrfTokenFromContext(r),
			})
			return
		}

		repo = &db.Repo{
			OwnerID:       user.ID,
			Host:          host,
			RepoOwner:     owner,
			RepoName:      repoName,
			RemoteURL:     strings.TrimSpace(r.FormValue("remote_url")),
			WebhookSecret: strings.TrimSpace(r.FormValue("webhook_secret")),
			TrustedHTML:   r.FormValue("trusted_html") == "on",
			Status:        db.RepoStatusPending,
		}
	}

	if err := s.dbStore.CreateRepo(r.Context(), repo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
			Title:     "Add Repo",
			IsAdmin:   user.IsAdmin,
			User:      user,
			Error:     "Could not create repo: " + err.Error(),
			RepoType:  repoType,
			CSRFToken: csrfTokenFromContext(r),
		})
		return
	}

	// Trigger background clone for remote repos.
	if repoType == "remote" && s.gitStore != nil {
		repoID := repo.ID
		repoHost := repo.Host
		repoOwner := repo.RepoOwner
		repoNameLocal := repo.RepoName
		repoRemoteURL := repo.RemoteURL
		repoStaleTTL := repo.StaleTTLSecs
		go func() {
			ctx := context.Background()
			staleTTL := time.Duration(repoStaleTTL) * time.Second
			err := s.gitStore.AddRepo(ctx, gitstore.RepoEntry{
				Host:      repoHost,
				Owner:     repoOwner,
				Name:      repoNameLocal,
				RemoteURL: repoRemoteURL,
				StaleTTL:  staleTTL,
			})
			if err != nil {
				log.Printf("background clone failed for %s/%s/%s: %v", repoHost, repoOwner, repoNameLocal, err)
				_ = s.dbStore.UpdateRepoStatus(context.Background(), repoID, db.RepoStatusError, err.Error())
			} else {
				_ = s.dbStore.UpdateRepoStatus(context.Background(), repoID, db.RepoStatusReady, "")
			}
		}()
	}

	setFlash(w, "Repo added.")
	http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
}

func parseID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id: %w", err)
	}
	return id, nil
}

func (s *Server) handleRepoEdit(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	repo, err := s.dbStore.GetRepo(r.Context(), id)
	if err != nil || id == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if repo.OwnerID != user.ID && !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	repoType := repo.RepoType
	if repoType == "" {
		repoType = "remote"
	}
	webhookURL := fmt.Sprintf("/%s/%s/%s/-/webhook", repo.Host, repo.RepoOwner, repo.RepoName)
	s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
		Title:      "Edit Repo",
		Flash:      getFlash(w, r),
		IsAdmin:    user.IsAdmin,
		User:       user,
		Repo:       repo,
		RepoType:   repoType,
		WebhookURL: webhookURL,
		CSRFToken:  csrfTokenFromContext(r),
	})
}

func (s *Server) handleRepoUpdate(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	repo, err := s.dbStore.GetRepo(r.Context(), id)
	if err != nil || id == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if repo.OwnerID != user.ID && !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	repoType := repo.RepoType
	if repoType == "" {
		repoType = "remote"
	}

	if repoType == "local" {
		path := strings.TrimSpace(r.FormValue("path"))
		if path == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
				Title:     "Edit Repo",
				IsAdmin:   user.IsAdmin,
				User:      user,
				Repo:      repo,
				RepoType:  "local",
				Error:     "Path is required.",
				CSRFToken: csrfTokenFromContext(r),
			})
			return
		}
		repo.Path = path
		repo.TrustedHTML = r.FormValue("trusted_html") == "on"
	} else {
		host := strings.TrimSpace(r.FormValue("host"))
		owner := strings.TrimSpace(r.FormValue("owner"))
		repoName := strings.TrimSpace(r.FormValue("repo_name"))
		if host == "" || owner == "" || repoName == "" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
				Title:     "Edit Repo",
				IsAdmin:   user.IsAdmin,
				User:      user,
				Repo:      repo,
				RepoType:  "remote",
				Error:     "Host, Owner, and Repo Name are required.",
				CSRFToken: csrfTokenFromContext(r),
			})
			return
		}
		repo.Host = host
		repo.RepoOwner = owner
		repo.RepoName = repoName
		repo.RemoteURL = strings.TrimSpace(r.FormValue("remote_url"))
		repo.WebhookSecret = strings.TrimSpace(r.FormValue("webhook_secret"))
		repo.TrustedHTML = r.FormValue("trusted_html") == "on"
	}

	if err := s.dbStore.UpdateRepo(r.Context(), repo); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		s.renderTemplate(w, "dashboard_repo_form.html", repoFormData{
			Title:     "Edit Repo",
			IsAdmin:   user.IsAdmin,
			User:      user,
			Repo:      repo,
			RepoType:  repoType,
			Error:     "Save failed: " + err.Error(),
			CSRFToken: csrfTokenFromContext(r),
		})
		return
	}
	setFlash(w, "Repo updated.")
	http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
}

func (s *Server) handleRepoDelete(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	repo, err := s.dbStore.GetRepo(r.Context(), id)
	if err != nil || id == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if repo.OwnerID != user.ID && !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if repo.RepoType == "local" && s.gitStore != nil {
		s.gitStore.RemoveLocal(repo.Label)
	} else if s.gitStore != nil {
		s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
	}

	if err := s.dbStore.DeleteRepo(r.Context(), id); err != nil {
		http.Error(w, "delete failed", http.StatusInternalServerError)
		return
	}
	setFlash(w, "Repo deleted.")
	http.Redirect(w, r, "/-/dashboard/", http.StatusSeeOther)
}

func (s *Server) handleRepoSync(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	id, err := parseID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	repo, err := s.dbStore.GetRepo(r.Context(), id)
	if err != nil || id == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if repo.OwnerID != user.ID && !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if repo.RepoType == "local" {
		setFlash(w, "Local repos cannot be synced.")
		http.Redirect(w, r, fmt.Sprintf("/-/dashboard/repos/%d", id), http.StatusSeeOther)
		return
	}

	if s.gitStore != nil {
		host := repo.Host
		owner := repo.RepoOwner
		name := repo.RepoName
		go func() {
			gitRepo, err := s.gitStore.Get(host, owner, name)
			if err != nil {
				log.Printf("sync: repo not registered %s/%s/%s: %v", host, owner, name, err)
				return
			}
			if err := gitRepo.FetchNow(context.Background()); err != nil {
				log.Printf("sync failed for %s/%s/%s: %v", host, owner, name, err)
			}
		}()
	}
	setFlash(w, "Sync triggered.")
	http.Redirect(w, r, fmt.Sprintf("/-/dashboard/repos/%d", id), http.StatusSeeOther)
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
