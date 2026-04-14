package dashboard

import (
	"net/http"
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
