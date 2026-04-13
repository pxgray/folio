package dashboard

import "net/http"

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
	http.NotFound(w, r)
}

func (s *Server) renderSetup(w http.ResponseWriter, data setupPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.setupTmpl.Execute(w, data); err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
	}
}
