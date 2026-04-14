package dashboard

import (
	"net/http"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

type adminUsersData struct {
	Title   string
	IsAdmin bool
	Flash   string
	User    *db.User   // current admin (for self-identification in the template)
	Users   []*db.User // all users
}

type adminUserEditData struct {
	Title   string
	IsAdmin bool
	Flash   string
	User    *db.User // current admin (for self-edit check)
	Target  *db.User // user being edited
	Error   string
}

type adminSettingsData struct {
	Title    string
	IsAdmin  bool
	Flash    string
	User     *db.User
	Settings map[string]string
}

// handleAdminUsersPage handles GET /-/dashboard/admin/.
func (s *Server) handleAdminUsersPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	users, err := s.dbStore.ListUsers(ctx)
	if err != nil {
		http.Error(w, "failed to list users", http.StatusInternalServerError)
		return
	}

	data := adminUsersData{
		Title:   "Admin — Users",
		IsAdmin: true,
		Flash:   getFlash(w, r),
		User:    currentUser,
		Users:   users,
	}
	s.renderTemplate(w, "dashboard_admin_users.html", data)
}
