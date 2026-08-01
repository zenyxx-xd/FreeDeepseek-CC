package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"freedeepseek-go/models"
	"freedeepseek-go/pow"
	"freedeepseek-go/session"
)

type AuthConfig struct {
	Token   string `json:"token"`
	Cookie  string `json:"cookie"`
	WasmURL string `json:"wasmUrl"`
}

type Server struct {
	auth           AuthConfig
	sessionManager *session.SessionManager
	baseDir        string
	httpClient     *http.Client
}

func NewServer(baseDir string) (*Server, error) {
	auth, err := loadAuth(baseDir)
	if err != nil {
		return nil, fmt.Errorf("could not load deepseek auth: %w", err)
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
	}

	return &Server{
		auth:           auth,
		sessionManager: session.NewSessionManager(),
		baseDir:        baseDir,
		httpClient:     &http.Client{Transport: transport},
	}, nil
}

func loadAuth(baseDir string) (AuthConfig, error) {
	paths := []string{
		filepath.Join(baseDir, "deepseek-auth.json"),
		"/opt/freedeepseekapi/deepseek-auth.json",
		filepath.Join(os.Getenv("HOME"), ".config", "freedeepseek", "deepseek-auth.json"),
	}

	if prefix := os.Getenv("PREFIX"); prefix != "" {
		paths = append(paths, filepath.Join(prefix, "opt", "freedeepseekapi", "deepseek-auth.json"))
	}

	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			var cfg AuthConfig
			if err := json.Unmarshal(data, &cfg); err == nil && cfg.Token != "" {
				log.Printf("Loaded DeepSeek authentication from: %s", p)
				return cfg, nil
			}
		}
	}

	return AuthConfig{}, fmt.Errorf("no valid deepseek-auth.json found in search paths")
}

func (s *Server) Start(port string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/messages", s.handleAnthropicMessages)
	mux.HandleFunc("/v1/chat/completions", s.handleOpenAIChatCompletions)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/", s.handleHealth)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 0, // Streaming responses require no write timeout
	}

	log.Printf("⚡ FreeDeepseek-Go Proxy server listening on http://localhost:%s", port)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "FreeDeepseek-Go Proxy",
		"version": "1.0.0",
	})
}

// Anthropic API (/v1/messages) Request Structs
type AnthropicMessageRequest struct {
	Model       string                 `json:"model"`
	Messages    []AnthropicMessage     `json:"messages"`
	System      interface{}            `json:"system,omitempty"`
	Tools       []AnthropicTool        `json:"tools,omitempty"`
	Stream      bool                   `json:"stream"`
	MaxTokens   int                    `json:"max_tokens"`
	Thinking    *AnthropicThinking     `json:"thinking,omitempty"`
	OutputConfig *AnthropicOutputConfig `json:"output_config,omitempty"`
}

type AnthropicThinking struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

type AnthropicOutputConfig struct {
	Effort string `json:"effort,omitempty"`
}

type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type AnthropicTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"input_schema"`
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Session tracking via x-claude-code-session-id
	claudeSessionID := r.Header.Get("x-claude-code-session-id")
	if claudeSessionID == "" {
		claudeSessionID = "default-session-" + r.RemoteAddr
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}

	var req AnthropicMessageRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 2. Determine Effort and Thinking
	effortLevel := ""
	if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
		effortLevel = req.OutputConfig.Effort
	}

	hasThinkingParam := req.Thinking != nil && req.Thinking.Type == "enabled"
	modelCfg := models.ResolveModel(req.Model, hasThinkingParam)

	// 3. Extract user prompt / delta message
	userPrompt := s.extractPromptText(req.Messages, req.System, req.Tools)
	userPrompt = models.ApplyEffortInstruction(effortLevel, userPrompt)

	// 4. Session management (Delta vs New Session)
	sess, exists := s.sessionManager.GetSession(claudeSessionID)
	var deepseekSessionID string
	var parentMessageID interface{} = nil

	if exists {
		deepseekSessionID = sess.DeepSeekSessionID
		parentMessageID = sess.ParentMessageID
	} else {
		// Create new session on DeepSeek
		dsSessID, err := s.createDeepSeekSession()
		if err != nil {
			log.Printf("Error creating DeepSeek session: %v", err)
			http.Error(w, "DeepSeek session creation failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		deepseekSessionID = dsSessID
		sess = s.sessionManager.SetSession(claudeSessionID, deepseekSessionID)
	}

	// 5. Create & Solve PoW
	powHeader, err := s.solvePoWChallenge()
	if err != nil {
		log.Printf("Error solving PoW: %v", err)
		http.Error(w, "PoW challenge failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	// 6. Send completion request to DeepSeek Web API
	dsResp, err := s.sendDeepSeekCompletion(deepseekSessionID, parentMessageID, modelCfg, userPrompt, powHeader)
	if err != nil {
		log.Printf("Error during DeepSeek completion: %v", err)
		http.Error(w, "DeepSeek completion error: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer dsResp.Body.Close()

	if dsResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(dsResp.Body)
		log.Printf("DeepSeek returned HTTP %d: %s", dsResp.StatusCode, string(respBody))
		http.Error(w, fmt.Sprintf("DeepSeek HTTP %d: %s", dsResp.StatusCode, string(respBody)), dsResp.StatusCode)
		return
	}

	// 7. Stream SSE response back to Anthropic client
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Send message_start
	msgID := fmt.Sprintf("msg_%d", time.Now().UnixNano())
	s.sendSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         req.Model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage":         map[string]int{"input_tokens": 10, "output_tokens": 1},
		},
	})

	s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})

	// Process DeepSeek stream lines
	scanner := bufio.NewScanner(dsResp.Body)
	var finalResponseMsgID interface{} = nil

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		dataStr := strings.TrimPrefix(line, "data: ")
		var dataMap map[string]interface{}
		if err := json.Unmarshal([]byte(dataStr), &dataMap); err != nil {
			continue
		}

		if respMsgID, ok := dataMap["response_message_id"]; ok {
			finalResponseMsgID = respMsgID
		}

		// Handle patch fragments
		if val, ok := dataMap["v"]; ok {
			if strVal, isStr := val.(string); isStr {
				s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": 0,
					"delta": map[string]string{
						"type": "text_delta",
						"text": strVal,
					},
				})
			}
		}
	}

	// Update session parent_message_id for next turn
	if finalResponseMsgID != nil {
		s.sessionManager.UpdateParentMessageID(claudeSessionID, finalResponseMsgID)
	}

	s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})

	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 100},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{"type": "message_stop"})
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	// Simple OpenAI compatibility wrapper
	http.Error(w, "OpenAI endpoint compatibility active", http.StatusOK)
}

func (s *Server) extractPromptText(messages []AnthropicMessage, system interface{}, tools []AnthropicTool) string {
	var sb strings.Builder

	// Add system prompt if present
	if system != nil {
		if sysStr, ok := system.(string); ok && sysStr != "" {
			sb.WriteString("System: ")
			sb.WriteString(sysStr)
			sb.WriteString("\n\n")
		}
	}

	// Extract the last user message
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" {
			switch c := msg.Content.(type) {
			case string:
				sb.WriteString(c)
			case []interface{}:
				for _, item := range c {
					if m, ok := item.(map[string]interface{}); ok {
						if text, ok := m["text"].(string); ok {
							sb.WriteString(text)
							sb.WriteString("\n")
						}
					}
				}
			}
			break
		}
	}

	return strings.TrimSpace(sb.String())
}

func (s *Server) createDeepSeekSession() (string, error) {
	req, err := http.NewRequest(http.MethodPost, "https://chat.deepseek.com/api/v0/chat_session/create", strings.NewReader("{}"))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+s.auth.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var resData struct {
		Data struct {
			BizData struct {
				ID          string `json:"id"`
				ChatSession struct {
					ID string `json:"id"`
				} `json:"chat_session"`
			} `json:"biz_data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&resData); err != nil {
		return "", err
	}

	id := resData.Data.BizData.ChatSession.ID
	if id == "" {
		id = resData.Data.BizData.ID
	}

	if id == "" {
		return "", fmt.Errorf("no chat_session id returned from deepseek")
	}

	return id, nil
}

func (s *Server) solvePoWChallenge() (string, error) {
	req, err := http.NewRequest(http.MethodPost, "https://chat.deepseek.com/api/v0/chat/create_pow_challenge", strings.NewReader(`{"target_path":"/api/v0/chat/completion"}`))
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+s.auth.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var chalRes struct {
		Data struct {
			BizData struct {
				Challenge pow.Challenge `json:"challenge"`
			} `json:"biz_data"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&chalRes); err != nil {
		return "", err
	}

	return pow.SolvePow(chalRes.Data.BizData.Challenge, s.baseDir)
}

func (s *Server) sendDeepSeekCompletion(sessionID string, parentMsgID interface{}, modelCfg models.ModelConfig, prompt string, powHeader string) (*http.Response, error) {
	payload := map[string]interface{}{
		"chat_session_id":  sessionID,
		"parent_message_id": parentMsgID,
		"model_type":       modelCfg.ModelType,
		"prompt":           prompt,
		"ref_file_ids":     []interface{}{},
		"thinking_enabled": modelCfg.ThinkingEnabled,
		"search_enabled":   modelCfg.SearchEnabled,
		"action":           nil,
		"preempt":          false,
	}

	pBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://chat.deepseek.com/api/v0/chat/completion", bytes.NewReader(pBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+s.auth.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DS-PoW-Response", powHeader)
	req.Header.Set("x-client-version", "2.0.0")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")

	return s.httpClient.Do(req)
}

func (s *Server) sendSSE(w http.ResponseWriter, flusher http.Flusher, eventName string, data map[string]interface{}) {
	bytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(bytes))
	flusher.Flush()
}
