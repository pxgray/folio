package dashboard

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pxgray/folio/internal/auth"
	"github.com/pxgray/folio/internal/db"
	"golang.org/x/oauth2"
)

func (s *Server) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	if !s.setupComplete {
		complete, err := s.dbStore.IsSetupComplete(r.Context())
		if err == nil && !complete {
			http.Redirect(w, r, "/-/setup", http.StatusSeeOther)
			return
		}
		if err == nil && complete {
			s.setupComplete = true
		}
	}
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
	profile, err := parseGoogleIDToken(ctx, token, clientID)
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

	http.SetCookie(w, &http.Cookie{
		Name:     "_csrf",
		Value:    session.CSRFToken,
		Path:     "/",
		HttpOnly: false,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/-/dashboard/", http.StatusFound)
}

// jwksCache caches Google's public keys for JWT verification.
var jwksCache struct {
	sync.RWMutex
	keys    []*rsa.PublicKey
	kids    []string
	fetched time.Time
	expires time.Duration
	once    sync.Once
}

const jwksExpiry = 6 * time.Hour

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func initJWKS() {
	jwksCache.expires = jwksExpiry
}

func fetchJWKS(ctx context.Context) ([]*rsa.PublicKey, []string, error) {
	jwksCache.once.Do(initJWKS)

	jwksCache.RLock()
	if time.Since(jwksCache.fetched) < jwksCache.expires && len(jwksCache.keys) > 0 {
		keys := make([]*rsa.PublicKey, len(jwksCache.keys))
		copy(keys, jwksCache.keys)
		kids := make([]string, len(jwksCache.kids))
		copy(kids, jwksCache.kids)
		jwksCache.RUnlock()
		return keys, kids, nil
	}
	jwksCache.RUnlock()

	jwksCache.Lock()
	defer jwksCache.Unlock()

	if time.Since(jwksCache.fetched) < jwksCache.expires && len(jwksCache.keys) > 0 {
		keys := make([]*rsa.PublicKey, len(jwksCache.keys))
		copy(keys, jwksCache.keys)
		kids := make([]string, len(jwksCache.kids))
		copy(kids, jwksCache.kids)
		return keys, kids, nil
	}

	resp, err := http.Get("https://www.googleapis.com/oauth2/v3/certs")
	if err != nil {
		return nil, nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetch JWKS: HTTP %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, nil, fmt.Errorf("decode JWKS: %w", err)
	}

	keys := make([]*rsa.PublicKey, 0, len(jwks.Keys))
	kids := make([]string, 0, len(jwks.Keys))
	for _, k := range jwks.Keys {
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		e := new(big.Int).SetBytes(eBytes)
		n := new(big.Int).SetBytes(nBytes)
		rsaKey := &rsa.PublicKey{N: n, E: int(e.Int64())}
		keys = append(keys, rsaKey)
		kids = append(kids, k.Kid)
	}

	jwksCache.keys = keys
	jwksCache.kids = kids
	jwksCache.fetched = time.Now()

	return keys, kids, nil
}

// parseGoogleIDToken verifies and extracts the user profile from the Google
// OpenID Connect id_token. The token signature, audience, issuer, and expiry
// are all verified.
func parseGoogleIDToken(ctx context.Context, token *oauth2.Token, clientID string) (*auth.OAuthProfile, error) {
	raw := token.Extra("id_token")
	rawStr, ok := raw.(string)
	if !ok || rawStr == "" {
		return nil, errors.New("parseGoogleIDToken: id_token missing from token response")
	}

	// Split JWT into header, payload, signature.
	parts := strings.Split(rawStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("parseGoogleIDToken: malformed JWT")
	}

	// Decode and verify header.
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("parseGoogleIDToken: base64 decode header: " + err.Error())
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, errors.New("parseGoogleIDToken: parse header: " + err.Error())
	}
	if header.Alg != "RS256" {
		return nil, errors.New("parseGoogleIDToken: unsupported algorithm: " + header.Alg)
	}

	// Decode payload.
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("parseGoogleIDToken: base64 decode payload: " + err.Error())
	}

	// Verify signature against Google's public keys.
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("parseGoogleIDToken: base64 decode signature: " + err.Error())
	}

	keys, kids, err := fetchJWKS(ctx)
	if err != nil {
		return nil, fmt.Errorf("parseGoogleIDToken: fetch keys: %w", err)
	}

	var verified bool
	for i, key := range keys {
		if header.Kid != "" && kids[i] != header.Kid {
			continue
		}
		hashed := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(key, 0, hashed[:], signature); err == nil {
			verified = true
			break
		}
	}
	if !verified {
		return nil, errors.New("parseGoogleIDToken: signature verification failed")
	}

	// Verify claims.
	var claims struct {
		Sub      string `json:"sub"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Aud      string `json:"aud"`
		Iss      string `json:"iss"`
		Exp      int64  `json:"exp"`
		Iat      int64  `json:"iat"`
		AuthTime int64  `json:"auth_time"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, errors.New("parseGoogleIDToken: parse claims: " + err.Error())
	}
	if claims.Aud != clientID {
		return nil, errors.New("parseGoogleIDToken: audience mismatch")
	}
	if claims.Iss != "accounts.google.com" && claims.Iss != "https://accounts.google.com" {
		return nil, errors.New("parseGoogleIDToken: invalid issuer")
	}
	if time.Now().After(time.Unix(claims.Exp, 0)) {
		return nil, errors.New("parseGoogleIDToken: token expired")
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
