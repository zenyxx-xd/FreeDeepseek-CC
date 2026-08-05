package session

import (
	"sync"
	"sync/atomic"
	"time"
)

type AgentSession struct {
	ClaudeSessionID   string
	DeepSeekSessionID string
	ParentMessageID   interface{} // int64 or string or nil
	ModelType         string
	ThinkingEnabled   bool
	lastUsedUnix      atomic.Int64
	MessageCount      int
	LastMessagesLen   int
}

func (s *AgentSession) GetLastUsed() time.Time {
	return time.Unix(0, s.lastUsedUnix.Load())
}

func (s *AgentSession) Touch() {
	s.lastUsedUnix.Store(time.Now().UnixNano())
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
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			sm.mu.Lock()
			now := time.Now().UnixNano()
			twoHoursNano := int64(2 * time.Hour)
			for k, s := range sm.sessions {
				if now-s.lastUsedUnix.Load() > twoHoursNano {
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
	s, ok := sm.sessions[claudeSessionID]
	sm.mu.RUnlock()

	if ok {
		s.Touch()
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
		MessageCount:      0,
		LastMessagesLen:   0,
	}
	s.Touch()
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
	s.MessageCount = 0
	s.LastMessagesLen = 0
	s.Touch()
	return s
}

func (sm *SessionManager) UpdateParentMessageID(claudeSessionID string, parentMsgID interface{}, messagesLen int) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.sessions[claudeSessionID]; ok {
		s.ParentMessageID = parentMsgID
		s.MessageCount++
		s.LastMessagesLen = messagesLen
		s.Touch()
	}
}

func (sm *SessionManager) ClearSession(claudeSessionID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.sessions, claudeSessionID)
}
