package mitm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"strings"
	"time"
)

// LLMInspector parses LLM API traffic and publishes structured events
type LLMInspector struct {
	*BaseInspector
	logger   *slog.Logger
	eventBus *EventBus
	httpProc *HTTPProcessor
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
	return l.inspectResponse(data, requestID)
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
	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return
	}

	// Try to find a provider for this request
	provider := FindProvider(httpMsg.Hostname, httpMsg.Path, bodyBytes)
	if provider == nil {
		return
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

	// Extract conversation ID
	conversationID := provider.ExtractConversationID(bodyBytes)
	model := l.extractModel(bodyBytes)

	// Publish message events for each message in the request
	for _, msg := range messages {
		event := &LLMMessageEvent{
			ID:             generateEventID(),
			Timestamp:      time.Now(),
			ConversationID: conversationID,
			RequestID:      requestID,
			Message:        msg,
		}

		l.publishEvent("llm_message", event)
	}

	// Publish conversation update
	l.publishConversationUpdate(conversationID, "streaming", len(messages), 0, model)

	l.logger.Debug("LLM request inspected",
		"conversation_id", conversationID,
		"message_count", len(messages),
		"request_id", requestID,
	)
}

// inspectResponse processes server-to-client (response) traffic
func (l *LLMInspector) inspectResponse(inputData []byte, requestID string) ([]byte, error) {
	_, httpMsg, complete, err := l.httpProc.ProcessResponse(inputData, requestID)
	if err != nil || httpMsg == nil {
		return inputData, nil
	}

	if httpMsg.IsSSE {
		return l.processSSEStream(httpMsg, requestID)
	}

	if complete {
		l.processCompleteResponse(httpMsg, requestID)
		l.httpProc.ClearPending(requestID)
	}

	return inputData, nil
}

// processSSEStream processes streaming responses
func (l *LLMInspector) processSSEStream(httpMsg *HTTPMessage, requestID string) ([]byte, error) {
	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return bodyBytes, nil
	}

	// Try to find a provider
	provider := FindProvider(httpMsg.Hostname, httpMsg.Path, bodyBytes)
	if provider == nil {
		return bodyBytes, nil
	}

	// Parse SSE stream tokens
	deltas := provider.ParseSSEStream(bodyBytes)
	conversationID := l.extractConversationID(requestID)

	for _, delta := range deltas {
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
			l.publishConversationUpdate(conversationID, "complete", 1, estimateTokenCount(delta.Text), "")
		}
	}

	return bodyBytes, nil
}

// processCompleteResponse processes regular JSON responses
func (l *LLMInspector) processCompleteResponse(httpMsg *HTTPMessage, requestID string) {
	bodyBytes := httpMsg.Body
	if len(bodyBytes) == 0 {
		return
	}

	// Try to find a provider
	provider := FindProvider(httpMsg.Hostname, httpMsg.Path, bodyBytes)
	if provider == nil {
		return
	}

	resp, err := provider.ParseResponse(bodyBytes)
	if err != nil {
		l.logger.Debug("failed to parse LLM response", "error", err)
		return
	}

	conversationID := l.extractConversationID(requestID)

	// Create assistant message from response
	msg := LLMMessage{
		Role:    "assistant",
		Content: resp.Content,
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

// extractConversationID extracts conversation ID from requestID
func (l *LLMInspector) extractConversationID(requestID string) string {
	if idx := strings.LastIndex(requestID, "-"); idx > 0 {
		connID := requestID[:idx]
		return "conv-" + connID
	}
	return "conv-" + requestID
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
