package dashboard

import (
	"html/template"
	"log"
	"net/http"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

type repoListData struct {
	Title   string
	Flash   string
	IsAdmin bool
	User    *db.User
	Repos   []*db.Repo
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
