package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// OAuthConfig holds OAuth2 client credentials for GitHub and Google.
type OAuthConfig struct {
	GitHubClientID     string
	GitHubClientSecret string
	GoogleClientID     string
	GoogleClientSecret string
	BaseURL            string // e.g. "https://folio.example.com"
}

// OAuthProfile is the normalised user profile returned by a provider.
type OAuthProfile struct {
	ProviderID string
	Email      string
	Name       string
}

// GitHubOAuthConfig builds an *oauth2.Config for GitHub.
func GitHubOAuthConfig(cfg OAuthConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.GitHubClientID,
		ClientSecret: cfg.GitHubClientSecret,
		Scopes:       []string{"user:email"},
		Endpoint:     github.Endpoint,
		RedirectURL:  cfg.BaseURL + "/-/auth/github/callback",
	}
}

// GoogleOAuthConfig builds an *oauth2.Config for Google.
func GoogleOAuthConfig(cfg OAuthConfig) *oauth2.Config {
	return &oauth2.Config{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
		RedirectURL:  cfg.BaseURL + "/-/auth/google/callback",
	}
}

// FetchGitHubProfile exchanges token for the GitHub user profile.
func FetchGitHubProfile(ctx context.Context, token *oauth2.Token, cfg OAuthConfig) (*OAuthProfile, error) {
	ts := oauth2.StaticTokenSource(token)
	tc := oauth2.NewClient(ctx, ts)
	tc.Timeout = 10 * time.Second
	resp, err := tc.Get("https://api.github.com/user")
	if err != nil {
		return nil, fmt.Errorf("FetchGitHubProfile: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("FetchGitHubProfile: status %d: %s", resp.StatusCode, body)
	}
	var gh struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, fmt.Errorf("FetchGitHubProfile: decode: %w", err)
	}
	name := gh.Name
	if name == "" {
		name = gh.Login
	}
	return &OAuthProfile{
		ProviderID: fmt.Sprintf("%d", gh.ID),
		Email:      gh.Email,
		Name:       name,
	}, nil
}
