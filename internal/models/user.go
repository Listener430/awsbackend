package models

import (
	"time"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // Password is never exposed in JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MoodCheckIn struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	MoodScore   int       `json:"mood_score"`   // PHI - encrypted at rest
	Notes       string    `json:"notes"`        // PHI - encrypted at rest
	Timestamp   time.Time `json:"timestamp"`
	CreatedAt   time.Time `json:"created_at"`
	Encrypted   bool      `json:"encrypted"`    // Track encryption status
}

// HIPAA-compliant models for Therma
type JournalEntry struct {
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	Content     string    `json:"content"`     // PHI - encrypted at rest
	Mood        string    `json:"mood"`        // PHI - encrypted at rest
	Tags        []string  `json:"tags"`        // PHI - encrypted at rest
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Encrypted   bool      `json:"encrypted"`   // Track encryption status
}

type IdempotencyKey struct {
	Key         string    `json:"key"`
	UserID      string    `json:"user_id"`
	RequestHash string    `json:"request_hash"`
	Response    string    `json:"response"`
	Status      string    `json:"status"`      // "pending", "completed", "failed"
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type UserSpendTracking struct {
	UserID      string    `json:"user_id"`
	Date        string    `json:"date"`        // YYYY-MM-DD format
	LLMRequests int       `json:"llm_requests"`
	LLMCost     float64   `json:"llm_cost"`
	DailyLimit  float64   `json:"daily_limit"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}