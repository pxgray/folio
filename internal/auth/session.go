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
