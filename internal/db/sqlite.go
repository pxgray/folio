package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStore is a Store backed by a SQLite database file (or :memory:).
type SQLiteStore struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and runs migrations.
// For tests, use ":memory:".
func Open(path string) (*SQLiteStore, error) {
	var dsn string
	if path == ":memory:" {
		dsn = ":memory:?_foreign_keys=on"
	} else {
		dsn = path + "?_journal_mode=WAL&_foreign_keys=on"
	}
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("db.Open: %w", err)
	}
	// SQLite writers must be serialized.
	sqlDB.SetMaxOpenConns(1)

	s := &SQLiteStore{db: sqlDB}
	if err := s.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("db.Open migrate: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY,
    email      TEXT UNIQUE NOT NULL,
    name       TEXT NOT NULL,
    password   TEXT,
    is_admin   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS oauth_accounts (
    id          INTEGER PRIMARY KEY,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider    TEXT NOT NULL,
    provider_id TEXT NOT NULL,
    UNIQUE(provider, provider_id)
);

CREATE TABLE IF NOT EXISTS sessions (
    token      TEXT PRIMARY KEY,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS repos (
    id             INTEGER PRIMARY KEY,
    owner_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    host           TEXT NOT NULL,
    repo_owner     TEXT NOT NULL,
    repo_name      TEXT NOT NULL,
    remote_url     TEXT NOT NULL DEFAULT '',
    webhook_secret TEXT NOT NULL DEFAULT '',
    trusted_html   BOOLEAN NOT NULL DEFAULT FALSE,
    stale_ttl_secs INTEGER NOT NULL DEFAULT 0,
    status         TEXT NOT NULL DEFAULT 'pending_clone',
    status_msg     TEXT NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL,
    UNIQUE(host, repo_owner, repo_name)
);

CREATE TABLE IF NOT EXISTS server_settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS repo_web_artifacts (
    id      INTEGER PRIMARY KEY,
    repo_id INTEGER NOT NULL REFERENCES repos(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    path    TEXT NOT NULL,
    UNIQUE(repo_id, name)
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_repos_owner_id ON repos(owner_id);
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_id ON oauth_accounts(user_id);
`

func (s *SQLiteStore) migrate() error {
	_, err := s.db.ExecContext(context.Background(), schema)
	return err
}

// --- Stub implementations (to be replaced in later tasks) ---

func (s *SQLiteStore) CreateUser(ctx context.Context, u *User) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	panic("not implemented")
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	panic("not implemented")
}

func (s *SQLiteStore) UpdateUser(ctx context.Context, u *User) error {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteUser(ctx context.Context, id int64) error {
	panic("not implemented")
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]*User, error) {
	panic("not implemented")
}

func (s *SQLiteStore) CreateOAuthAccount(ctx context.Context, a *OAuthAccount) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteOAuthAccount(ctx context.Context, userID int64, provider string) error {
	panic("not implemented")
}

func (s *SQLiteStore) ListOAuthAccounts(ctx context.Context, userID int64) ([]*OAuthAccount, error) {
	panic("not implemented")
}

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *Session) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetSession(ctx context.Context, token string) (*Session, error) {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, token string) error {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteUserSessions(ctx context.Context, userID int64) error {
	panic("not implemented")
}

func (s *SQLiteStore) TouchSession(ctx context.Context, token string, expiresAt time.Time) error {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteExpiredSessions(ctx context.Context) error {
	panic("not implemented")
}

func (s *SQLiteStore) CreateRepo(ctx context.Context, r *Repo) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetRepo(ctx context.Context, id int64) (*Repo, error) {
	panic("not implemented")
}

func (s *SQLiteStore) GetRepoByKey(ctx context.Context, host, repoOwner, repoName string) (*Repo, error) {
	panic("not implemented")
}

func (s *SQLiteStore) ListReposByOwner(ctx context.Context, ownerID int64) ([]*Repo, error) {
	panic("not implemented")
}

func (s *SQLiteStore) ListAllRepos(ctx context.Context) ([]*Repo, error) {
	panic("not implemented")
}

func (s *SQLiteStore) UpdateRepo(ctx context.Context, r *Repo) error {
	panic("not implemented")
}

func (s *SQLiteStore) UpdateRepoStatus(ctx context.Context, id int64, status, msg string) error {
	panic("not implemented")
}

func (s *SQLiteStore) DeleteRepo(ctx context.Context, id int64) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetSetting(ctx context.Context, key string) (string, error) {
	panic("not implemented")
}

func (s *SQLiteStore) UpsertSetting(ctx context.Context, key, value string) error {
	panic("not implemented")
}

func (s *SQLiteStore) IsSetupComplete(ctx context.Context) (bool, error) {
	panic("not implemented")
}

func (s *SQLiteStore) SetRepoArtifacts(ctx context.Context, repoID int64, artifacts map[string]string) error {
	panic("not implemented")
}

func (s *SQLiteStore) GetRepoArtifacts(ctx context.Context, repoID int64) (map[string]string, error) {
	panic("not implemented")
}

// Compile-time interface check.
var _ Store = (*SQLiteStore)(nil)
