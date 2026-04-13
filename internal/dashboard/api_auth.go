package dashboard

import (
	"encoding/json"
	"net/http"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

// userResp is the JSON shape returned for authenticated user endpoints.
type userResp struct {
	ID      int64  `json:"id"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	IsAdmin bool   `json:"is_admin"`
}

// userResponse converts a *db.User to its JSON-safe representation.
func userResponse(u *db.User) userResp {
	return userResp{
		ID:      u.ID,
		Email:   u.Email,
		Name:    u.Name,
		IsAdmin: u.IsAdmin,
	}
}

// writeJSON writes v as JSON with the given HTTP status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// handleAPILogin authenticates a user with email/password and sets a session cookie.
//
// POST /-/api/v1/auth/login
func (s *Server) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Email    string
		Password string
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid request body"})
		return
	}

	ctx := r.Context()

	user, err := s.dbStore.GetUserByEmail(ctx, creds.Email)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid credentials"})
		return
	}

	if !auth.CheckPassword(user.Password, creds.Password) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "invalid credentials"})
		return
	}

	session, err := s.authn.NewSession(ctx, user.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, userResponse(user))
}

// handleAPIMe returns the currently authenticated user.
//
// GET /-/api/v1/auth/me — protected by auth.RequireAuth middleware at route registration.
func (s *Server) handleAPIMe(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	writeJSON(w, http.StatusOK, userResponse(user))
}

// handleAPILogout clears the session cookie and deletes the session from the database.
//
// POST /-/api/v1/auth/logout
func (s *Server) handleAPILogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Read the session cookie. If absent, still return 200.
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		// Delete session from database.
		s.dbStore.DeleteSession(ctx, cookie.Value)
	}

	// Clear the session cookie.
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
