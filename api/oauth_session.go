package api

import (
	"sync"
	"time"

	db "github.com/The-You-School-HeadLamp/headlamp_backend/db/sqlc"
)

// OAuthSessionStatus defines the status of an OAuth session.
type OAuthSessionStatus int

const (
	StatusPending OAuthSessionStatus = iota
	StatusCompleted
	StatusFailed
)

// OAuthSessionData holds the data for a single OAuth session.
type OAuthSessionData struct {
	Status               OAuthSessionStatus
	AccessToken          string
	AccessTokenExpiresAt time.Time
	Parent               *db.Parent
	CreatedAt            time.Time
	ErrorMessage         string
}

// OAuthSessionStore is a thread-safe in-memory store for OAuth sessions.
type OAuthSessionStore struct {
	sessions map[string]OAuthSessionData
	mu       sync.RWMutex
}

// NewOAuthSessionStore creates a new session store and starts a cleanup routine.
func NewOAuthSessionStore(cleanupInterval time.Duration) *OAuthSessionStore {
	store := &OAuthSessionStore{
		sessions: make(map[string]OAuthSessionData),
	}

	go store.cleanupExpired(cleanupInterval)

	return store
}

// CreateSession initializes a new session with pending status.
func (s *OAuthSessionStore) CreateSession(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionID] = OAuthSessionData{
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
}

// CompleteSession marks a session as completed and stores the tokens.
func (s *OAuthSessionStore) CompleteSession(sessionID, accessToken string, expiresAt time.Time, parent *db.Parent) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[sessionID]; ok {
		session.Status = StatusCompleted
		session.AccessToken = accessToken
		session.AccessTokenExpiresAt = expiresAt
		session.Parent = parent
		s.sessions[sessionID] = session
	}
}

// FailSession marks a session as failed.
func (s *OAuthSessionStore) FailSession(sessionID, errorMessage string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.sessions[sessionID]; ok {
		session.Status = StatusFailed
		session.ErrorMessage = errorMessage
		s.sessions[sessionID] = session
	}
}

// GetAndClearSession retrieves a session's data and then deletes it.
func (s *OAuthSessionStore) GetAndClearSession(sessionID string) (OAuthSessionData, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if ok {
		delete(s.sessions, sessionID)
	}
	return session, ok
}

// PeekSession retrieves a session's data without deleting it.
func (s *OAuthSessionStore) PeekSession(sessionID string) (OAuthSessionData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	return session, ok
}

// cleanupExpired periodically removes old sessions.
func (s *OAuthSessionStore) cleanupExpired(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	const sessionTimeout = 5 * time.Minute
	const cleanupThreshold = 10 * time.Minute

	for range ticker.C {
		s.mu.Lock()
		for id, session := range s.sessions {
			// Clean up any session older than the max threshold.
			if time.Since(session.CreatedAt) > cleanupThreshold {
				delete(s.sessions, id)
				continue
			}

			// Mark pending sessions as failed if they have timed out.
			if session.Status == StatusPending && time.Since(session.CreatedAt) > sessionTimeout {
				session.Status = StatusFailed
				session.ErrorMessage = "Session timed out."
				s.sessions[id] = session
			}
		}
		s.mu.Unlock()
	}
}
