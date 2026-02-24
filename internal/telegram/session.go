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
	Data      map[string]string // accumulated key-value pairs
	ExpiresAt time.Time
}

// sessions stores active sessions keyed by chat ID.
type sessionStore struct {
	mu   sync.RWMutex
	data map[int64]*Session
}

func newSessionStore() *sessionStore {
	return &sessionStore{data: make(map[int64]*Session)}
}

func (s *sessionStore) get(chatID int64) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.data[chatID]
	if !ok || time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *sessionStore) set(chatID int64, sess *Session) {
	sess.ExpiresAt = time.Now().Add(sessionTTL)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[chatID] = sess
}

func (s *sessionStore) touch(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.data[chatID]; ok {
		sess.ExpiresAt = time.Now().Add(sessionTTL)
	}
}

func (s *sessionStore) clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, chatID)
}
