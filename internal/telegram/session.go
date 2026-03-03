package telegram

import (
	"sync"
	"time"
)

// SessionStep identifies which step of a multi-step conversation the user is in.
type SessionStep string

const (
	// /adduser interactive flow
	StepAddUserUsername  SessionStep = "adduser_username"
	StepAddUserFirstName SessionStep = "adduser_firstname"
	StepAddUserLastName  SessionStep = "adduser_lastname"
	StepAddUserWeight    SessionStep = "adduser_weight"

	// /addepic interactive flow (team is picked via inline keyboard)
	StepAddEpicNumber SessionStep = "addepic_number"
	StepAddEpicName   SessionStep = "addepic_name"
	StepAddEpicDesc   SessionStep = "addepic_desc"

	// /addrisk interactive flow (epic is picked via inline keyboard)
	StepAddRiskDesc SessionStep = "addrisk_desc"

	// /score epic effort text-input flow
	StepScoreEpicEffort SessionStep = "score_epic_effort"

	// /renameuser interactive flow (user is picked via inline keyboard)
	StepRenameUserFirstName SessionStep = "renameuser_firstname"
	StepRenameUserLastName  SessionStep = "renameuser_lastname"

	// /changerate interactive flow (user is picked via inline keyboard)
	StepChangeRateWeight SessionStep = "changerate_weight"

	// delete confirmation
	StepConfirmDeleteEpic SessionStep = "confirm_delete_epic"
	StepConfirmDeleteRisk SessionStep = "confirm_delete_risk"
)

// sessionTTL is the inactivity timeout for a session.
const sessionTTL = 5 * time.Minute

// Session holds the state of a multi-step admin interaction for one chat.
type Session struct {
	Step      SessionStep
	ThreadID  int               // Telegram forum topic ID
	Username  string            // Telegram username of the session initiator
	MessageID int               // ID of the bot message to edit in-place
	Data      map[string]string // accumulated key-value pairs
	ExpiresAt time.Time
}

// sessionKey uniquely identifies a session by chat, thread and user.
type sessionKey struct {
	ChatID   int64
	ThreadID int
	Username string
}

// sessions stores active sessions keyed by (chatID, threadID, username).
type sessionStore struct {
	mu   sync.RWMutex
	data map[sessionKey]*Session
}

func newSessionStore() *sessionStore {
	return &sessionStore{data: make(map[sessionKey]*Session)}
}

func (s *sessionStore) get(key sessionKey) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.data[key]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *sessionStore) set(key sessionKey, sess *Session) {
	sess.ExpiresAt = time.Now().Add(sessionTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = sess
}

func (s *sessionStore) touch(key sessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.data[key]; ok {
		sess.ExpiresAt = time.Now().Add(sessionTTL)
	}
}

func (s *sessionStore) clear(key sessionKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

// findByChat returns the first active session for the given chatID, regardless of
// threadID/username. This is used when we need to find a session from a text message
// without knowing which user originally started it.
func (s *sessionStore) findByChat(chatID int64, threadID int) (*Session, sessionKey, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for k, sess := range s.data {
		if k.ChatID == chatID && k.ThreadID == threadID && !time.Now().After(sess.ExpiresAt) {
			return sess, k, true
		}
	}
	return nil, sessionKey{}, false
}
