package db

import (
	"context"
	"errors"
	"time"
)

// ErrSettingNotFound is returned by GetSetting when the key does not exist.
var ErrSettingNotFound = errors.New("db: setting not found")

// Store is the persistence interface for all Folio data. All methods must be
// safe for concurrent use. Implementations must support context cancellation.
type Store interface {
	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id int64) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	DemoteAdmin(ctx context.Context, userID int64) (int64, error)
	UpdateUser(ctx context.Context, u *User, password *string) error
	DeleteUser(ctx context.Context, id int64) error
	ListUsers(ctx context.Context) ([]*User, error)

	// OAuth accounts
	CreateOAuthAccount(ctx context.Context, a *OAuthAccount) error
	GetUserByOAuth(ctx context.Context, provider, providerID string) (*User, error)
	DeleteOAuthAccount(ctx context.Context, userID int64, provider string) error
	ListOAuthAccounts(ctx context.Context, userID int64) ([]*OAuthAccount, error)

	// Sessions
	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, token string) (*Session, error)
	DeleteSession(ctx context.Context, token string) error
	DeleteUserSessions(ctx context.Context, userID int64) error
	TouchSession(ctx context.Context, token string, expiresAt time.Time) error
	DeleteExpiredSessions(ctx context.Context) error

	// Repos
	CreateRepo(ctx context.Context, r *Repo) error
	GetRepo(ctx context.Context, id int64) (*Repo, error)
	GetRepoByKey(ctx context.Context, host, repoOwner, repoName string) (*Repo, error)
	ListReposByOwner(ctx context.Context, ownerID int64) ([]*Repo, error)
	ListAllRepos(ctx context.Context) ([]*Repo, error)
	UpdateRepo(ctx context.Context, r *Repo) error
	UpdateRepoStatus(ctx context.Context, id int64, status, msg string) error
	DeleteRepo(ctx context.Context, id int64) error

	// Settings
	GetSetting(ctx context.Context, key string) (string, error)
	UpsertSetting(ctx context.Context, key, value string) error
	IsSetupComplete(ctx context.Context) (bool, error)

	// Artifacts
	SetRepoArtifacts(ctx context.Context, repoID int64, artifacts map[string]string) error
	GetRepoArtifacts(ctx context.Context, repoID int64) (map[string]string, error)

	Close() error
}
