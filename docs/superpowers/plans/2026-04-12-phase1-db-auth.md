# Phase 1: DB + Auth Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the internal/db (SQLite store) and internal/auth (password, sessions, OAuth, middleware) packages as pure library code with no HTTP surface.

**Architecture:** SQLiteStore implements the db.Store interface using modernc.org/sqlite (pure Go). auth.Auth wraps the store with session management, bcrypt password hashing, OAuth2 config helpers, and HTTP middleware.

**Tech Stack:** modernc.org/sqlite, golang.org/x/crypto/bcrypt, golang.org/x/oauth2

---

## Task 1: Add dependencies

- [ ] Add `modernc.org/sqlite` (pure-Go SQLite, no CGO):
  ```bash
  cd /home/pxgray/src/g3doc-clone
  go get modernc.org/sqlite
  ```
- [ ] Promote `golang.org/x/oauth2` from indirect to direct:
  ```bash
  go get golang.org/x/oauth2
  ```
- [ ] Promote `golang.org/x/crypto` from indirect to direct:
  ```bash
  go get golang.org/x/crypto
  ```
- [ ] Tidy and verify:
  ```bash
  go mod tidy
  grep -E 'modernc|oauth2|crypto' go.mod
  ```
  Expected: all three appear as direct (no `// indirect`) in `require`.

- [ ] Commit:
  ```bash
  git add go.mod go.sum
  git commit -m "chore: add modernc.org/sqlite and promote oauth2/crypto to direct deps"
  ```

---

## Task 2: internal/db — types, Store interface, ErrSettingNotFound

- [ ] Create `internal/db/types.go`:
  ```go
  package db

  import "time"

  type User struct {
      ID        int64
      Email     string
      Name      string
      Password  string    // bcrypt hash; empty for OAuth-only accounts
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
      StaleTTLSecs  int64
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
  ```

- [ ] Create `internal/db/store.go`:
  ```go
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
      UpdateUser(ctx context.Context, u *User) error
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
  ```

- [ ] Verify it compiles (no test yet):
  ```bash
  go build ./internal/db/...
  ```

- [ ] Commit:
  ```bash
  git add internal/db/types.go internal/db/store.go
  git commit -m "feat(db): add types, Store interface, and ErrSettingNotFound"
  ```

---

## Task 3: SQLiteStore — Open, migrate, Close

- [ ] Write the test first in `internal/db/sqlite_test.go`:
  ```go
  package db_test

  import (
      "context"
      "testing"

      "github.com/pxgray/folio/internal/db"
  )

  func openTestDB(t *testing.T) db.Store {
      t.Helper()
      s, err := db.Open(":memory:")
      if err != nil {
          t.Fatalf("db.Open: %v", err)
      }
      t.Cleanup(func() { s.Close() })
      return s
  }

  func TestOpen(t *testing.T) {
      s := openTestDB(t)
      if s == nil {
          t.Fatal("expected non-nil store")
      }
  }
  ```

- [ ] Run the test — expect a compile error (SQLiteStore not yet defined):
  ```bash
  go test ./internal/db/... -run TestOpen -timeout 60s
  ```

- [ ] Create `internal/db/sqlite.go` with Open, migrate, Close:
  ```go
  package db

  import (
      "context"
      "database/sql"
      "fmt"

      _ "modernc.org/sqlite"
  )

  // SQLiteStore is a Store backed by a SQLite database file (or :memory:).
  type SQLiteStore struct {
      db *sql.DB
  }

  // Open opens (or creates) the SQLite database at path and runs migrations.
  // For tests, use ":memory:".
  func Open(path string) (*SQLiteStore, error) {
      dsn := path
      if path != ":memory:" {
          dsn = path + "?_journal_mode=WAL&_foreign_keys=on"
      } else {
          dsn = ":memory:?_foreign_keys=on"
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
  PRAGMA foreign_keys = ON;

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
  `

  func (s *SQLiteStore) migrate() error {
      _, err := s.db.ExecContext(context.Background(), schema)
      return err
  }
  ```

- [ ] Run the test again — expect pass:
  ```bash
  go test ./internal/db/... -run TestOpen -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): SQLiteStore Open/migrate/Close with in-memory test"
  ```

---

## Task 4: User CRUD

- [ ] Add tests to `internal/db/sqlite_test.go`:
  ```go
  func TestUserCRUD(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      // CreateUser sets ID
      u := &db.User{Email: "alice@example.com", Name: "Alice", IsAdmin: true}
      if err := s.CreateUser(ctx, u); err != nil {
          t.Fatalf("CreateUser: %v", err)
      }
      if u.ID == 0 {
          t.Fatal("expected ID to be set after CreateUser")
      }

      // GetUserByID
      got, err := s.GetUserByID(ctx, u.ID)
      if err != nil {
          t.Fatalf("GetUserByID: %v", err)
      }
      if got.Email != u.Email || got.Name != u.Name || !got.IsAdmin {
          t.Errorf("GetUserByID mismatch: %+v", got)
      }

      // GetUserByEmail
      got2, err := s.GetUserByEmail(ctx, "alice@example.com")
      if err != nil {
          t.Fatalf("GetUserByEmail: %v", err)
      }
      if got2.ID != u.ID {
          t.Errorf("GetUserByEmail ID mismatch")
      }

      // UpdateUser
      u.Name = "Alice Updated"
      if err := s.UpdateUser(ctx, u); err != nil {
          t.Fatalf("UpdateUser: %v", err)
      }
      got3, _ := s.GetUserByID(ctx, u.ID)
      if got3.Name != "Alice Updated" {
          t.Errorf("UpdateUser did not persist: %q", got3.Name)
      }

      // ListUsers
      _ = s.CreateUser(ctx, &db.User{Email: "bob@example.com", Name: "Bob"})
      users, err := s.ListUsers(ctx)
      if err != nil {
          t.Fatalf("ListUsers: %v", err)
      }
      if len(users) != 2 {
          t.Errorf("expected 2 users, got %d", len(users))
      }

      // DeleteUser cascades (no foreign-key violation)
      if err := s.DeleteUser(ctx, u.ID); err != nil {
          t.Fatalf("DeleteUser: %v", err)
      }
      _, err = s.GetUserByID(ctx, u.ID)
      if err == nil {
          t.Fatal("expected error after DeleteUser, got nil")
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/db/... -run TestUserCRUD -timeout 60s
  ```

- [ ] Add User CRUD methods to `internal/db/sqlite.go`. Helper for scanning a user row:
  ```go
  import (
      "database/sql"
      "errors"
      "time"
  )

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

  // nullString converts an empty Go string to a SQL NULL.
  func nullString(s string) sql.NullString {
      return sql.NullString{String: s, Valid: s != ""}
  }
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/db/... -run TestUserCRUD -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): User CRUD (CreateUser, GetUserByID/Email, Update, Delete, List)"
  ```

---

## Task 5: OAuth account CRUD

- [ ] Add tests to `internal/db/sqlite_test.go`:
  ```go
  func TestOAuthAccountCRUD(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      u := &db.User{Email: "carol@example.com", Name: "Carol"}
      _ = s.CreateUser(ctx, u)

      a := &db.OAuthAccount{UserID: u.ID, Provider: "github", ProviderID: "gh-123"}
      if err := s.CreateOAuthAccount(ctx, a); err != nil {
          t.Fatalf("CreateOAuthAccount: %v", err)
      }
      if a.ID == 0 {
          t.Fatal("expected ID to be set")
      }

      // GetUserByOAuth
      got, err := s.GetUserByOAuth(ctx, "github", "gh-123")
      if err != nil {
          t.Fatalf("GetUserByOAuth: %v", err)
      }
      if got.ID != u.ID {
          t.Errorf("GetUserByOAuth returned wrong user")
      }

      // ListOAuthAccounts
      accounts, err := s.ListOAuthAccounts(ctx, u.ID)
      if err != nil {
          t.Fatalf("ListOAuthAccounts: %v", err)
      }
      if len(accounts) != 1 || accounts[0].Provider != "github" {
          t.Errorf("unexpected accounts: %v", accounts)
      }

      // DeleteOAuthAccount
      if err := s.DeleteOAuthAccount(ctx, u.ID, "github"); err != nil {
          t.Fatalf("DeleteOAuthAccount: %v", err)
      }
      accounts2, _ := s.ListOAuthAccounts(ctx, u.ID)
      if len(accounts2) != 0 {
          t.Errorf("expected 0 accounts after delete, got %d", len(accounts2))
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/db/... -run TestOAuthAccountCRUD -timeout 60s
  ```

- [ ] Add OAuth methods to `internal/db/sqlite.go`:
  ```go
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
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/db/... -run TestOAuthAccountCRUD -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): OAuth account CRUD (Create, GetUserByOAuth, Delete, List)"
  ```

---

## Task 6: Session CRUD

- [ ] Add tests to `internal/db/sqlite_test.go`:
  ```go
  func TestSessionCRUD(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      u := &db.User{Email: "dave@example.com", Name: "Dave"}
      _ = s.CreateUser(ctx, u)

      sess := &db.Session{
          Token:     "tok-abc123",
          UserID:    u.ID,
          ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
          CreatedAt: time.Now().UTC(),
      }
      if err := s.CreateSession(ctx, sess); err != nil {
          t.Fatalf("CreateSession: %v", err)
      }

      // GetSession
      got, err := s.GetSession(ctx, "tok-abc123")
      if err != nil {
          t.Fatalf("GetSession: %v", err)
      }
      if got.UserID != u.ID {
          t.Errorf("wrong UserID: %d", got.UserID)
      }

      // TouchSession (extend expiry)
      newExpiry := time.Now().UTC().Add(60 * 24 * time.Hour)
      if err := s.TouchSession(ctx, "tok-abc123", newExpiry); err != nil {
          t.Fatalf("TouchSession: %v", err)
      }
      got2, _ := s.GetSession(ctx, "tok-abc123")
      if got2.ExpiresAt.Before(newExpiry.Add(-time.Second)) {
          t.Errorf("TouchSession did not update expiry")
      }

      // DeleteExpiredSessions — create an expired one first
      expired := &db.Session{
          Token:     "tok-expired",
          UserID:    u.ID,
          ExpiresAt: time.Now().UTC().Add(-time.Hour),
          CreatedAt: time.Now().UTC().Add(-time.Hour),
      }
      _ = s.CreateSession(ctx, expired)
      if err := s.DeleteExpiredSessions(ctx); err != nil {
          t.Fatalf("DeleteExpiredSessions: %v", err)
      }
      _, err = s.GetSession(ctx, "tok-expired")
      if err == nil {
          t.Fatal("expected error for expired/deleted session")
      }

      // DeleteUserSessions
      if err := s.DeleteUserSessions(ctx, u.ID); err != nil {
          t.Fatalf("DeleteUserSessions: %v", err)
      }
      _, err = s.GetSession(ctx, "tok-abc123")
      if err == nil {
          t.Fatal("expected session gone after DeleteUserSessions")
      }

      // DeleteSession (individual)
      sess2 := &db.Session{Token: "tok-xyz", UserID: u.ID,
          ExpiresAt: time.Now().UTC().Add(time.Hour), CreatedAt: time.Now().UTC()}
      _ = s.CreateSession(ctx, sess2)
      if err := s.DeleteSession(ctx, "tok-xyz"); err != nil {
          t.Fatalf("DeleteSession: %v", err)
      }
      _, err = s.GetSession(ctx, "tok-xyz")
      if err == nil {
          t.Fatal("expected error for deleted session")
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/db/... -run TestSessionCRUD -timeout 60s
  ```

- [ ] Add Session methods to `internal/db/sqlite.go`:
  ```go
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
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/db/... -run TestSessionCRUD -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): Session CRUD (Create, Get, Delete, DeleteUser, Touch, DeleteExpired)"
  ```

---

## Task 7: Repo CRUD

- [ ] Add tests to `internal/db/sqlite_test.go`:
  ```go
  func TestRepoCRUD(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      owner := &db.User{Email: "erin@example.com", Name: "Erin"}
      _ = s.CreateUser(ctx, owner)

      r := &db.Repo{
          OwnerID:   owner.ID,
          Host:      "github.com",
          RepoOwner: "acme",
          RepoName:  "docs",
          Status:    db.RepoStatusPending,
      }
      if err := s.CreateRepo(ctx, r); err != nil {
          t.Fatalf("CreateRepo: %v", err)
      }
      if r.ID == 0 {
          t.Fatal("expected ID to be set")
      }

      // GetRepo
      got, err := s.GetRepo(ctx, r.ID)
      if err != nil {
          t.Fatalf("GetRepo: %v", err)
      }
      if got.RepoName != "docs" || got.Status != db.RepoStatusPending {
          t.Errorf("GetRepo mismatch: %+v", got)
      }

      // GetRepoByKey
      got2, err := s.GetRepoByKey(ctx, "github.com", "acme", "docs")
      if err != nil {
          t.Fatalf("GetRepoByKey: %v", err)
      }
      if got2.ID != r.ID {
          t.Errorf("GetRepoByKey returned wrong ID")
      }

      // Key() helper
      if r.Key() != "github.com/acme/docs" {
          t.Errorf("Key() = %q", r.Key())
      }

      // ListReposByOwner
      repos, err := s.ListReposByOwner(ctx, owner.ID)
      if err != nil {
          t.Fatalf("ListReposByOwner: %v", err)
      }
      if len(repos) != 1 {
          t.Errorf("expected 1 repo, got %d", len(repos))
      }

      // ListAllRepos
      all, err := s.ListAllRepos(ctx)
      if err != nil {
          t.Fatalf("ListAllRepos: %v", err)
      }
      if len(all) != 1 {
          t.Errorf("expected 1 repo in ListAllRepos, got %d", len(all))
      }

      // UpdateRepoStatus
      if err := s.UpdateRepoStatus(ctx, r.ID, db.RepoStatusReady, ""); err != nil {
          t.Fatalf("UpdateRepoStatus: %v", err)
      }
      got3, _ := s.GetRepo(ctx, r.ID)
      if got3.Status != db.RepoStatusReady {
          t.Errorf("UpdateRepoStatus did not persist")
      }

      // UpdateRepo
      r.TrustedHTML = true
      r.StaleTTLSecs = 300
      if err := s.UpdateRepo(ctx, r); err != nil {
          t.Fatalf("UpdateRepo: %v", err)
      }
      got4, _ := s.GetRepo(ctx, r.ID)
      if !got4.TrustedHTML || got4.StaleTTLSecs != 300 {
          t.Errorf("UpdateRepo did not persist: %+v", got4)
      }

      // DeleteRepo
      if err := s.DeleteRepo(ctx, r.ID); err != nil {
          t.Fatalf("DeleteRepo: %v", err)
      }
      _, err = s.GetRepo(ctx, r.ID)
      if err == nil {
          t.Fatal("expected error after DeleteRepo")
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/db/... -run TestRepoCRUD -timeout 60s
  ```

- [ ] Add Repo CRUD methods to `internal/db/sqlite.go`. Helper to scan a repo row:
  ```go
  func scanRepo(row *sql.Row) (*Repo, error) {
      var r Repo
      var createdAt string
      err := row.Scan(
          &r.ID, &r.OwnerID, &r.Host, &r.RepoOwner, &r.RepoName,
          &r.RemoteURL, &r.WebhookSecret, &r.TrustedHTML, &r.StaleTTLSecs,
          &r.Status, &r.StatusMsg, &createdAt,
      )
      if errors.Is(err, sql.ErrNoRows) {
          return nil, fmt.Errorf("repo not found: %w", err)
      }
      if err != nil {
          return nil, err
      }
      r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
      return &r, nil
  }

  const repoColumns = `id, owner_id, host, repo_owner, repo_name,
      remote_url, webhook_secret, trusted_html, stale_ttl_secs,
      status, status_msg, created_at`

  func (s *SQLiteStore) CreateRepo(ctx context.Context, r *Repo) error {
      r.CreatedAt = time.Now().UTC()
      if r.Status == "" {
          r.Status = RepoStatusPending
      }
      res, err := s.db.ExecContext(ctx,
          `INSERT INTO repos (owner_id, host, repo_owner, repo_name,
              remote_url, webhook_secret, trusted_html, stale_ttl_secs,
              status, status_msg, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
          r.OwnerID, r.Host, r.RepoOwner, r.RepoName,
          r.RemoteURL, r.WebhookSecret, r.TrustedHTML, r.StaleTTLSecs,
          r.Status, r.StatusMsg, r.CreatedAt.Format(time.RFC3339),
      )
      if err != nil {
          return fmt.Errorf("CreateRepo: %w", err)
      }
      r.ID, err = res.LastInsertId()
      return err
  }

  func (s *SQLiteStore) GetRepo(ctx context.Context, id int64) (*Repo, error) {
      row := s.db.QueryRowContext(ctx,
          `SELECT `+repoColumns+` FROM repos WHERE id = ?`, id)
      return scanRepo(row)
  }

  func (s *SQLiteStore) GetRepoByKey(ctx context.Context, host, repoOwner, repoName string) (*Repo, error) {
      row := s.db.QueryRowContext(ctx,
          `SELECT `+repoColumns+` FROM repos WHERE host=? AND repo_owner=? AND repo_name=?`,
          host, repoOwner, repoName)
      return scanRepo(row)
  }

  func (s *SQLiteStore) ListReposByOwner(ctx context.Context, ownerID int64) ([]*Repo, error) {
      return s.listRepos(ctx, `WHERE owner_id = ?`, ownerID)
  }

  func (s *SQLiteStore) ListAllRepos(ctx context.Context) ([]*Repo, error) {
      return s.listRepos(ctx, ``, nil)
  }

  func (s *SQLiteStore) listRepos(ctx context.Context, where string, arg any) ([]*Repo, error) {
      q := `SELECT ` + repoColumns + ` FROM repos ` + where + ` ORDER BY id`
      var rows *sql.Rows
      var err error
      if arg != nil {
          rows, err = s.db.QueryContext(ctx, q, arg)
      } else {
          rows, err = s.db.QueryContext(ctx, q)
      }
      if err != nil {
          return nil, err
      }
      defer rows.Close()
      var repos []*Repo
      for rows.Next() {
          var r Repo
          var createdAt string
          if err := rows.Scan(
              &r.ID, &r.OwnerID, &r.Host, &r.RepoOwner, &r.RepoName,
              &r.RemoteURL, &r.WebhookSecret, &r.TrustedHTML, &r.StaleTTLSecs,
              &r.Status, &r.StatusMsg, &createdAt,
          ); err != nil {
              return nil, err
          }
          r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
          repos = append(repos, &r)
      }
      return repos, rows.Err()
  }

  func (s *SQLiteStore) UpdateRepo(ctx context.Context, r *Repo) error {
      _, err := s.db.ExecContext(ctx,
          `UPDATE repos SET owner_id=?, host=?, repo_owner=?, repo_name=?,
              remote_url=?, webhook_secret=?, trusted_html=?, stale_ttl_secs=?,
              status=?, status_msg=?
           WHERE id=?`,
          r.OwnerID, r.Host, r.RepoOwner, r.RepoName,
          r.RemoteURL, r.WebhookSecret, r.TrustedHTML, r.StaleTTLSecs,
          r.Status, r.StatusMsg, r.ID,
      )
      return err
  }

  func (s *SQLiteStore) UpdateRepoStatus(ctx context.Context, id int64, status, msg string) error {
      _, err := s.db.ExecContext(ctx,
          `UPDATE repos SET status=?, status_msg=? WHERE id=?`, status, msg, id)
      return err
  }

  func (s *SQLiteStore) DeleteRepo(ctx context.Context, id int64) error {
      _, err := s.db.ExecContext(ctx, `DELETE FROM repos WHERE id = ?`, id)
      return err
  }
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/db/... -run TestRepoCRUD -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): Repo CRUD (Create, Get, GetByKey, List, Update, UpdateStatus, Delete)"
  ```

---

## Task 8: Settings + Artifacts CRUD

- [ ] Add tests to `internal/db/sqlite_test.go`:
  ```go
  func TestSettings(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      // GetSetting on missing key returns ErrSettingNotFound
      _, err := s.GetSetting(ctx, "setup_complete")
      if !errors.Is(err, db.ErrSettingNotFound) {
          t.Fatalf("expected ErrSettingNotFound, got %v", err)
      }

      // UpsertSetting inserts
      if err := s.UpsertSetting(ctx, "setup_complete", "false"); err != nil {
          t.Fatalf("UpsertSetting insert: %v", err)
      }
      val, err := s.GetSetting(ctx, "setup_complete")
      if err != nil || val != "false" {
          t.Fatalf("GetSetting: err=%v val=%q", err, val)
      }

      // IsSetupComplete — false
      done, err := s.IsSetupComplete(ctx)
      if err != nil || done {
          t.Fatalf("IsSetupComplete expected false, got %v / %v", done, err)
      }

      // UpsertSetting updates
      if err := s.UpsertSetting(ctx, "setup_complete", "true"); err != nil {
          t.Fatalf("UpsertSetting update: %v", err)
      }
      done2, _ := s.IsSetupComplete(ctx)
      if !done2 {
          t.Fatal("IsSetupComplete expected true after upsert")
      }
  }

  func TestArtifacts(t *testing.T) {
      ctx := context.Background()
      s := openTestDB(t)

      owner := &db.User{Email: "frank@example.com", Name: "Frank"}
      _ = s.CreateUser(ctx, owner)
      repo := &db.Repo{OwnerID: owner.ID, Host: "github.com", RepoOwner: "x", RepoName: "y"}
      _ = s.CreateRepo(ctx, repo)

      arts := map[string]string{"app": "/app", "docs": "/docs"}
      if err := s.SetRepoArtifacts(ctx, repo.ID, arts); err != nil {
          t.Fatalf("SetRepoArtifacts: %v", err)
      }

      got, err := s.GetRepoArtifacts(ctx, repo.ID)
      if err != nil {
          t.Fatalf("GetRepoArtifacts: %v", err)
      }
      if got["app"] != "/app" || got["docs"] != "/docs" {
          t.Errorf("unexpected artifacts: %v", got)
      }

      // Replace artifacts (SetRepoArtifacts is replace-all)
      arts2 := map[string]string{"new": "/new"}
      _ = s.SetRepoArtifacts(ctx, repo.ID, arts2)
      got2, _ := s.GetRepoArtifacts(ctx, repo.ID)
      if len(got2) != 1 || got2["new"] != "/new" {
          t.Errorf("expected replace-all, got: %v", got2)
      }
  }
  ```

- [ ] Run failing tests:
  ```bash
  go test ./internal/db/... -run "TestSettings|TestArtifacts" -timeout 60s
  ```

- [ ] Add Settings and Artifacts methods to `internal/db/sqlite.go`:
  ```go
  func (s *SQLiteStore) GetSetting(ctx context.Context, key string) (string, error) {
      var value string
      err := s.db.QueryRowContext(ctx,
          `SELECT value FROM server_settings WHERE key = ?`, key).Scan(&value)
      if errors.Is(err, sql.ErrNoRows) {
          return "", ErrSettingNotFound
      }
      return value, err
  }

  func (s *SQLiteStore) UpsertSetting(ctx context.Context, key, value string) error {
      _, err := s.db.ExecContext(ctx,
          `INSERT INTO server_settings (key, value) VALUES (?, ?)
           ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
          key, value)
      return err
  }

  func (s *SQLiteStore) IsSetupComplete(ctx context.Context) (bool, error) {
      val, err := s.GetSetting(ctx, "setup_complete")
      if errors.Is(err, ErrSettingNotFound) {
          return false, nil
      }
      if err != nil {
          return false, err
      }
      return val == "true", nil
  }

  func (s *SQLiteStore) SetRepoArtifacts(ctx context.Context, repoID int64, artifacts map[string]string) error {
      tx, err := s.db.BeginTx(ctx, nil)
      if err != nil {
          return err
      }
      defer tx.Rollback()
      if _, err := tx.ExecContext(ctx,
          `DELETE FROM repo_web_artifacts WHERE repo_id = ?`, repoID); err != nil {
          return err
      }
      for name, path := range artifacts {
          if _, err := tx.ExecContext(ctx,
              `INSERT INTO repo_web_artifacts (repo_id, name, path) VALUES (?, ?, ?)`,
              repoID, name, path); err != nil {
              return err
          }
      }
      return tx.Commit()
  }

  func (s *SQLiteStore) GetRepoArtifacts(ctx context.Context, repoID int64) (map[string]string, error) {
      rows, err := s.db.QueryContext(ctx,
          `SELECT name, path FROM repo_web_artifacts WHERE repo_id = ?`, repoID)
      if err != nil {
          return nil, err
      }
      defer rows.Close()
      result := make(map[string]string)
      for rows.Next() {
          var name, path string
          if err := rows.Scan(&name, &path); err != nil {
              return nil, err
          }
          result[name] = path
      }
      return result, rows.Err()
  }
  ```

- [ ] Run all db tests to confirm nothing is broken:
  ```bash
  go test ./internal/db/... -timeout 60s -v
  ```

- [ ] Commit:
  ```bash
  git add internal/db/sqlite.go internal/db/sqlite_test.go
  git commit -m "feat(db): Settings (GetSetting, UpsertSetting, IsSetupComplete) and Artifacts CRUD"
  ```

---

## Task 9: internal/auth — password hashing

- [ ] Write the test first in `internal/auth/password_test.go`:
  ```go
  package auth_test

  import (
      "testing"

      "github.com/pxgray/folio/internal/auth"
  )

  func TestHashAndCheckPassword(t *testing.T) {
      hash, err := auth.HashPassword("hunter2")
      if err != nil {
          t.Fatalf("HashPassword: %v", err)
      }
      if hash == "" {
          t.Fatal("expected non-empty hash")
      }
      if hash == "hunter2" {
          t.Fatal("hash must not equal plaintext")
      }

      if !auth.CheckPassword(hash, "hunter2") {
          t.Error("CheckPassword should return true for correct password")
      }
      if auth.CheckPassword(hash, "wrong") {
          t.Error("CheckPassword should return false for wrong password")
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/auth/... -run TestHashAndCheckPassword -timeout 60s
  ```

- [ ] Create `internal/auth/password.go`:
  ```go
  package auth

  import (
      "fmt"

      "golang.org/x/crypto/bcrypt"
  )

  const bcryptCost = 12

  // HashPassword returns a bcrypt hash of password at cost 12.
  func HashPassword(password string) (string, error) {
      b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
      if err != nil {
          return "", fmt.Errorf("HashPassword: %w", err)
      }
      return string(b), nil
  }

  // CheckPassword reports whether password matches the bcrypt hash.
  func CheckPassword(hash, password string) bool {
      return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
  }
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/auth/... -run TestHashAndCheckPassword -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/auth/password.go internal/auth/password_test.go
  git commit -m "feat(auth): HashPassword and CheckPassword using bcrypt cost 12"
  ```

---

## Task 10: internal/auth — Auth struct + session management

- [ ] Write the test first in `internal/auth/session_test.go`:
  ```go
  package auth_test

  import (
      "context"
      "testing"
      "time"

      "github.com/pxgray/folio/internal/auth"
      "github.com/pxgray/folio/internal/db"
  )

  func newTestAuth(t *testing.T) (*auth.Auth, db.Store) {
      t.Helper()
      store, err := db.Open(":memory:")
      if err != nil {
          t.Fatalf("db.Open: %v", err)
      }
      t.Cleanup(func() { store.Close() })
      return auth.New(store), store
  }

  func TestNewAndValidateSession(t *testing.T) {
      ctx := context.Background()
      a, store := newTestAuth(t)

      u := &db.User{Email: "grace@example.com", Name: "Grace"}
      if err := store.CreateUser(ctx, u); err != nil {
          t.Fatalf("CreateUser: %v", err)
      }

      sess, err := a.NewSession(ctx, u.ID)
      if err != nil {
          t.Fatalf("NewSession: %v", err)
      }
      if len(sess.Token) == 0 {
          t.Fatal("expected non-empty token")
      }
      if sess.ExpiresAt.Before(time.Now().Add(29 * 24 * time.Hour)) {
          t.Fatal("expected 30-day expiry")
      }

      // ValidateSession returns the user
      got, err := a.ValidateSession(ctx, sess.Token)
      if err != nil {
          t.Fatalf("ValidateSession: %v", err)
      }
      if got.ID != u.ID {
          t.Errorf("ValidateSession returned wrong user ID: %d", got.ID)
      }

      // Unknown token returns error
      _, err = a.ValidateSession(ctx, "bad-token")
      if err == nil {
          t.Fatal("expected error for unknown token")
      }

      // Expired session returns error
      expSess := &db.Session{
          Token:     "expired-tok",
          UserID:    u.ID,
          ExpiresAt: time.Now().UTC().Add(-time.Minute),
          CreatedAt: time.Now().UTC(),
      }
      _ = store.CreateSession(ctx, expSess)
      _, err = a.ValidateSession(ctx, "expired-tok")
      if err == nil {
          t.Fatal("expected error for expired session")
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/auth/... -run TestNewAndValidateSession -timeout 60s
  ```

- [ ] Create `internal/auth/auth.go` (Auth struct + constructor):
  ```go
  package auth

  import "github.com/pxgray/folio/internal/db"

  // Auth provides session management, password hashing, and OAuth helpers.
  // It wraps a db.Store for persistence.
  type Auth struct {
      store db.Store
  }

  // New creates an Auth backed by the given store.
  func New(store db.Store) *Auth {
      return &Auth{store: store}
  }
  ```

- [ ] Create `internal/auth/session.go`:
  ```go
  package auth

  import (
      "context"
      "crypto/rand"
      "encoding/hex"
      "fmt"
      "time"

      "github.com/pxgray/folio/internal/db"
  )

  const sessionDuration = 30 * 24 * time.Hour

  // NewSession creates a new 30-day session for userID and persists it.
  // The token is 32 random bytes encoded as hex (64 characters).
  func (a *Auth) NewSession(ctx context.Context, userID int64) (db.Session, error) {
      var buf [32]byte
      if _, err := rand.Read(buf[:]); err != nil {
          return db.Session{}, fmt.Errorf("NewSession: rand: %w", err)
      }
      now := time.Now().UTC()
      sess := db.Session{
          Token:     hex.EncodeToString(buf[:]),
          UserID:    userID,
          ExpiresAt: now.Add(sessionDuration),
          CreatedAt: now,
      }
      if err := a.store.CreateSession(ctx, &sess); err != nil {
          return db.Session{}, fmt.Errorf("NewSession: %w", err)
      }
      return sess, nil
  }

  // ValidateSession looks up token, rejects expired sessions, and returns the
  // owning User. It does NOT slide the expiry — callers (middleware) do that
  // separately to avoid a DB write on every request.
  func (a *Auth) ValidateSession(ctx context.Context, token string) (*db.User, error) {
      sess, err := a.store.GetSession(ctx, token)
      if err != nil {
          return nil, fmt.Errorf("ValidateSession: %w", err)
      }
      if time.Now().UTC().After(sess.ExpiresAt) {
          // Lazy delete — don't block the response on cleanup
          _ = a.store.DeleteSession(ctx, token)
          return nil, fmt.Errorf("ValidateSession: session expired")
      }
      user, err := a.store.GetUserByID(ctx, sess.UserID)
      if err != nil {
          return nil, fmt.Errorf("ValidateSession: load user: %w", err)
      }
      return user, nil
  }
  ```

- [ ] Run passing test:
  ```bash
  go test ./internal/auth/... -run TestNewAndValidateSession -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/auth/auth.go internal/auth/session.go internal/auth/session_test.go
  git commit -m "feat(auth): Auth struct, NewSession (32-byte hex token, 30-day expiry), ValidateSession"
  ```

---

## Task 11: internal/auth — OAuth config helpers

- [ ] Create `internal/auth/oauth.go`. No test file is required for pure config construction, but include a compile-check test. Write `internal/auth/oauth.go`:
  ```go
  package auth

  import (
      "context"
      "encoding/json"
      "fmt"
      "io"
      "net/http"

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
      client := GitHubOAuthConfig(cfg).Client(ctx, token)
      resp, err := client.Get("https://api.github.com/user")
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
  ```

- [ ] Verify it compiles:
  ```bash
  go build ./internal/auth/...
  ```

- [ ] Commit:
  ```bash
  git add internal/auth/oauth.go
  git commit -m "feat(auth): GitHubOAuthConfig, GoogleOAuthConfig, FetchGitHubProfile"
  ```

---

## Task 12: internal/auth — middleware

- [ ] Write the test first in `internal/auth/middleware_test.go`:
  ```go
  package auth_test

  import (
      "context"
      "net/http"
      "net/http/httptest"
      "testing"
      "time"

      "github.com/pxgray/folio/internal/auth"
      "github.com/pxgray/folio/internal/db"
  )

  func TestRequireAuth_NoCookie(t *testing.T) {
      a, _ := newTestAuth(t)

      // Dashboard path → redirect
      req := httptest.NewRequest(http.MethodGet, "/-/dashboard/", nil)
      w := httptest.NewRecorder()
      auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          w.WriteHeader(http.StatusOK)
      })).ServeHTTP(w, req)
      if w.Code != http.StatusFound {
          t.Errorf("expected 302 redirect, got %d", w.Code)
      }
      if w.Header().Get("Location") != "/-/auth/login" {
          t.Errorf("unexpected redirect location: %q", w.Header().Get("Location"))
      }

      // API path → 401
      req2 := httptest.NewRequest(http.MethodGet, "/-/api/v1/repos", nil)
      w2 := httptest.NewRecorder()
      auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          w.WriteHeader(http.StatusOK)
      })).ServeHTTP(w2, req2)
      if w2.Code != http.StatusUnauthorized {
          t.Errorf("expected 401 for API path, got %d", w2.Code)
      }
  }

  func TestRequireAuth_ValidSession(t *testing.T) {
      ctx := context.Background()
      a, store := newTestAuth(t)

      u := &db.User{Email: "hank@example.com", Name: "Hank"}
      _ = store.CreateUser(ctx, u)
      sess, _ := a.NewSession(ctx, u.ID)

      req := httptest.NewRequest(http.MethodGet, "/-/dashboard/", nil)
      req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
      w := httptest.NewRecorder()

      var gotUser *db.User
      auth.RequireAuth(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          gotUser = auth.UserFromContext(r.Context())
          w.WriteHeader(http.StatusOK)
      })).ServeHTTP(w, req)

      if w.Code != http.StatusOK {
          t.Errorf("expected 200, got %d", w.Code)
      }
      if gotUser == nil || gotUser.ID != u.ID {
          t.Errorf("UserFromContext returned wrong user: %v", gotUser)
      }
  }

  func TestRequireAdmin_NonAdmin(t *testing.T) {
      ctx := context.Background()
      a, store := newTestAuth(t)

      u := &db.User{Email: "ivan@example.com", Name: "Ivan", IsAdmin: false}
      _ = store.CreateUser(ctx, u)
      sess, _ := a.NewSession(ctx, u.ID)

      req := httptest.NewRequest(http.MethodGet, "/-/dashboard/admin/", nil)
      req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
      w := httptest.NewRecorder()

      auth.RequireAdmin(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          w.WriteHeader(http.StatusOK)
      })).ServeHTTP(w, req)

      if w.Code != http.StatusForbidden {
          t.Errorf("expected 403 for non-admin, got %d", w.Code)
      }
  }

  func TestRequireAdmin_Admin(t *testing.T) {
      ctx := context.Background()
      a, store := newTestAuth(t)

      u := &db.User{Email: "judy@example.com", Name: "Judy", IsAdmin: true}
      _ = store.CreateUser(ctx, u)
      sess, _ := a.NewSession(ctx, u.ID)

      req := httptest.NewRequest(http.MethodGet, "/-/dashboard/admin/", nil)
      req.AddCookie(&http.Cookie{Name: "session", Value: sess.Token})
      w := httptest.NewRecorder()

      auth.RequireAdmin(a)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          w.WriteHeader(http.StatusOK)
      })).ServeHTTP(w, req)

      if w.Code != http.StatusOK {
          t.Errorf("expected 200 for admin, got %d", w.Code)
      }
  }

  // Ensure session expiry is not in the past (sanity check on NewSession).
  func TestSessionExpiry(t *testing.T) {
      ctx := context.Background()
      a, store := newTestAuth(t)
      u := &db.User{Email: "ken@example.com", Name: "Ken"}
      _ = store.CreateUser(ctx, u)
      sess, _ := a.NewSession(ctx, u.ID)
      if sess.ExpiresAt.Before(time.Now().Add(29 * 24 * time.Hour)) {
          t.Errorf("session expiry too short: %v", sess.ExpiresAt)
      }
  }
  ```

- [ ] Run failing test:
  ```bash
  go test ./internal/auth/... -run "TestRequireAuth|TestRequireAdmin|TestSessionExpiry" -timeout 60s
  ```

- [ ] Create `internal/auth/middleware.go`:
  ```go
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
  ```

- [ ] Run passing tests:
  ```bash
  go test ./internal/auth/... -timeout 60s -v
  ```

- [ ] Run the full Phase 1 suite to confirm everything is green:
  ```bash
  go test ./internal/db/... ./internal/auth/... -timeout 60s
  ```

- [ ] Commit:
  ```bash
  git add internal/auth/middleware.go internal/auth/middleware_test.go
  git commit -m "feat(auth): RequireAuth, RequireAdmin middleware, UserFromContext"
  ```

---

## Phase 1 Complete

At the end of Task 12 the following should be true:

- `go test ./internal/db/... ./internal/auth/... -timeout 60s` passes with zero failures.
- `go build ./...` compiles without errors.
- No changes to `internal/web`, `internal/gitstore`, `internal/config`, `cmd/`, or `main.go`.
- The `db.Store` interface is the only public contract; nothing outside `internal/db` and `internal/auth` imports these packages yet.
