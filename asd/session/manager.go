package session

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"google.golang.org/grpc"

	"telegram_project/asd/config"
)

// Manager holds all active sessions
type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
	cfg      *config.Config
}

func NewManager(cfg *config.Config) *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
		cfg:      cfg,
	}
}

// Create creates a new session
func (m *Manager) Create(ctx context.Context) (string, string, error) {
	sessionID := uuid.New().String()

	sessionCtx, cancel := context.WithCancel(context.Background())

	s, err := NewSession(sessionCtx, sessionID, m.cfg, cancel)
	if err != nil {
		cancel()
		return "", "", fmt.Errorf("new session: %w", err)
	}

	qrCode, err := s.Run(ctx)
	if err != nil {
		cancel()
		return "", "", fmt.Errorf("run session: %w", err)
	}

	m.mu.Lock()
	m.sessions[sessionID] = s
	m.mu.Unlock()

	return sessionID, qrCode, nil
}

// Delete stops and removes a session
func (m *Manager) Delete(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return nil
	}

	s.Stop()
	delete(m.sessions, sessionID)
	return nil
}

// get returns a session by ID
func (m *Manager) get(sessionID string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, errors.New("session not found")
	}
	return s, nil
}

// SendMessage sends a message via session
func (m *Manager) SendMessage(sessionID, peer, text string) (int64, error) {
	s, err := m.get(sessionID)
	if err != nil {
		return 0, err
	}
	return s.SendMessage(context.Background(), peer, text)
}

// SubscribeMessages subscribes to messages from a session
func (m *Manager) SubscribeMessages(sessionID string, stream grpc.ServerStream) error {
	s, err := m.get(sessionID)
	if err != nil {
		return err
	}
	return s.Subscribe(stream)
}
