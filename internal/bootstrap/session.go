package bootstrap

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"rpc_plugin_system/internal/auth"
)

type Session struct {
	PluginID          string
	SessionID         string
	Token             []byte
	IssuedAt          time.Time
	ExpiresAt         time.Time
	Consumed          bool
	SubstrateKeyPair  *KeyPair
	PluginPublicKey   []byte
	SharedSecret      []byte
	SessionRootKey    []byte
}

type Manager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
}

func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Manager{sessions: map[string]*Session{}, ttl: ttl}
}

func (m *Manager) New(pluginID string, now time.Time) (*Session, error) {
	if pluginID == "" {
		return nil, fmt.Errorf("plugin id is required")
	}
	token, err := auth.NewToken()
	if err != nil {
		return nil, err
	}
	sid, err := newSessionID()
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	s := &Session{
		PluginID:         pluginID,
		SessionID:        sid,
		Token:            token,
		IssuedAt:         now,
		ExpiresAt:        now.Add(m.ttl),
		SubstrateKeyPair: kp,
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.SessionID] = s
	return cloneSession(s), nil
}

func (m *Manager) Consume(pluginID, sessionID string, token []byte, now time.Time) (*Session, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("unknown session")
	}
	if s.PluginID != pluginID {
		return nil, fmt.Errorf("plugin id mismatch")
	}
	if now.After(s.ExpiresAt) {
		delete(m.sessions, sessionID)
		return nil, fmt.Errorf("session expired")
	}
	if s.Consumed {
		return nil, fmt.Errorf("session already consumed")
	}
	if !auth.EqualToken(s.Token, token) {
		return nil, fmt.Errorf("token mismatch")
	}
	s.Consumed = true
	return cloneSession(s), nil
}

func (m *Manager) Delete(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, sessionID)
}

func newSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

func cloneSession(s *Session) *Session {
	cp := *s
	cp.Token = append([]byte(nil), s.Token...)
	cp.PluginPublicKey = append([]byte(nil), s.PluginPublicKey...)
	cp.SharedSecret = append([]byte(nil), s.SharedSecret...)
	cp.SessionRootKey = append([]byte(nil), s.SessionRootKey...)
	return &cp
}
