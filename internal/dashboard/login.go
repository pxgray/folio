package dashboard

import (
	"net/http"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Sign in", "Error": r.URL.Query().Get("error")}
	s.loginTmpl.Execute(w, data)
}
