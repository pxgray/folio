package db

import (
	"context"
	"database/sql"
	"errors"
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

// --- Helper functions ---

func scanUser(row *sql.Row) (*User, error) {
	var u User
	var pw sql.NullString
	var createdAt string
	err := row.Scan(&u.ID, &u.Email, &u.Name, &pw, &u.IsAdmin, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	u.Password = pw.String
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

// nullString converts an empty Go string to a SQL NULL.
func nullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// --- User CRUD ---

func (s *SQLiteStore) CreateUser(ctx context.Context, u *User) error {
	u.CreatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (email, name, password, is_admin, created_at)
         VALUES (?, ?, ?, ?, ?)`,
		u.Email, u.Name, nullString(u.Password), u.IsAdmin,
		u.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("CreateUser: %w", err)
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (s *SQLiteStore) GetUserByID(ctx context.Context, id int64) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password, is_admin, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *SQLiteStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, password, is_admin, created_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *SQLiteStore) UpdateUser(ctx context.Context, u *User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=?, name=?, password=?, is_admin=? WHERE id=?`,
		u.Email, u.Name, nullString(u.Password), u.IsAdmin, u.ID)
	return err
}

func (s *SQLiteStore) DeleteUser(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *SQLiteStore) ListUsers(ctx context.Context) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, password, is_admin, created_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []*User
	for rows.Next() {
		var u User
		var pw sql.NullString
		var createdAt string
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &pw, &u.IsAdmin, &createdAt); err != nil {
			return nil, err
		}
		u.Password = pw.String
		u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		users = append(users, &u)
	}
	return users, rows.Err()
}

// --- Stub implementations (to be replaced in later tasks) ---

func (s *SQLiteStore) CreateOAuthAccount(ctx context.Context, a *OAuthAccount) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO oauth_accounts (user_id, provider, provider_id) VALUES (?, ?, ?)`,
		a.UserID, a.Provider, a.ProviderID)
	if err != nil {
		return fmt.Errorf("CreateOAuthAccount: %w", err)
	}
	a.ID, err = res.LastInsertId()
	return err
}

func (s *SQLiteStore) GetUserByOAuth(ctx context.Context, provider, providerID string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.email, u.name, u.password, u.is_admin, u.created_at
         FROM users u
         JOIN oauth_accounts o ON o.user_id = u.id
         WHERE o.provider = ? AND o.provider_id = ?`,
		provider, providerID)
	return scanUser(row)
}

func (s *SQLiteStore) DeleteOAuthAccount(ctx context.Context, userID int64, provider string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM oauth_accounts WHERE user_id = ? AND provider = ?`, userID, provider)
	return err
}

func (s *SQLiteStore) ListOAuthAccounts(ctx context.Context, userID int64) ([]*OAuthAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, provider, provider_id FROM oauth_accounts WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var accounts []*OAuthAccount
	for rows.Next() {
		var a OAuthAccount
		if err := rows.Scan(&a.ID, &a.UserID, &a.Provider, &a.ProviderID); err != nil {
			return nil, err
		}
		accounts = append(accounts, &a)
	}
	return accounts, rows.Err()
}

func (s *SQLiteStore) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		sess.Token, sess.UserID,
		sess.ExpiresAt.UTC().Format(time.RFC3339),
		sess.CreatedAt.UTC().Format(time.RFC3339),
	)
	return err
}

func (s *SQLiteStore) GetSession(ctx context.Context, token string) (*Session, error) {
	var sess Session
	var expiresAt, createdAt string
	err := s.db.QueryRowContext(ctx,
		`SELECT token, user_id, expires_at, created_at FROM sessions WHERE token = ?`, token,
	).Scan(&sess.Token, &sess.UserID, &expiresAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("session not found: %w", err)
	}
	if err != nil {
		return nil, err
	}
	sess.ExpiresAt, _ = time.Parse(time.RFC3339, expiresAt)
	sess.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &sess, nil
}

func (s *SQLiteStore) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *SQLiteStore) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *SQLiteStore) TouchSession(ctx context.Context, token string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET expires_at = ? WHERE token = ?`,
		expiresAt.UTC().Format(time.RFC3339), token)
	return err
}

func (s *SQLiteStore) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM sessions WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
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
