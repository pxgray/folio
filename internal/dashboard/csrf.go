package dashboard

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

const csrfCookieName = "_csrf"

type csrfKey struct{}

// requireCSRF is a chi middleware that verifies the CSRF token on non-idempotent requests.
// It uses the double-submit cookie pattern: the token is stored in both a cookie and a form field.
func requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		if strings.HasPrefix(r.URL.Path, "/-/api/") {
			next.ServeHTTP(w, r)
			return
		}

		cookie, err := r.Cookie(csrfCookieName)
		if err != nil {
			http.Error(w, "CSRF token missing", http.StatusForbidden)
			return
		}
		cookieToken := cookie.Value

		formToken := r.FormValue("_csrf")
		if formToken == "" {
			http.Error(w, "CSRF token missing from form", http.StatusForbidden)
			return
		}

		if subtle.ConstantTimeCompare([]byte(cookieToken), []byte(formToken)) != 1 {
			http.Error(w, "CSRF token mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// withCSRFToken reads the CSRF cookie and stores the token in the request context.
func withCSRFToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(csrfCookieName)
		if err == nil {
			ctx := context.WithValue(r.Context(), csrfKey{}, cookie.Value)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

// csrfTokenFromContext returns the CSRF token from the request context, or "".
func csrfTokenFromContext(r *http.Request) string {
	v, _ := r.Context().Value(csrfKey{}).(string)
	return v
}
