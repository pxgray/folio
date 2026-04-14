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
	if body.Password != nil && *body.Password != "" {
		hashed, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to hash password"})
			return
		}
		target.Password = hashed
	}

	if err := s.dbStore.UpdateUser(ctx, target); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
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
