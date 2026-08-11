// Package auth provides users, sessions, and the HTTP handlers and middleware
// that guard the rest of the API.
package auth

import "time"

// User represents an application user.
type User struct {
	ID           int64
	Username     string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
}

// Session represents an authenticated session stored in the database.
type Session struct {
	ID        string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}
