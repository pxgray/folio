package db

import (
	_ "golang.org/x/crypto/bcrypt"
	_ "golang.org/x/oauth2"
	_ "modernc.org/sqlite"
)

// Package db provides database access and models for the folio dashboard.
// Dependencies are imported as blanks to ensure they're tracked as direct deps.
