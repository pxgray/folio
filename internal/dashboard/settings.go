package dashboard

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

type settingsData struct {
	Title       string
	Flash       string
	IsAdmin     bool
	User        *db.User
	LinkedOAuth []string // provider names, e.g. ["github"]
	Error       string
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	linked := s.linkedProviders(r, user.ID)
	s.renderTemplate(w, "dashboard_settings.html", settingsData{
		Title:       "Settings",
		Flash:       getFlash(w, r),
		IsAdmin:     user.IsAdmin,
		User:        user,
		LinkedOAuth: linked,
	})
}

func (s *Server) handleSettingsPost(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	// Update display name if provided.
	if name := strings.TrimSpace(r.FormValue("display_name")); name != "" {
		user.Name = name
		if err := s.dbStore.UpdateUser(r.Context(), user); err != nil {
			s.renderSettingsError(w, r, user, "Failed to update name: "+err.Error())
			return
		}
	}

	// Change password if current_password provided.
	if current := r.FormValue("current_password"); current != "" {
		if !auth.CheckPassword(user.Password, current) {
			s.renderSettingsError(w, r, user, "Current password is incorrect.")
			return
		}
		newPw := r.FormValue("new_password")
		if len(newPw) < 8 {
			s.renderSettingsError(w, r, user, "New password must be at least 8 characters.")
			return
		}
		hash, err := auth.HashPassword(newPw)
		if err != nil {
			s.renderSettingsError(w, r, user, "Failed to hash password.")
			return
		}
		user.Password = hash
		if err := s.dbStore.UpdateUser(r.Context(), user); err != nil {
			s.renderSettingsError(w, r, user, "Failed to save password: "+err.Error())
			return
		}
	}

	setFlash(w, "Settings saved.")
	http.Redirect(w, r, "/-/dashboard/settings", http.StatusSeeOther)
}

func (s *Server) handleOAuthUnlink(w http.ResponseWriter, r *http.Request) {
	user := auth.UserFromContext(r.Context())
	provider := chi.URLParam(r, "provider")
	if err := s.dbStore.DeleteOAuthAccount(r.Context(), user.ID, provider); err != nil {
		setFlash(w, "Failed to unlink: "+err.Error())
	} else {
		setFlash(w, "Unlinked "+provider+".")
	}
	http.Redirect(w, r, "/-/dashboard/settings", http.StatusSeeOther)
}

func (s *Server) renderSettingsError(w http.ResponseWriter, r *http.Request, user *db.User, msg string) {
	linked := s.linkedProviders(r, user.ID)
	w.WriteHeader(http.StatusUnprocessableEntity)
	s.renderTemplate(w, "dashboard_settings.html", settingsData{
		Title:       "Settings",
		IsAdmin:     user.IsAdmin,
		User:        user,
		LinkedOAuth: linked,
		Error:       msg,
	})
}

// linkedProviders returns the list of OAuth provider names linked to userID.
func (s *Server) linkedProviders(r *http.Request, userID int64) []string {
	accounts, _ := s.dbStore.ListOAuthAccounts(r.Context(), userID)
	providers := make([]string, 0, len(accounts))
	for _, a := range accounts {
		providers = append(providers, a.Provider)
	}
	return providers
}
