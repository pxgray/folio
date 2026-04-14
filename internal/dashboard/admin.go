package dashboard

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
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

// handleAdminUserEditPage handles GET /-/dashboard/admin/users/{id}.
func (s *Server) handleAdminUserEditPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	target, err := s.dbStore.GetUserByID(ctx, targetID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}
	data := adminUserEditData{
		Title:   "Admin — Edit User",
		IsAdmin: true,
		Flash:   getFlash(w, r),
		User:    currentUser,
		Target:  target,
	}
	s.renderTemplate(w, "dashboard_admin_user_edit.html", data)
}

// handleAdminUserEditPost handles POST /-/dashboard/admin/users/{id}.
func (s *Server) handleAdminUserEditPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	target, err := s.dbStore.GetUserByID(ctx, targetID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	target.Name = r.FormValue("name")
	target.Email = r.FormValue("email")

	// is_admin: only update if not editing own record
	if currentUser.ID != targetID {
		target.IsAdmin = r.FormValue("is_admin") == "true"
	}

	if pw := r.FormValue("password"); pw != "" {
		hashed, err := auth.HashPassword(pw)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		target.Password = hashed
	}

	if err := s.dbStore.UpdateUser(ctx, target); err != nil {
		data := adminUserEditData{
			Title:   "Admin — Edit User",
			IsAdmin: true,
			User:    currentUser,
			Target:  target,
			Error:   "Failed to save: " + err.Error(),
		}
		s.renderTemplate(w, "dashboard_admin_user_edit.html", data)
		return
	}

	setFlash(w, "User updated.")
	http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
}

// handleAdminUserDeletePost handles POST /-/dashboard/admin/users/{id}/delete.
func (s *Server) handleAdminUserDeletePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if targetID == currentUser.ID {
		setFlash(w, "Cannot delete your own account.")
		http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
		return
	}

	repos, _ := s.dbStore.ListReposByOwner(ctx, targetID)
	if s.gitStore != nil {
		for _, repo := range repos {
			s.gitStore.RemoveRepo(repo.Host, repo.RepoOwner, repo.RepoName)
		}
	}
	s.dbStore.DeleteUser(ctx, targetID)
	if s.docSrv != nil {
		_ = s.docSrv.Reload(ctx)
	}

	setFlash(w, "User deleted.")
	http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
}

// handleAdminToggleAdmin handles POST /-/dashboard/admin/users/{id}/toggle-admin.
func (s *Server) handleAdminToggleAdmin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	currentUser := auth.UserFromContext(ctx)

	idStr := chi.URLParam(r, "id")
	targetID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid user id", http.StatusBadRequest)
		return
	}
	if targetID == currentUser.ID {
		setFlash(w, "Cannot change your own admin status.")
		http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
		return
	}

	target, err := s.dbStore.GetUserByID(ctx, targetID)
	if err != nil {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	// Guard: demoting last admin.
	if target.IsAdmin {
		users, _ := s.dbStore.ListUsers(ctx)
		adminCount := 0
		for _, u := range users {
			if u.IsAdmin {
				adminCount++
			}
		}
		if adminCount <= 1 {
			setFlash(w, "Cannot demote the last admin.")
			http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
			return
		}
	}

	target.IsAdmin = !target.IsAdmin
	s.dbStore.UpdateUser(ctx, target)

	setFlash(w, "Admin status updated.")
	http.Redirect(w, r, "/-/dashboard/admin/", http.StatusSeeOther)
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
