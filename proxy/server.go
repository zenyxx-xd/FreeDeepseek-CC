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

	"freedeepseek-cc/models"
	"freedeepseek-cc/pow"
	"freedeepseek-cc/session"
)

type AuthConfig struct {
	Token   string `json:"token"`
	Cookie  string `json:"cookie"`
	WasmURL string `json:"wasmUrl"`
	HifDliq string `json:"hif_dliq"`
	HifLeim string `json:"hif_leim"`
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
		"/opt/freedeepseek-cc/deepseek-auth.json",
		filepath.Join(os.Getenv("HOME"), ".config", "freedeepseek", "deepseek-auth.json"),
	}

	if prefix := os.Getenv("PREFIX"); prefix != "" {
		paths = append(paths, filepath.Join(prefix, "opt", "freedeepseek-cc", "deepseek-auth.json"))
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
		WriteTimeout: 0,
	}

	log.Printf("⚡ FreeDeepseek-CC Proxy server listening on http://localhost:%s", port)
	return server.ListenAndServe()
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": "FreeDeepseek-CC Proxy",
		"version": "2.5.0",
	})
}

type AnthropicMessageRequest struct {
	Model        string                 `json:"model"`
	Messages     []AnthropicMessage     `json:"messages"`
	System       interface{}            `json:"system,omitempty"`
	Tools        []AnthropicTool        `json:"tools,omitempty"`
	Stream       bool                   `json:"stream"`
	MaxTokens    int                    `json:"max_tokens"`
	Thinking     *AnthropicThinking     `json:"thinking,omitempty"`
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

type DeepSeekFragment struct {
	Type    string `json:"type"`
	Content string `json:"content"`
}

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

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

	effortLevel := ""
	if req.OutputConfig != nil && req.OutputConfig.Effort != "" {
		effortLevel = req.OutputConfig.Effort
	}

	hasThinkingParam := req.Thinking != nil && req.Thinking.Type == "enabled"
	modelCfg := models.ResolveModel(req.Model, hasThinkingParam)

	userPrompt := s.extractPromptText(req.Messages, req.System, req.Tools)
	userPrompt = models.ApplyEffortInstruction(effortLevel, userPrompt, modelCfg.ThinkingEnabled)
	log.Printf("Extracted user prompt: %q, Model: %s", userPrompt, req.Model)

	logEntry := map[string]interface{}{
		"timestamp":         time.Now().Format(time.RFC3339),
		"session_id":        claudeSessionID,
		"raw_model":         req.Model,
		"resolved_model":    modelCfg.DisplayName,
		"resolved_type":     modelCfg.ModelType,
		"resolved_thinking": modelCfg.ThinkingEnabled,
		"effort":            effortLevel,
		"raw_body":          string(bodyBytes),
		"extracted_prompt":  userPrompt,
	}
	if logData, err := json.Marshal(logEntry); err == nil {
		if f, err := os.OpenFile("/tmp/claude_raw_requests.jsonl", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
			f.Write(append(logData, '\n'))
			f.Close()
		}
	}

	sess, exists := s.sessionManager.GetSession(claudeSessionID)
	var deepseekSessionID string
	var parentMessageID interface{} = nil

	if exists {
		deepseekSessionID = sess.DeepSeekSessionID
		parentMessageID = sess.ParentMessageID
	} else {
		dsSessID, err := s.createDeepSeekSession()
		if err != nil {
			log.Printf("Error creating DeepSeek session: %v", err)
			http.Error(w, "DeepSeek session creation failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		deepseekSessionID = dsSessID
		sess = s.sessionManager.SetSession(claudeSessionID, deepseekSessionID)
	}

	powHeader, err := s.solvePoWChallenge()
	if err != nil {
		log.Printf("Error solving PoW: %v", err)
		http.Error(w, "PoW challenge failed: "+err.Error(), http.StatusBadGateway)
		return
	}

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

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 1. Send message_start
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

	var fragments []DeepSeekFragment
	var lastPath string
	var finalResponseMsgID interface{} = nil

	thinkingStarted := false
	thinkingStopped := false
	textStarted := false
	textStopped := false

	thinkingIndex := 0
	textIndex := 0
	if modelCfg.ThinkingEnabled {
		textIndex = 1
	}

	ensureThinkingStarted := func() {
		if modelCfg.ThinkingEnabled && !thinkingStarted {
			thinkingStarted = true
			s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         thinkingIndex,
				"content_block": map[string]string{"type": "thinking", "thinking": ""},
			})
		}
	}

	ensureThinkingStopped := func() {
		if thinkingStarted && !thinkingStopped {
			thinkingStopped = true
			s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": thinkingIndex,
			})
		}
	}

	ensureTextStarted := func() {
		if modelCfg.ThinkingEnabled {
			ensureThinkingStopped()
		}
		if !textStarted {
			textStarted = true
			s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         textIndex,
				"content_block": map[string]string{"type": "text", "text": ""},
			})
		}
	}

	ensureTextStopped := func() {
		if textStarted && !textStopped {
			textStopped = true
			s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": textIndex,
			})
		}
	}

	appendFragmentDelta := func(fragType string, textDelta string) {
		if textDelta == "" || textDelta == "FINISHED" {
			return
		}

		if fragType == "THINK" || fragType == "REASONING" {
			if modelCfg.ThinkingEnabled {
				ensureThinkingStarted()
				s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": thinkingIndex,
					"delta": map[string]string{
						"type":     "thinking_delta",
						"thinking": textDelta,
					},
				})
			}
		} else if fragType == "RESPONSE" || fragType == "SEARCH" {
			ensureTextStarted()
			s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": textIndex,
				"delta": map[string]string{
					"type": "text_delta",
					"text": textDelta,
				},
			})
		}
	}

	scanner := bufio.NewScanner(dsResp.Body)

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

		if respMsgID, ok := dataMap["response_message_id"]; ok && respMsgID != nil {
			finalResponseMsgID = respMsgID
		}

		if p, ok := dataMap["p"].(string); ok {
			lastPath = p
		}

		if val, ok := dataMap["v"]; ok && val != nil {
			if vMap, isMap := val.(map[string]interface{}); isMap {
				if respObj, hasResp := vMap["response"].(map[string]interface{}); hasResp {
					if msgID, hasID := respObj["message_id"]; hasID && msgID != nil {
						finalResponseMsgID = msgID
					}
					if frags, hasFrags := respObj["fragments"].([]interface{}); hasFrags {
						fragments = nil
						for _, f := range frags {
							if fMap, isFMap := f.(map[string]interface{}); isFMap {
								tStr, _ := fMap["type"].(string)
								cStr, _ := fMap["content"].(string)
								fragments = append(fragments, DeepSeekFragment{Type: tStr, Content: cStr})
								appendFragmentDelta(tStr, cStr)
							}
						}
					}
				}
			}

			if strings.HasSuffix(lastPath, "fragments") || lastPath == "response/fragments" {
				if fragList, isList := val.([]interface{}); isList {
					for _, f := range fragList {
						if fMap, isFMap := f.(map[string]interface{}); isFMap {
							tStr, _ := fMap["type"].(string)
							cStr, _ := fMap["content"].(string)
							frag := DeepSeekFragment{Type: tStr, Content: cStr}
							fragments = append(fragments, frag)
							appendFragmentDelta(tStr, cStr)
						}
					}
				}
			}

			if typeofString(val) {
				deltaStr := val.(string)
				fragType := "RESPONSE"
				if len(fragments) > 0 {
					lastFragIndex := len(fragments) - 1
					fragments[lastFragIndex].Content += deltaStr
					fragType = fragments[lastFragIndex].Type
				}
				appendFragmentDelta(fragType, deltaStr)
			}
		}
	}

	if finalResponseMsgID != nil {
		s.sessionManager.UpdateParentMessageID(claudeSessionID, finalResponseMsgID)
	}

	if modelCfg.ThinkingEnabled && thinkingStarted && !thinkingStopped {
		ensureThinkingStopped()
	}
	if textStarted && !textStopped {
		ensureTextStopped()
	} else if !textStarted {
		ensureTextStarted()
		ensureTextStopped()
	}

	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 100},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{"type": "message_stop"})
}

func typeofString(v interface{}) bool {
	_, ok := v.(string)
	return ok
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "OpenAI endpoint compatibility active", http.StatusOK)
}

func (s *Server) extractPromptText(messages []AnthropicMessage, system interface{}, tools []AnthropicTool) string {
	var sb strings.Builder

	if system != nil {
		switch sys := system.(type) {
		case string:
			if sys != "" {
				sb.WriteString("System: ")
				sb.WriteString(sys)
				sb.WriteString("\n\n")
			}
		case []interface{}:
			for _, item := range sys {
				if m, ok := item.(map[string]interface{}); ok {
					if text, ok := m["text"].(string); ok && text != "" {
						sb.WriteString("System: ")
						sb.WriteString(text)
						sb.WriteString("\n\n")
					}
				}
			}
		}
	}

	if len(tools) > 0 {
		sb.WriteString("[Available Tools]\n")
		for _, t := range tools {
			sb.WriteString("- ")
			sb.WriteString(t.Name)
			if t.Description != "" {
				sb.WriteString(": ")
				sb.WriteString(t.Description)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == "user" {
			switch c := msg.Content.(type) {
			case string:
				sb.WriteString(c)
			case []interface{}:
				for _, item := range c {
					if m, ok := item.(map[string]interface{}); ok {
						blockType, _ := m["type"].(string)

						if text, ok := m["text"].(string); ok && text != "" {
							sb.WriteString(text)
							sb.WriteString("\n")
						}

						if blockType == "tool_result" {
							if contentVal, exists := m["content"]; exists {
								switch cv := contentVal.(type) {
								case string:
									if cv != "" {
										sb.WriteString("\n[Tool Result]:\n")
										sb.WriteString(cv)
										sb.WriteString("\n")
									}
								case []interface{}:
									for _, subItem := range cv {
										if subMap, ok := subItem.(map[string]interface{}); ok {
											if subText, ok := subMap["text"].(string); ok && subText != "" {
												sb.WriteString("\n[Tool Result]:\n")
												sb.WriteString(subText)
												sb.WriteString("\n")
											}
										}
									}
								}
							}
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

	s.setWebHeaders(req)

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

	s.setWebHeaders(req)

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

func (s *Server) setWebHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36")
	req.Header.Set("x-client-platform", "web")
	req.Header.Set("x-client-version", "2.0.0")
	req.Header.Set("x-client-locale", "ru")
	req.Header.Set("x-client-timezone-offset", "14400")
	req.Header.Set("x-app-version", "2.0.0")
	req.Header.Set("Authorization", "Bearer "+s.auth.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://chat.deepseek.com")
	req.Header.Set("Referer", "https://chat.deepseek.com/")

	if s.auth.HifDliq != "" {
		req.Header.Set("x-hif-dliq", s.auth.HifDliq)
	}
	if s.auth.HifLeim != "" {
		req.Header.Set("x-hif-leim", s.auth.HifLeim)
	}
}

func (s *Server) sendDeepSeekCompletion(sessionID string, parentMsgID interface{}, modelCfg models.ModelConfig, prompt string, powHeader string) (*http.Response, error) {
	payload := map[string]interface{}{
		"chat_session_id":   sessionID,
		"parent_message_id": parentMsgID,
		"model_type":        modelCfg.ModelType,
		"prompt":            prompt,
		"ref_file_ids":      []interface{}{},
		"thinking_enabled":  modelCfg.ThinkingEnabled,
		"search_enabled":    modelCfg.SearchEnabled,
		"action":            nil,
		"preempt":           false,
	}

	pBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, "https://chat.deepseek.com/api/v0/chat/completion", bytes.NewReader(pBytes))
	if err != nil {
		return nil, err
	}

	s.setWebHeaders(req)
	req.Header.Set("X-DS-PoW-Response", powHeader)

	return s.httpClient.Do(req)
}

func (s *Server) sendSSE(w http.ResponseWriter, flusher http.Flusher, eventName string, data map[string]interface{}) {
	bytes, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventName, string(bytes))
	flusher.Flush()
}
