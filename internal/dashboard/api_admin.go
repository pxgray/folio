package dashboard

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/pxgray/folio/internal/auth"
)

// handleAdminListUsers handles GET /-/api/v1/admin/users.
// Returns a JSON array of all users. Requires admin role.
func (s *Server) handleAdminListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.dbStore.ListUsers(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list users"})
		return
	}
	type userRow struct {
		ID        int64  `json:"id"`
		Email     string `json:"email"`
		Name      string `json:"name"`
		IsAdmin   bool   `json:"is_admin"`
		CreatedAt string `json:"created_at"`
	}
	rows := make([]userRow, len(users))
	for i, u := range users {
		rows[i] = userRow{
			ID:        u.ID,
			Email:     u.Email,
			Name:      u.Name,
			IsAdmin:   u.IsAdmin,
			CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	writeJSON(w, http.StatusOK, rows)
}

// handleAdminUpdateUser handles PATCH /-/api/v1/admin/users/{id}.
func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	var body struct {
		Name     *string `json:"name"`
		IsAdmin  *bool   `json:"is_admin"`
		Password *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	target, err := s.dbStore.GetUserByID(ctx, targetID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	// Guard: cannot change your own admin status.
	if body.IsAdmin != nil && currentUser.ID == targetID {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cannot modify your own admin status"})
		return
	}

	// Guard: cannot demote the last admin.
	if body.IsAdmin != nil && !*body.IsAdmin && target.IsAdmin {
		users, err := s.dbStore.ListUsers(ctx)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			return
		}
		adminCount := 0
		for _, u := range users {
			if u.IsAdmin {
				adminCount++
			}
		}
		if adminCount <= 1 {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cannot demote the last admin"})
			return
		}
	}

	if body.Name != nil {
		target.Name = *body.Name
	}
	if body.IsAdmin != nil {
		target.IsAdmin = *body.IsAdmin
	}

	var pwHash *string
	if body.Password != nil && *body.Password != "" {
		hashed, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
			return
		}
		pwHash = &hashed
	}

	if err := s.dbStore.UpdateUser(ctx, target, pwHash); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// knownSettings is the ordered list of all server_settings keys served by the API.
var knownSettings = []string{
	"addr",
	"cache_dir",
	"stale_ttl",
	"base_url",
	"oauth_github_client_id",
	"oauth_github_client_secret",
	"oauth_google_client_id",
	"oauth_google_client_secret",
}

// restartRequiredSettings are settings whose change requires a server restart.
var restartRequiredSettings = map[string]bool{
	"addr":      true,
	"cache_dir": true,
}

// handleAdminGetSettings handles GET /-/api/v1/admin/settings.
func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	result := make(map[string]string, len(knownSettings))
	for _, key := range knownSettings {
		val, err := s.dbStore.GetSetting(ctx, key)
		if err != nil {
			val = "" // missing keys default to empty string
		}
		result[key] = val
	}
	writeJSON(w, http.StatusOK, result)
}

// handleAdminPatchSettings handles PATCH /-/api/v1/admin/settings.
func (s *Server) handleAdminPatchSettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body map[string]string
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	restartNeeded := false
	for key, val := range body {
		if err := s.dbStore.UpsertSetting(ctx, key, val); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save setting: " + key})
			return
		}
		if restartRequiredSettings[key] {
			restartNeeded = true
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"restart_required": restartNeeded,
	})
}

// handleAdminDeleteUser handles DELETE /-/api/v1/admin/users/{id}.
func (s *Server) handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid user id"})
		return
	}

	if targetID == currentUser.ID {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "cannot delete your own account"})
		return
	}

	// Remove all repos owned by this user from the live gitStore before deleting from DB.
	repos, err := s.dbStore.ListReposByOwner(ctx, targetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list repos"})
		return
	}
	if s.gitStore != nil {
		for _, repo := range repos {
			s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
		}
	}

	if err := s.dbStore.DeleteUser(ctx, targetID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete user"})
		return
	}

	if s.docSrv != nil {
		_ = s.docSrv.Reload(ctx)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
