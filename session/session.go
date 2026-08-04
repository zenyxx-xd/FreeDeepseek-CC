package session

import (
	"sync"
	"time"
)

type AgentSession struct {
	ClaudeSessionID   string
	DeepSeekSessionID string
	ParentMessageID   interface{} // int64 or string or nil
	ModelType         string
	ThinkingEnabled   bool
	LastUsed          time.Time
	MessageCount      int
}

type SessionManager struct {
	mu       sync.RWMutex
	sessions map[string]*AgentSession
}

func NewSessionManager() *SessionManager {
	sm := &SessionManager{
		sessions: make(map[string]*AgentSession),
	}
	// Background cleanup of inactive sessions older than 2 hours
	go func() {
		for {
			time.Sleep(10 * time.Minute)
			sm.mu.Lock()
			now := time.Now()
			for k, s := range sm.sessions {
				if now.Sub(s.LastUsed) > 2*time.Hour {
					delete(sm.sessions, k)
				}
			}
			sm.mu.Unlock()
		}
	}()
	return sm
}

func (sm *SessionManager) GetSession(claudeSessionID string) (*AgentSession, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.sessions[claudeSessionID]
	if ok {
		s.LastUsed = time.Now()
	}
	return s, ok
}

func (sm *SessionManager) SetSession(claudeSessionID, deepseekSessionID, modelType string, thinkingEnabled bool) *AgentSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s := &AgentSession{
		ClaudeSessionID:   claudeSessionID,
		DeepSeekSessionID: deepseekSessionID,
		ParentMessageID:   nil,
		ModelType:         modelType,
		ThinkingEnabled:   thinkingEnabled,
		LastUsed:          time.Now(),
		MessageCount:      0,
	}
	sm.sessions[claudeSessionID] = s
	return s
}

func (sm *SessionManager) ResetForModelSwitch(claudeSessionID, newDeepSeekSessionID, modelType string, thinkingEnabled bool) *AgentSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	s, ok := sm.sessions[claudeSessionID]
	if !ok {
		s = &AgentSession{
			ClaudeSessionID: claudeSessionID,
		}
		sm.sessions[claudeSessionID] = s
	}
	s.DeepSeekSessionID = newDeepSeekSessionID
	s.ParentMessageID = nil
	s.ModelType = modelType
	s.ThinkingEnabled = thinkingEnabled
	s.LastUsed = time.Now()
	return s
}

func (sm *SessionManager) UpdateParentMessageID(claudeSessionID string, parentMsgID interface{}) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[claudeSessionID]; ok {
		s.ParentMessageID = parentMsgID
		s.MessageCount++
		s.LastUsed = time.Now()
	}
}

func (sm *SessionManager) ClearSession(claudeSessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, claudeSessionID)
}
