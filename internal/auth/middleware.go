package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/pxgray/folio/internal/db"
)

type contextKey struct{}

// UserFromContext returns the authenticated *db.User from ctx, or nil.
func UserFromContext(ctx context.Context) *db.User {
	u, _ := ctx.Value(contextKey{}).(*db.User)
	return u
}

// RequireAuth is HTTP middleware that validates the "session" cookie.
// On success it injects *db.User into the request context via UserFromContext.
// On failure: /-/api/* paths receive 401; all others are redirected to /-/auth/login.
func RequireAuth(a *Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie("session")
			if err != nil {
				denyAuth(w, r)
				return
			}
			user, err := a.ValidateSession(r.Context(), cookie.Value)
			if err != nil {
				denyAuth(w, r)
				return
			}
			ctx := context.WithValue(r.Context(), contextKey{}, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin wraps RequireAuth and additionally requires user.IsAdmin.
// Returns 403 if the authenticated user is not an admin.
func RequireAdmin(a *Auth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := UserFromContext(r.Context())
			if user == nil || !user.IsAdmin {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		}))
	}
}

// denyAuth sends 401 for API paths and redirects to login for all others.
func denyAuth(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/-/api/") {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, "/-/auth/login", http.StatusFound)
}
