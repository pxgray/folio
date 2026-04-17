package dashboard

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Sign in", "Error": r.URL.Query().Get("error")}
	s.loginTmpl.Execute(w, data)
}

// handleFormLogin handles POST /-/auth/login from the HTML login form.
// It authenticates with email/password, sets a session cookie, and redirects to the dashboard.
func (s *Server) handleFormLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	ctx := r.Context()
	user, err := s.dbStore.GetUserByEmail(ctx, email)
	if err != nil || !auth.CheckPassword(user.Password, password) {
		http.Redirect(w, r, "/-/auth/login?error=invalid_credentials", http.StatusSeeOther)
		return
	}

	s.createSessionAndRedirect(w, r, user.ID)
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

// handleGitHubOAuth initiates the GitHub OAuth flow.
//
// GET /-/auth/github
func (s *Server) handleGitHubOAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	clientID, err := s.dbStore.GetSetting(ctx, "oauth_github_client_id")
	if err != nil || clientID == "" {
		http.Redirect(w, r, "/-/auth/login?error=github_not_configured", http.StatusFound)
		return
	}
	clientSecret, err := s.dbStore.GetSetting(ctx, "oauth_github_client_secret")
	if err != nil || clientSecret == "" {
		http.Redirect(w, r, "/-/auth/login?error=github_not_configured", http.StatusFound)
		return
	}

	state, err := generateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	cfg := auth.GitHubOAuthConfig(auth.OAuthConfig{
		GitHubClientID:     clientID,
		GitHubClientSecret: clientSecret,
		BaseURL:            baseURL(r),
	})
	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
}

// handleGitHubCallback handles the GitHub OAuth callback.
//
// GET /-/auth/github/callback
func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate state cookie.
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie.
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	clientID, err := s.dbStore.GetSetting(ctx, "oauth_github_client_id")
	if err != nil || clientID == "" {
		http.Redirect(w, r, "/-/auth/login?error=github_not_configured", http.StatusFound)
		return
	}
	clientSecret, err := s.dbStore.GetSetting(ctx, "oauth_github_client_secret")
	if err != nil || clientSecret == "" {
		http.Redirect(w, r, "/-/auth/login?error=github_not_configured", http.StatusFound)
		return
	}

	oauthCfg := auth.OAuthConfig{
		GitHubClientID:     clientID,
		GitHubClientSecret: clientSecret,
		BaseURL:            baseURL(r),
	}
	cfg := auth.GitHubOAuthConfig(oauthCfg)

	token, err := cfg.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/-/auth/login?error=github_exchange_failed", http.StatusFound)
		return
	}

	profile, err := auth.FetchGitHubProfile(ctx, token, oauthCfg)
	if err != nil {
		http.Redirect(w, r, "/-/auth/login?error=github_profile_failed", http.StatusFound)
		return
	}

	user, err := s.dbStore.GetUserByOAuth(ctx, "github", profile.ProviderID)
	if err != nil {
		// User not found — create new user and OAuth account.
		newUser := &db.User{
			Email: profile.Email,
			Name:  profile.Name,
		}
		if createErr := s.dbStore.CreateUser(ctx, newUser); createErr != nil {
			http.Redirect(w, r, "/-/auth/login?error=github_create_user_failed", http.StatusFound)
			return
		}
		oauthAccount := &db.OAuthAccount{
			UserID:     newUser.ID,
			Provider:   "github",
			ProviderID: profile.ProviderID,
		}
		if linkErr := s.dbStore.CreateOAuthAccount(ctx, oauthAccount); linkErr != nil {
			http.Redirect(w, r, "/-/auth/login?error=github_link_failed", http.StatusFound)
			return
		}
		user = newUser
	}

	s.createSessionAndRedirect(w, r, user.ID)
}

// handleGoogleOAuth initiates the Google OAuth flow.
//
// GET /-/auth/google
func (s *Server) handleGoogleOAuth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	clientID, err := s.dbStore.GetSetting(ctx, "oauth_google_client_id")
	if err != nil || clientID == "" {
		http.Redirect(w, r, "/-/auth/login?error=google_not_configured", http.StatusFound)
		return
	}
	clientSecret, err := s.dbStore.GetSetting(ctx, "oauth_google_client_secret")
	if err != nil || clientSecret == "" {
		http.Redirect(w, r, "/-/auth/login?error=google_not_configured", http.StatusFound)
		return
	}

	state, err := generateState()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})

	cfg := auth.GoogleOAuthConfig(auth.OAuthConfig{
		GoogleClientID:     clientID,
		GoogleClientSecret: clientSecret,
		BaseURL:            baseURL(r),
	})
	http.Redirect(w, r, cfg.AuthCodeURL(state), http.StatusFound)
}

// handleGoogleCallback handles the Google OAuth callback.
//
// GET /-/auth/google/callback
func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Validate state cookie.
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}

	// Clear state cookie.
	http.SetCookie(w, &http.Cookie{
		Name:   "oauth_state",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

	clientID, err := s.dbStore.GetSetting(ctx, "oauth_google_client_id")
	if err != nil || clientID == "" {
		http.Redirect(w, r, "/-/auth/login?error=google_not_configured", http.StatusFound)
		return
	}
	clientSecret, err := s.dbStore.GetSetting(ctx, "oauth_google_client_secret")
	if err != nil || clientSecret == "" {
		http.Redirect(w, r, "/-/auth/login?error=google_not_configured", http.StatusFound)
		return
	}

	cfg := auth.GoogleOAuthConfig(auth.OAuthConfig{
		GoogleClientID:     clientID,
		GoogleClientSecret: clientSecret,
		BaseURL:            baseURL(r),
	})

	token, err := cfg.Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Redirect(w, r, "/-/auth/login?error=google_exchange_failed", http.StatusFound)
		return
	}

	// Extract profile from the OpenID Connect id_token JWT.
	profile, err := parseGoogleIDToken(token)
	if err != nil {
		http.Redirect(w, r, "/-/auth/login?error=google_profile_failed", http.StatusFound)
		return
	}

	user, err := s.dbStore.GetUserByOAuth(ctx, "google", profile.ProviderID)
	if err != nil {
		// User not found — create new user and OAuth account.
		newUser := &db.User{
			Email: profile.Email,
			Name:  profile.Name,
		}
		if createErr := s.dbStore.CreateUser(ctx, newUser); createErr != nil {
			http.Redirect(w, r, "/-/auth/login?error=google_create_user_failed", http.StatusFound)
			return
		}
		oauthAccount := &db.OAuthAccount{
			UserID:     newUser.ID,
			Provider:   "google",
			ProviderID: profile.ProviderID,
		}
		if linkErr := s.dbStore.CreateOAuthAccount(ctx, oauthAccount); linkErr != nil {
			http.Redirect(w, r, "/-/auth/login?error=google_link_failed", http.StatusFound)
			return
		}
		user = newUser
	}

	s.createSessionAndRedirect(w, r, user.ID)
}

// createSessionAndRedirect creates a session for userID, sets the session cookie,
// and redirects the browser to the dashboard.
func (s *Server) createSessionAndRedirect(w http.ResponseWriter, r *http.Request, userID int64) {
	ctx := r.Context()

	session, err := s.authn.NewSession(ctx, userID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
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

	http.Redirect(w, r, "/-/dashboard/", http.StatusFound)
}

// parseGoogleIDToken extracts the user profile from the OpenID Connect id_token
// embedded in the token response. The id_token is a JWT; we base64-decode the
// payload (middle section) without verifying the signature, which is acceptable
// for this server-side flow where the token was received directly from Google.
func parseGoogleIDToken(token interface{ Extra(string) interface{} }) (*auth.OAuthProfile, error) {
	raw, _ := token.Extra("id_token").(string)
	if raw == "" {
		return nil, errors.New("parseGoogleIDToken: id_token missing from token response")
	}

	// JWT is three base64url-encoded sections separated by ".".
	// We only need the payload (index 1).
	parts := splitJWT(raw)
	if len(parts) != 3 {
		return nil, errors.New("parseGoogleIDToken: malformed JWT")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("parseGoogleIDToken: base64 decode: " + err.Error())
	}

	var claims struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, errors.New("parseGoogleIDToken: json unmarshal: " + err.Error())
	}
	if claims.Sub == "" {
		return nil, errors.New("parseGoogleIDToken: sub claim missing")
	}

	return &auth.OAuthProfile{
		ProviderID: claims.Sub,
		Email:      claims.Email,
		Name:       claims.Name,
	}, nil
}

// splitJWT splits a JWT string on "." without importing strings to keep imports tidy.
func splitJWT(s string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// generateState returns a 16-byte cryptographically random state token as hex.
func generateState() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// baseURL derives the scheme+host from the incoming request.
func baseURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil && r.Header.Get("X-Forwarded-Proto") == "" {
		scheme = "http"
	}
	return scheme + "://" + r.Host
}
