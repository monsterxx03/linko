package mitm

import (
	"bufio"
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
)

type pendingHTTPRequest struct {
	data          []byte
	headers       []byte
	contentLength int64
	isComplete    bool
}

type pendingHTTPResponse struct {
	data          []byte
	headers       []byte
	contentLength int64
	isComplete    bool
	isSSE         bool
}

// HTTPProcessor provides common HTTP protocol parsing capabilities
type HTTPProcessor struct {
	logger       *slog.Logger
	pendingReqs  sync.Map // requestID -> *pendingHTTPRequest
	pendingResps sync.Map // requestID -> *pendingHTTPResponse
	maxBodySize  int64
}

// HTTPMessage represents a complete HTTP message
type HTTPMessage struct {
	Hostname    string
	Path        string
	Method      string
	Headers     map[string]string
	Body        []byte
	ContentType string
	IsResponse  bool
	StatusCode  int
	IsSSE       bool
}

// NewHTTPProcessor creates a new HTTPProcessor
func NewHTTPProcessor(logger *slog.Logger, maxBodySize int64) *HTTPProcessor {
	if maxBodySize == 0 {
		maxBodySize = 1024 * 1024 // 1MB default
	}
	return &HTTPProcessor{
		logger:      logger,
		maxBodySize: maxBodySize,
	}
}

// ProcessRequest processes incoming request data incrementally
// Returns: (completeMessage, isComplete, error)
func (p *HTTPProcessor) ProcessRequest(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if len(inputData) == 0 {
		return inputData, nil, false, nil
	}

	pending := p.loadOrCreatePendingRequest(requestID)

	// If we already have headers, just append the data
	// Otherwise, check if this is the start of a new request
	if pending.headers == nil && !isHTTPPrefix(inputData) {
		return inputData, nil, false, nil
	}

	pending.data = append(pending.data, inputData...)

	if pending.headers == nil {
		idx := bytes.Index(pending.data, []byte("\r\n\r\n"))
		if idx < 0 {
			return inputData, nil, false, nil
		}
		pending.headers = make([]byte, idx+4)
		copy(pending.headers, pending.data[:idx+4])
		pending.contentLength = p.parseContentLength(pending.headers, false)
	}

	headerLen := len(pending.headers)

	switch pending.contentLength {
	case 0:
		// No body - request is complete
		p.pendingReqs.Delete(requestID)
		msg := p.buildRequestMessage(pending.data)
		if msg == nil {
			return pending.data, nil, true, nil
		}
		return pending.data, msg, true, nil
	case -1:
		// Chunked transfer encoding
		bodyEndIdx := bytes.Index(pending.data, []byte("\r\n0\r\n\r\n"))
		if bodyEndIdx < 0 {
			return inputData, nil, false, nil
		}
		p.pendingReqs.Delete(requestID)
		msg := p.buildRequestMessage(pending.data)
		if msg == nil {
			return pending.data, nil, true, nil
		}
		return pending.data, msg, true, nil
	default:
		needed := int(pending.contentLength) + headerLen
		if len(pending.data) < needed {
			return inputData, nil, false, nil
		}
		p.pendingReqs.Delete(requestID)
		// Make a copy to ensure the returned data is independent
		fullData := make([]byte, needed)
		copy(fullData, pending.data[:needed])
		msg := p.buildRequestMessage(fullData)
		if msg == nil {
			return fullData, nil, true, nil
		}
		return fullData, msg, true, nil
	}
}

// ProcessResponse processes incoming response data incrementally
// Returns: (completeMessage, isComplete, error)
// For SSE responses, always returns accumulated data (for streaming inspection)
func (p *HTTPProcessor) ProcessResponse(inputData []byte, requestID string) ([]byte, *HTTPMessage, bool, error) {
	if len(inputData) == 0 {
		return inputData, nil, false, nil
	}

	pending := p.loadOrCreatePendingResponse(requestID)

	// If we already have headers, just append the data
	// Otherwise, check if this is the start of a new response
	if pending.headers == nil && !isHTTPResponsePrefix(inputData) {
		return inputData, nil, false, nil
	}

	pending.data = append(pending.data, inputData...)

	if pending.headers == nil {
		idx := bytes.Index(pending.data, []byte("\r\n\r\n"))
		if idx < 0 {
			return inputData, nil, false, nil
		}
		pending.headers = make([]byte, idx+4)
		copy(pending.headers, pending.data[:idx+4])
		pending.contentLength = p.parseContentLength(pending.headers, true)
		pending.isSSE = p.detectSSE(pending.headers)
	}

	headerLen := len(pending.headers)

	// For SSE responses, always return accumulated data (don't consume it)
	if pending.isSSE {
		msg := p.buildResponseMessage(pending.data)
		return pending.data, msg, false, nil
	}

	switch pending.contentLength {
	case 0:
		// No body - response is complete
		p.pendingResps.Delete(requestID)
		msg := p.buildResponseMessage(pending.data)
		if msg == nil {
			return pending.data, nil, true, nil
		}
		return pending.data, msg, true, nil
	case -1:
		// Chunked transfer encoding
		bodyEndIdx := bytes.Index(pending.data, []byte("\r\n0\r\n\r\n"))
		if bodyEndIdx < 0 {
			return inputData, nil, false, nil
		}
		p.pendingResps.Delete(requestID)
		msg := p.buildResponseMessage(pending.data)
		if msg == nil {
			return pending.data, nil, true, nil
		}
		return pending.data, msg, true, nil
	default:
		needed := int(pending.contentLength) + headerLen
		if len(pending.data) < needed {
			return inputData, nil, false, nil
		}
		p.pendingResps.Delete(requestID)
		// Make a copy to ensure the returned data is independent
		fullData := make([]byte, needed)
		copy(fullData, pending.data[:needed])
		msg := p.buildResponseMessage(fullData)
		if msg == nil {
			return fullData, nil, true, nil
		}
		return fullData, msg, true, nil
	}
}

// GetPendingMessage gets a pending request/response by requestID
func (p *HTTPProcessor) GetPendingMessage(requestID string) (*HTTPMessage, bool) {
	if val, exists := p.pendingReqs.Load(requestID); exists {
		pending := val.(*pendingHTTPRequest)
		msg := p.buildRequestMessage(pending.data)
		return msg, true
	}
	if val, exists := p.pendingResps.Load(requestID); exists {
		pending := val.(*pendingHTTPResponse)
		msg := p.buildResponseMessage(pending.data)
		return msg, true
	}
	return nil, false
}

// ClearPending clears pending state for a requestID
func (p *HTTPProcessor) ClearPending(requestID string) {
	p.pendingReqs.Delete(requestID)
	p.pendingResps.Delete(requestID)
}

func (p *HTTPProcessor) loadOrCreatePendingRequest(requestID string) *pendingHTTPRequest {
	if val, exists := p.pendingReqs.Load(requestID); exists {
		return val.(*pendingHTTPRequest)
	}
	pending := &pendingHTTPRequest{contentLength: -2}
	p.pendingReqs.Store(requestID, pending)
	return pending
}

func (p *HTTPProcessor) loadOrCreatePendingResponse(requestID string) *pendingHTTPResponse {
	if val, exists := p.pendingResps.Load(requestID); exists {
		return val.(*pendingHTTPResponse)
	}
	pending := &pendingHTTPResponse{contentLength: -2}
	p.pendingResps.Store(requestID, pending)
	return pending
}

func (p *HTTPProcessor) parseContentLength(headerData []byte, isResponse bool) int64 {
	reader := bytes.NewReader(headerData)

	if isResponse {
		resp, err := http.ReadResponse(bufio.NewReader(reader), nil)
		if err != nil {
			return 0
		}
		defer resp.Body.Close()

		if resp.Header.Get("Transfer-Encoding") == "chunked" {
			return -1
		}
		return resp.ContentLength
	}

	req, err := http.ReadRequest(bufio.NewReader(reader))
	if err != nil {
		return 0
	}
	defer req.Body.Close()

	if req.Header.Get("Transfer-Encoding") == "chunked" {
		return -1
	}
	return req.ContentLength
}

func (p *HTTPProcessor) detectSSE(headerData []byte) bool {
	reader := bytes.NewReader(headerData)
	resp, err := http.ReadResponse(bufio.NewReader(reader), nil)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream")
}

func (p *HTTPProcessor) buildRequestMessage(data []byte) *HTTPMessage {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err != nil {
		return nil
	}
	defer req.Body.Close()

	bodyBytes, _ := io.ReadAll(req.Body)

	contentType := req.Header.Get("Content-Type")
	// Only decompress readable content types, but always apply body size limit
	if isReadableTextType(contentType) {
		// Decompress if needed
		contentEncoding := getContentEncoding(req.Header)
		decompressed := decompressBody(bodyBytes, contentEncoding, contentType, p.logger)
		bodyBytes = p.truncateBody(decompressed)
	} else {
		// Apply body size limit even for non-readable types
		bodyBytes = p.truncateBody(bodyBytes)
	}

	return &HTTPMessage{
		Hostname:    req.Host,
		Path:        req.URL.Path,
		Method:      req.Method,
		Headers:     extractHeaders(req.Header),
		Body:        bodyBytes,
		ContentType: contentType,
		IsResponse:  false,
	}
}

func (p *HTTPProcessor) buildResponseMessage(data []byte) *HTTPMessage {
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)

	contentType := resp.Header.Get("Content-Type")
	// Only decompress readable content types, but always apply body size limit
	if isReadableTextType(contentType) {
		// Decompress if needed
		contentEncoding := getContentEncoding(resp.Header)
		decompressed := decompressBody(bodyBytes, contentEncoding, contentType, p.logger)
		bodyBytes = p.truncateBody(decompressed)
	} else {
		// Apply body size limit even for non-readable types
		bodyBytes = p.truncateBody(bodyBytes)
	}

	hostname := ""
	path := ""
	if resp.Request != nil && resp.Request.URL != nil {
		hostname = resp.Request.Host
		path = resp.Request.URL.Path
	}

	return &HTTPMessage{
		Hostname:    hostname,
		Path:        path,
		Headers:     extractHeaders(resp.Header),
		Body:        bodyBytes,
		ContentType: contentType,
		IsResponse:  true,
		StatusCode:  resp.StatusCode,
		IsSSE:       p.detectSSE(data[:bytes.Index(data, []byte("\r\n\r\n"))+4]),
	}
}

func (p *HTTPProcessor) truncateBody(body []byte) []byte {
	bodyStr := string(body)
	if p.maxBodySize > 0 && len(bodyStr) > int(p.maxBodySize) {
		return []byte(bodyStr[:p.maxBodySize])
	}
	return body
}

func extractHeaders(header http.Header) map[string]string {
	headers := make(map[string]string)
	for k, v := range header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}
	return headers
}

func getContentEncoding(header http.Header) string {
	if ce := header.Get("Content-Encoding"); ce != "" {
		return ce
	}
	return header.Get("Connect-Content-Encoding")
}
