package db

import "time"

type User struct {
	ID        int64
	Email     string
	Name      string
	Password  string // bcrypt hash; empty for OAuth-only accounts
	IsAdmin   bool
	CreatedAt time.Time
}

type OAuthAccount struct {
	ID         int64
	UserID     int64
	Provider   string
	ProviderID string
}

type Session struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
	CSRFToken string
}

type Repo struct {
	ID            int64
	OwnerID       int64
	Host          string
	RepoOwner     string
	RepoName      string
	RemoteURL     string
	WebhookSecret string
	TrustedHTML   bool
	StaleTTLSecs  int64 // seconds; multiply by time.Second before passing to gitstore
	Status        string
	StatusMsg     string
	CreatedAt     time.Time
}

const (
	RepoStatusPending = "pending_clone"
	RepoStatusReady   = "ready"
	RepoStatusError   = "error"
)

// Key returns "host/repoOwner/repoName"
func (r *Repo) Key() string { return r.Host + "/" + r.RepoOwner + "/" + r.RepoName }
