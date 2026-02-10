package mitm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// LLMInspector parses LLM API traffic and publishes structured events
type LLMInspector struct {
	*BaseInspector
	logger          *slog.Logger
	eventBus        *EventBus
	httpProc        *HTTPProcessor
	requestPaths    sync.Map // requestID -> string (path)
	conversationIDs sync.Map // requestID -> string (conversationID)
	streamMsgIDs    sync.Map // requestID -> string (assistant message ID for streaming)
}

type conversationState struct {
	ConversationID string
	Model          string
	MessageCount   int
	TotalTokens    int
	StartTime      time.Time
}

// NewLLMInspector creates a new LLMInspector
func NewLLMInspector(logger *slog.Logger, eventBus *EventBus, hostname string) *LLMInspector {
	return &LLMInspector{
		BaseInspector: NewBaseInspector("llm_inspector", hostname),
		logger:        logger,
		eventBus:      eventBus,
		httpProc:      NewHTTPProcessor(logger, 0),
	}
}

// Name returns the inspector name
func (l *LLMInspector) Name() string {
	return "llm_inspector"
}

// Inspect processes LLM API traffic
func (l *LLMInspector) Inspect(direction Direction, data []byte, hostname string, connectionID, requestID string) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	if direction == DirectionClientToServer {
		return l.inspectRequest(data, requestID)
	}
	return l.inspectResponse(data, hostname, requestID)
}

// inspectRequest processes client-to-server (request) traffic
func (l *LLMInspector) inspectRequest(inputData []byte, requestID string) ([]byte, error) {
	_, httpMsg, complete, err := l.httpProc.ProcessRequest(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if complete {
		l.processCompleteRequest(httpMsg, requestID)
		l.httpProc.ClearPending(requestID)
	}

	return inputData, nil
}

func (l *LLMInspector) processCompleteRequest(httpMsg *HTTPMessage, requestID string) {
	// 保存路径信息到缓存
	l.requestPaths.Store(requestID, httpMsg.Path)

	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return
	}

	// Try to find a provider for this request
	provider := FindProvider(httpMsg.Hostname, httpMsg.Path, bodyBytes)
	if provider == nil {
		return
	}

	// Extract conversation ID
	conversationID := provider.ExtractConversationID(bodyBytes)
	// 缓存 conversationID，用于响应处理时匹配
	l.conversationIDs.Store(requestID, conversationID)
	model := l.extractModel(bodyBytes)

	// Publish system prompt if present (merge all prompts into one message)
	systemPrompts := provider.ExtractSystemPrompt(bodyBytes)
	if len(systemPrompts) > 0 {
		event := &LLMMessageEvent{
			ID:             generateEventID(),
			Timestamp:      time.Now(),
			ConversationID: conversationID,
			RequestID:      requestID,
			Message: LLMMessage{
				Role:    "system",
				Content: systemPrompts,
			},
		}
		l.publishEvent("llm_message", event)
	}

	// Parse the request
	messages, err := provider.ParseRequest(bodyBytes)
	if err != nil {
		l.logger.Debug("failed to parse LLM request", "error", err)
		return
	}

	if len(messages) == 0 {
		return
	}

	// 只发布最后一条消息（当前用户消息），避免重复发布历史消息
	lastMsg := messages[len(messages)-1]
	event := &LLMMessageEvent{
		ID:             generateEventID(),
		Timestamp:      time.Now(),
		ConversationID: conversationID,
		RequestID:      requestID,
		Message:        lastMsg,
	}

	l.publishEvent("llm_message", event)

	// Publish conversation update (1 = only the new message)
	l.publishConversationUpdate(conversationID, "streaming", 1, 0, model)

	l.logger.Debug("LLM request inspected",
		"conversation_id", conversationID,
		"message_count", len(messages),
		"request_id", requestID,
	)
}

// inspectResponse processes server-to-client (response) traffic
func (l *LLMInspector) inspectResponse(inputData []byte, hostname string, requestID string) ([]byte, error) {
	_, httpMsg, complete, err := l.httpProc.ProcessResponse(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if httpMsg.IsSSE {
		return l.processSSEStream(httpMsg, hostname, requestID)
	}

	if complete {
		l.processCompleteResponse(httpMsg, hostname, requestID)
		l.httpProc.ClearPending(requestID)
	}

	return inputData, nil
}

// processSSEStream processes streaming responses
func (l *LLMInspector) processSSEStream(httpMsg *HTTPMessage, hostname string, requestID string) ([]byte, error) {
	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return bodyBytes, nil
	}

	// 从缓存中获取路径信息
	path := ""
	if val, exists := l.requestPaths.Load(requestID); exists {
		path = val.(string)
	}

	// Try to find a provider using the hostname from the connection and cached path
	provider := FindProvider(hostname, path, bodyBytes)
	if provider == nil {
		return bodyBytes, nil
	}

	// Parse SSE stream tokens
	deltas := provider.ParseSSEStream(bodyBytes)
	// 从缓存中获取 conversationID（与请求时一致）
	conversationID := ""
	if val, exists := l.conversationIDs.Load(requestID); exists {
		conversationID = val.(string)
	}

	// Check if this is the first chunk for this conversation
	hasPublishedStart := false

	// Accumulate content for streaming completion
	accumulatedContent := ""

	for _, delta := range deltas {
		// 收到第一个 token 时立即更新状态为 streaming
		if !hasPublishedStart {
			// 生成并缓存消息 ID，流结束时复用同一个 ID
			msgID := generateEventID()
			l.streamMsgIDs.Store(requestID, msgID)

			// 发布初始的 assistant 消息（空内容），让前端能正确追加 token
			l.publishEvent("llm_message", &LLMMessageEvent{
				ID:             msgID,
				Timestamp:      time.Now(),
				ConversationID: conversationID,
				RequestID:      requestID,
				Message: LLMMessage{
					Role:    "assistant",
					Content: []string{""},
				},
			})
			l.publishConversationUpdate(conversationID, "streaming", 0, 0, "")
			hasPublishedStart = true
		}

		// Accumulate content
		accumulatedContent += delta.Text

		event := &LLMTokenEvent{
			ID:             generateEventID(),
			Timestamp:      time.Now(),
			ConversationID: conversationID,
			RequestID:      requestID,
			Delta:          delta.Text,
			IsComplete:     delta.IsComplete,
			StopReason:     delta.StopReason,
		}

		l.publishEvent("llm_token", event)

		if delta.IsComplete {
			// 获取流开始时生成的消息 ID（保持同一消息）
			var msgID string
			if val, exists := l.streamMsgIDs.Load(requestID); exists {
				msgID = val.(string)
				l.streamMsgIDs.Delete(requestID)
			} else {
				msgID = generateEventID()
			}

			// Publish message event for streaming completion (使用相同 ID，前端会更新)
			msgEvent := &LLMMessageEvent{
				ID:             msgID,
				Timestamp:      time.Now(),
				ConversationID: conversationID,
				RequestID:      requestID,
				Message: LLMMessage{
					Role:    "assistant",
					Content: []string{accumulatedContent},
				},
			}
			l.publishEvent("llm_message", msgEvent)

			l.publishConversationUpdate(conversationID, "complete", 1, estimateTokenCount(accumulatedContent), "")
		}
	}

	return bodyBytes, nil
}

// processCompleteResponse processes regular JSON responses
func (l *LLMInspector) processCompleteResponse(httpMsg *HTTPMessage, hostname string, requestID string) {
	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return
	}

	// 从缓存中获取路径信息
	path := ""
	if val, exists := l.requestPaths.Load(requestID); exists {
		path = val.(string)
		// 处理完响应后清理缓存
		l.requestPaths.Delete(requestID)
	}

	// Try to find a provider using the hostname from the connection and cached path
	provider := FindProvider(hostname, path, bodyBytes)
	if provider == nil {
		return
	}

	resp, err := provider.ParseResponse(bodyBytes)
	if err != nil {
		l.logger.Debug("failed to parse LLM response", "error", err)
		return
	}

	// 从缓存中获取 conversationID（与请求时一致）
	conversationID := ""
	if val, exists := l.conversationIDs.Load(requestID); exists {
		conversationID = val.(string)
		// 清理缓存
		l.conversationIDs.Delete(requestID)
	}

	// Create assistant message from response
	msg := LLMMessage{
		Role:    "assistant",
		Content: []string{resp.Content},
	}

	event := &LLMMessageEvent{
		ID:             generateEventID(),
		Timestamp:      time.Now(),
		ConversationID: conversationID,
		RequestID:      requestID,
		Message:        msg,
		TokenCount:     resp.Usage.OutputTokens,
		TotalTokens:    resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}

	l.publishEvent("llm_message", event)

	// Publish completion update
	l.publishConversationUpdate(conversationID, "complete", 1, resp.Usage.OutputTokens, "")

	l.logger.Debug("LLM response inspected",
		"conversation_id", conversationID,
		"content_length", len(resp.Content),
		"stop_reason", resp.StopReason,
	)
}

// publishEvent publishes an event to the event bus
func (l *LLMInspector) publishEvent(direction string, extra interface{}) {
	if l.eventBus == nil {
		return
	}

	event := &TrafficEvent{
		ID:        generateEventID(),
		Timestamp: time.Now(),
		Direction: direction,
		Extra:     extra,
	}
	l.eventBus.Publish(event)
}

// publishConversationUpdate publishes a conversation status update
func (l *LLMInspector) publishConversationUpdate(conversationID, status string, messageCount, totalTokens int, model string) {
	if l.eventBus == nil {
		return
	}

	event := &ConversationUpdateEvent{
		ID:             generateEventID(),
		Timestamp:      time.Now(),
		ConversationID: conversationID,
		Status:         status,
		MessageCount:   messageCount,
		TotalTokens:    totalTokens,
		Duration:       0,
		Model:          model,
	}

	l.publishEvent("conversation", event)
}

// extractModel attempts to extract the model name from request body
func (l *LLMInspector) extractModel(data []byte) string {
	// Try Anthropic format first
	var anthropicReq anthropicRequest
	if err := json.Unmarshal(data, &anthropicReq); err == nil && anthropicReq.Model != "" {
		return anthropicReq.Model
	}

	// Try OpenAI format
	var openaiReq openaiRequest
	if err := json.Unmarshal(data, &openaiReq); err == nil && openaiReq.Model != "" {
		return openaiReq.Model
	}

	return ""
}

// estimateTokenCount provides a rough estimate of token count
func estimateTokenCount(text string) int {
	return len(text) / 4
}

// Helper function to generate unique event IDs
func generateEventID() string {
	hash := sha256.Sum256([]byte(time.Now().Format(time.RFC3339Nano) + "-" + randomString(8)))
	return hex.EncodeToString(hash[:8])
}

// randomString generates a random string of given length
func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}
