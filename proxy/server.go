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
	"sync/atomic"
	"time"

	"freedeepseek-cc/models"
	"freedeepseek-cc/pow"
	"freedeepseek-cc/session"
)

var msgCounter atomic.Uint64

func generateMsgID() string {
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), msgCounter.Add(1))
}

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
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     120 * time.Second,
		ForceAttemptHTTP2:   true,
	}

	return &Server{
		auth:           auth,
		sessionManager: session.NewSessionManager(),
		baseDir:        baseDir,
		httpClient:     &http.Client{Transport: transport, Timeout: 300 * time.Second},
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
		ReadTimeout:  300 * time.Second,
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
		"version": "1.0.0",
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

	sess, exists := s.sessionManager.GetSession(claudeSessionID)

	tempPrompt := s.extractPromptText(req.Messages, req.System, req.Tools, false)

	modelChanged := false
	isCompacted := false
	isUndo := false

	if exists && sess.MessageCount > 0 {
		if sess.ModelType != modelCfg.ModelType || sess.ThinkingEnabled != modelCfg.ThinkingEnabled {
			modelChanged = true
			log.Printf("Model switch detected for session %s (%s/thinking=%v -> %s/thinking=%v). Resetting DeepSeek session...",
				claudeSessionID, sess.ModelType, sess.ThinkingEnabled, modelCfg.ModelType, modelCfg.ThinkingEnabled)
		} else if isUndoRequest(tempPrompt, req.Messages, sess.MessageCount) {
			isUndo = true
			log.Printf("Undo / rewind request detected for session %s (history trimmed from %d to %d msgs). Creating new DeepSeek session and transferring history up to undo point...",
				claudeSessionID, sess.MessageCount, len(req.Messages))
		} else if isCompactRequest(tempPrompt, req.System, req.Messages, sess.MessageCount) {
			isCompacted = true
			log.Printf("Conversation compaction / /compact detected for session %s. Creating fresh session with summary...",
				claudeSessionID)
		}
	}

	isFirstTurn := (!exists || sess.MessageCount == 0 || modelChanged || isCompacted || isUndo)

	if modelChanged || isCompacted || isUndo {
		newDsSessID, err := s.createDeepSeekSession()
		if err != nil {
			log.Printf("Error creating new DeepSeek session on switch/compact/undo: %v", err)
			http.Error(w, "DeepSeek session switch failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		sess = s.sessionManager.ResetForModelSwitch(claudeSessionID, newDsSessID, modelCfg.ModelType, modelCfg.ThinkingEnabled)
		exists = true
	}

	userPrompt := s.extractPromptText(req.Messages, req.System, req.Tools, isFirstTurn)

	flusher, _ := w.(http.Flusher)

	if isInitialTestRequest(userPrompt) {
		log.Printf("Detected session warmup 'test' request. Replying with synthetic test response.")
		s.sendSyntheticTestResponse(w, flusher, req.Stream)
		return
	}

	if isTitleRequest(userPrompt, req.System) {
		log.Printf("Detected background Title Generation request. Intercepting with synthetic response.")
		s.sendSyntheticTitleResponse(w, flusher, req.Stream)
		return
	}

	if isSuggestionRequest(userPrompt) {
		log.Printf("Detected background Suggestion Mode request. Intercepting with synthetic response.")
		s.sendSyntheticEmptyResponse(w, flusher, req.Stream)
		return
	}

	userPrompt = models.ApplyEffortInstruction(effortLevel, userPrompt, modelCfg.ThinkingEnabled)
	log.Printf("Extracted user prompt (isFirstTurn=%v, modelChanged=%v): %q, Model: %s", isFirstTurn, modelChanged, userPrompt, req.Model)

	var deepseekSessionID string
	var parentMessageID interface{} = nil

	if exists {
		deepseekSessionID = sess.DeepSeekSessionID
		if sess.ParentMessageID != nil {
			switch v := sess.ParentMessageID.(type) {
			case float64:
				parentMessageID = int64(v)
			case int64:
				parentMessageID = v
			case int:
				parentMessageID = int64(v)
			default:
				parentMessageID = nil
			}
		}
	} else {
		dsSessID, err := s.createDeepSeekSession()
		if err != nil {
			log.Printf("Error creating DeepSeek session: %v", err)
			http.Error(w, "DeepSeek session creation failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		deepseekSessionID = dsSessID
		sess = s.sessionManager.SetSession(claudeSessionID, deepseekSessionID, modelCfg.ModelType, modelCfg.ThinkingEnabled)
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

	msgID := generateMsgID()
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

	var responseBuffer strings.Builder
	hasTools := len(req.Tools) > 0

	anthropicThinkingEnabled := hasThinkingParam

	thinkingStarted := false
	thinkingStopped := false
	textStarted := false
	textStopped := false

	thinkingIndex := 0
	textIndex := 0
	if anthropicThinkingEnabled {
		textIndex = 1
	}

	ensureThinkingStarted := func() {
		if anthropicThinkingEnabled && !thinkingStarted {
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
		if anthropicThinkingEnabled {
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
			if anthropicThinkingEnabled {
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
			if hasTools {
				responseBuffer.WriteString(textDelta)
			} else {
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
	}

	scanner := bufio.NewScanner(dsResp.Body)
	// Buffer up to 10MB to avoid bufio.ErrTooLong on massive SSE payloads
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

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

			if str, ok := val.(string); ok {
				fragType := "RESPONSE"
				if len(fragments) > 0 {
					fragType = fragments[len(fragments)-1].Type
				}
				appendFragmentDelta(fragType, str)
			}
		}
	}

	stopReasonToolUse := false

	if hasTools {
		if modelCfg.ThinkingEnabled && thinkingStarted && !thinkingStopped {
			ensureThinkingStopped()
		}

		fullText := responseBuffer.String()
		beforeText, toolName, toolArgs, found := extractToolCallFromText(fullText)

		currentIndex := textIndex

		if found {
			if beforeText != "" {
				s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
					"type":          "content_block_start",
					"index":         currentIndex,
					"content_block": map[string]string{"type": "text", "text": ""},
				})
				s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
					"type":  "content_block_delta",
					"index": currentIndex,
					"delta": map[string]string{
						"type": "text_delta",
						"text": beforeText + "\n",
					},
				})
				s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
					"type":  "content_block_stop",
					"index": currentIndex,
				})
				currentIndex++
			}

			toolID := fmt.Sprintf("toolu_%d", time.Now().UnixNano())

			s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": currentIndex,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    toolID,
					"name":  toolName,
					"input": map[string]interface{}{},
				},
			})

			argsBytes, _ := json.Marshal(toolArgs)
			s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": currentIndex,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": string(argsBytes),
				},
			})

			s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": currentIndex,
			})

			stopReasonToolUse = true
		}

		if !stopReasonToolUse && fullText != "" {
			s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
				"type":          "content_block_start",
				"index":         currentIndex,
				"content_block": map[string]string{"type": "text", "text": ""},
			})
			s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": currentIndex,
				"delta": map[string]string{
					"type": "text_delta",
					"text": fullText,
				},
			})
			s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": currentIndex,
			})
		}
	}

	if finalResponseMsgID != nil {
		s.sessionManager.UpdateParentMessageID(claudeSessionID, finalResponseMsgID)
	}

	if !hasTools {
		if anthropicThinkingEnabled && thinkingStarted && !thinkingStopped {
			ensureThinkingStopped()
		}
		if textStarted && !textStopped {
			ensureTextStopped()
		} else if !textStarted {
			ensureTextStarted()
			ensureTextStopped()
		}
	}

	stopReason := "end_turn"
	if stopReasonToolUse {
		stopReason = "tool_use"
	}
	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": stopReason, "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 100},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{"type": "message_stop"})
}

func (s *Server) handleOpenAIChatCompletions(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "OpenAI endpoint compatibility active", http.StatusOK)
}

func stringifyValue(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if str := stringifyValue(item); str != "" {
				parts = append(parts, str)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		if text, ok := v["text"].(string); ok && text != "" {
			return text
		}
		if content, ok := v["content"]; ok {
			return stringifyValue(content)
		}
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return ""
	default:
		if b, err := json.Marshal(v); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", v)
	}
}

func extractContentBlockGeneric(m map[string]interface{}) string {
	var sb strings.Builder
	blockType, _ := m["type"].(string)

	if text, ok := m["text"].(string); ok && text != "" {
		sb.WriteString(text)
		sb.WriteString("\n")
	}

	switch blockType {
	case "tool_result":
		toolID, _ := m["tool_use_id"].(string)
		if contentVal, exists := m["content"]; exists {
			extracted := stringifyValue(contentVal)
			if extracted != "" {
				if toolID != "" {
					sb.WriteString(fmt.Sprintf("\n[Tool Result (%s)]:\n", toolID))
				} else {
					sb.WriteString("\n[Tool Result]:\n")
				}
				sb.WriteString(extracted)
				sb.WriteString("\n")
			}
		}
	case "tool_use":
		name, _ := m["name"].(string)
		inputVal := m["input"]
		inputStr := stringifyValue(inputVal)
		sb.WriteString(fmt.Sprintf("\n[Assistant Tool Call: %s]\n%s\n", name, inputStr))
	}

	return sb.String()
}

func (s *Server) extractPromptText(messages []AnthropicMessage, system interface{}, tools []AnthropicTool, isFirstTurn bool) string {
	var sb strings.Builder

	if isFirstTurn && system != nil {
		sysText := stringifyValue(system)
		if sysText != "" {
			sb.WriteString("System: ")
			sb.WriteString(sysText)
			sb.WriteString("\n\n")
		}
	}

	if isFirstTurn && len(tools) > 0 {
		sb.WriteString("[Available Tools]\n")
		sb.WriteString("You have access to the following tools. To use a tool, you MUST output a JSON block wrapped in <tool_call> and </tool_call> tags. Example:\n")
		sb.WriteString("<tool_call>\n{\"name\": \"ToolName\", \"arguments\": {\"arg1\": \"value1\"}}\n</tool_call>\n")
		sb.WriteString("Do not output multiple tool calls in one response. Your tool call should be the last thing in your response.\n\n")
		for _, t := range tools {
			sb.WriteString("- ")
			sb.WriteString(t.Name)
			if t.Description != "" {
				sb.WriteString(": ")
				sb.WriteString(t.Description)
			}
			if t.InputSchema != nil {
				if schemaStr := stringifyValue(t.InputSchema); schemaStr != "" && schemaStr != "{}" {
					sb.WriteString(" (Schema: ")
					sb.WriteString(schemaStr)
					sb.WriteString(")")
				}
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if isFirstTurn && len(messages) > 1 {
		sb.WriteString("[Transferred Conversation History]\n")
		for i := 0; i < len(messages)-1; i++ {
			msg := messages[i]
			roleTitle := "User"
			if msg.Role == "assistant" {
				roleTitle = "Assistant"
			}
			var contentStr string
			switch c := msg.Content.(type) {
			case string:
				contentStr = c
			case []interface{}:
				var parts []string
				for _, item := range c {
					if m, ok := item.(map[string]interface{}); ok {
						parts = append(parts, extractContentBlockGeneric(m))
					} else if str, ok := item.(string); ok {
						parts = append(parts, str)
					}
				}
				contentStr = strings.Join(parts, "\n")
			}
			if strings.TrimSpace(contentStr) != "" {
				sb.WriteString(fmt.Sprintf("[%s]: %s\n\n", roleTitle, strings.TrimSpace(contentStr)))
			}
		}
		sb.WriteString("[Current User Message]\n")
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
						sb.WriteString(extractContentBlockGeneric(m))
					} else if str, ok := item.(string); ok {
						sb.WriteString(str)
						sb.WriteString("\n")
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
	b, err := json.Marshal(data)
	if err != nil {
		return
	}
	w.Write([]byte("event: "))
	w.Write([]byte(eventName))
	w.Write([]byte("\ndata: "))
	w.Write(b)
	w.Write([]byte("\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

func isTitleRequest(userPrompt string, system interface{}) bool {
	sysStr := stringifyValue(system)
	combined := strings.ToLower(userPrompt + " " + sysStr)
	return (strings.Contains(combined, "generate a concise") && strings.Contains(combined, "title")) ||
		strings.Contains(combined, "return json with a single \"title\" field") ||
		strings.Contains(combined, "return json with a single 'title' field")
}

func isSuggestionRequest(userPrompt string) bool {
	return strings.Contains(userPrompt, "[SUGGESTION MODE:")
}

func isUndoRequest(userPrompt string, messages []AnthropicMessage, prevCount int) bool {
	promptLower := strings.ToLower(userPrompt)

	if strings.Contains(promptLower, "/undo") ||
		strings.Contains(promptLower, "undo the last") ||
		strings.Contains(promptLower, "revert the last") ||
		strings.Contains(promptLower, "rewind to") {
		return true
	}

	if prevCount > 0 && len(messages) > 0 && len(messages) < prevCount {
		return true
	}

	return false
}

func isCompactRequest(userPrompt string, system interface{}, messages []AnthropicMessage, prevCount int) bool {
	promptLower := strings.ToLower(userPrompt)

	if strings.Contains(promptLower, "/compact") ||
		strings.Contains(promptLower, "compact the conversation") ||
		strings.Contains(promptLower, "summarize the conversation") ||
		strings.Contains(promptLower, "summary of the conversation") ||
		strings.Contains(promptLower, "conversation summary") ||
		strings.Contains(promptLower, "compress the context") ||
		strings.Contains(promptLower, "here is a summary of the conversation") {
		return true
	}

	return false
}

func (s *Server) sendSyntheticTitleResponse(w http.ResponseWriter, flusher http.Flusher, stream bool) {
	titleJSON := `{"title": "Claude Coding Session"}`

	if !stream || flusher == nil {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []map[string]string{{"type": "text", "text": titleJSON}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 10},
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.sendSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []interface{}{},
			"stop_reason": nil,
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 10},
		},
	})

	s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})

	s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]string{"type": "text_delta", "text": titleJSON},
	})

	s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})

	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 10},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func (s *Server) sendSyntheticEmptyResponse(w http.ResponseWriter, flusher http.Flusher, stream bool) {
	if !stream || flusher == nil {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []map[string]string{},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 0},
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.sendSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []interface{}{},
			"stop_reason": nil,
			"usage":       map[string]int{"input_tokens": 10, "output_tokens": 0},
		},
	})

	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 0},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func isInitialTestRequest(userPrompt string) bool {
	p := strings.TrimSpace(strings.ToLower(userPrompt))
	if idx := strings.LastIndex(p, "\n"); idx != -1 {
		p = strings.TrimSpace(p[idx+1:])
	}
	return p == "test" || p == "test." || p == "test!"
}

func (s *Server) sendSyntheticTestResponse(w http.ResponseWriter, flusher http.Flusher, stream bool) {
	testText := "test"

	if !stream || flusher == nil {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []map[string]string{{"type": "text", "text": testText}},
			"stop_reason": "end_turn",
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 1},
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	s.sendSSE(w, flusher, "message_start", map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":          generateMsgID(),
			"type":        "message",
			"role":        "assistant",
			"model":       "DeepSeek Pro",
			"content":     []interface{}{},
			"stop_reason": nil,
			"usage":       map[string]int{"input_tokens": 1, "output_tokens": 1},
		},
	})

	s.sendSSE(w, flusher, "content_block_start", map[string]interface{}{
		"type":          "content_block_start",
		"index":         0,
		"content_block": map[string]string{"type": "text", "text": ""},
	})

	s.sendSSE(w, flusher, "content_block_delta", map[string]interface{}{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]string{"type": "text_delta", "text": testText},
	})

	s.sendSSE(w, flusher, "content_block_stop", map[string]interface{}{
		"type":  "content_block_stop",
		"index": 0,
	})

	s.sendSSE(w, flusher, "message_delta", map[string]interface{}{
		"type":  "message_delta",
		"delta": map[string]interface{}{"stop_reason": "end_turn", "stop_sequence": nil},
		"usage": map[string]int{"output_tokens": 1},
	})

	s.sendSSE(w, flusher, "message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

func extractToolCallFromText(fullText string) (string, string, map[string]interface{}, bool) {
	// 1. Check for <tool_call...> ... </tool_call>
	startTagIdx := strings.Index(fullText, "<tool_call")
	endIdx := strings.Index(fullText, "</tool_call>")
	if startTagIdx != -1 && endIdx != -1 && endIdx > startTagIdx {
		before := strings.TrimSpace(fullText[:startTagIdx])

		tagCloseIdx := strings.Index(fullText[startTagIdx:], ">")
		if tagCloseIdx != -1 {
			tagHeader := fullText[startTagIdx : startTagIdx+tagCloseIdx]
			jsonStr := strings.TrimSpace(fullText[startTagIdx+tagCloseIdx+1 : endIdx])

			var toolName string
			if nameIdx := strings.Index(tagHeader, "name=\""); nameIdx != -1 {
				nameEnd := strings.Index(tagHeader[nameIdx+6:], "\"")
				if nameEnd != -1 {
					toolName = tagHeader[nameIdx+6 : nameIdx+6+nameEnd]
				}
			}

			var args map[string]interface{}
			var tc struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
				Input     map[string]interface{} `json:"input"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil {
				if tc.Name != "" {
					toolName = tc.Name
				}
				args = tc.Arguments
				if args == nil {
					args = tc.Input
				}
				if args == nil {
					_ = json.Unmarshal([]byte(jsonStr), &args)
				}
			} else {
				_ = json.Unmarshal([]byte(jsonStr), &args)
			}

			if toolName != "" && args != nil {
				return before, toolName, args, true
			}
		}
	}

	// 2. Check for ```json ... ``` with {"name": ...}
	codeBlockStart := strings.Index(fullText, "```json")
	if codeBlockStart != -1 {
		codeBlockEnd := strings.Index(fullText[codeBlockStart+7:], "```")
		if codeBlockEnd != -1 {
			jsonStr := strings.TrimSpace(fullText[codeBlockStart+7 : codeBlockStart+7+codeBlockEnd])
			var tc struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
				Input     map[string]interface{} `json:"input"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil && tc.Name != "" {
				before := strings.TrimSpace(fullText[:codeBlockStart])
				args := tc.Arguments
				if args == nil {
					args = tc.Input
				}
				if args == nil {
					args = make(map[string]interface{})
				}
				return before, tc.Name, args, true
			}
		}
	}

	// 3. Check for {"name": ...} in text
	jsonStart := strings.Index(fullText, "{\"name\":")
	if jsonStart == -1 {
		jsonStart = strings.Index(fullText, "{\n  \"name\":")
	}
	if jsonStart != -1 {
		jsonEnd := strings.LastIndex(fullText, "}")
		if jsonEnd > jsonStart {
			jsonStr := fullText[jsonStart : jsonEnd+1]
			var tc struct {
				Name      string                 `json:"name"`
				Arguments map[string]interface{} `json:"arguments"`
				Input     map[string]interface{} `json:"input"`
			}
			if err := json.Unmarshal([]byte(jsonStr), &tc); err == nil && tc.Name != "" {
				before := strings.TrimSpace(fullText[:jsonStart])
				args := tc.Arguments
				if args == nil {
					args = tc.Input
				}
				if args == nil {
					args = make(map[string]interface{})
				}
				return before, tc.Name, args, true
			}
		}
	}

	return "", "", nil, false
}
