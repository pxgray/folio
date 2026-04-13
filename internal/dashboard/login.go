package dashboard

import (
	"net/http"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Sign in", "Error": r.URL.Query().Get("error")}
	s.loginTmpl.Execute(w, data)
}

// handleFormLogout clears the session cookie, deletes the session from the database,
// and redirects to the login page.
//
// POST /-/auth/logout
func (s *Server) handleFormLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read the session cookie. If present, delete from database.
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		s.dbStore.DeleteSession(ctx, cookie.Value)
	}

	// Clear the session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	// Redirect to login page.
	http.Redirect(w, r, "/-/auth/login", http.StatusSeeOther)
}
