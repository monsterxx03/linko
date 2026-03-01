package mitm

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"slices"
	"strings"

	"log/slog"

	"github.com/andybalholm/brotli"
)

// readableAppTypes defines common text-based application MIME types
var readableAppTypes = []string{
	"application/json",
	"application/xml",
	"application/javascript",
	"application/x-www-form-urlencoded",
	"application/ld+json",
	"application/rss+xml",
	"application/atom+xml",
	"application/xhtml+xml",
	"application/svg+xml",
	"application/proto",
	"application/connect+proto",
}

func isHTTPPrefix(data []byte) bool {
	methods := []string{"GET ", "POST ", "HEAD ", "PUT ", "DELETE ", "PATCH ", "OPTIONS ", "CONNECT ", "TRACE "}
	for _, method := range methods {
		if bytes.HasPrefix(data, []byte(method)) {
			return true
		}
	}
	return false
}

// isHTTP2 detects if the data starts with an HTTP/2 frame
// HTTP/2 uses a binary framing format: 24-bit length + frame type + flags + stream identifier
func isHTTP2(data []byte) bool {
	if len(data) < 9 {
		return false
	}
	// HTTP/2 frames start with a 24-bit length (3 bytes), followed by frame type (1 byte)
	// Valid HTTP/2 frame types: 0x00 (DATA), 0x01 (HEADERS), 0x02 (PRIORITY),
	// 0x03 (RST_STREAM), 0x04 (SETTINGS), 0x05 (PUSH_PROMISE), 0x06 (PING),
	// 0x07 (GOAWAY), 0x08 (WINDOW_UPDATE), 0x09 (CONTINUATION)
	frameType := data[3]
	validFrameTypes := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}
	for _, t := range validFrameTypes {
		if frameType == t {
			// Also verify the length field is reasonable (not exceeding 16MB)
			length := uint32(data[0])<<16 | uint32(data[1])<<8 | uint32(data[2])
			return length < 0x1000000
		}
	}
	return false
}

func isHTTPResponsePrefix(data []byte) bool {
	if len(data) < 9 {
		return false
	}
	return bytes.HasPrefix(data, []byte("HTTP/1.1 ")) ||
		bytes.HasPrefix(data, []byte("HTTP/1.0 ")) ||
		bytes.HasPrefix(data, []byte("HTTP/2 "))
}

// isReadableTextType checks if the content type is a readable text type
func isReadableTextType(contentType string) bool {
	// Normalize content type by removing charset and other parameters
	contentType = strings.Split(contentType, ";")[0]
	contentType = strings.TrimSpace(strings.ToLower(contentType))

	// Text types
	if strings.HasPrefix(contentType, "text/") {
		return true
	}

	// Check if it's a common text-based application type
	return slices.Contains(readableAppTypes, contentType)
}

func decompressBody(body []byte, contentEncoding string, contentType string, logger *slog.Logger) []byte {
	if contentEncoding == "" {
		return body
	}

	// Check if it's SSE response
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	// Only decompress if it's a readable text type or SSE response
	if !isSSE && !isReadableTextType(contentType) {
		return body
	}

	var decompressed []byte
	var err error

	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			logger.Warn("Failed to create gzip reader", "error", err)
			return body
		}
		defer reader.Close()
		decompressed, err = io.ReadAll(reader)
	case "deflate":
		reader := flate.NewReader(bytes.NewReader(body))
		defer reader.Close()
		decompressed, err = io.ReadAll(reader)
	case "br":
		reader := brotli.NewReader(bytes.NewReader(body))
		decompressed, err = io.ReadAll(reader)
	default:
		return body
	}

	if err != nil {
		// For SSE, try to return whatever we managed to decompress
		if isSSE {
			if len(decompressed) > 0 {
				if !errors.Is(err, io.ErrUnexpectedEOF) {
					logger.Warn("Partial decompression for SSE", "contentEncoding", contentEncoding, "error", err)
				}
				return decompressed
			}
			// No decompressed data, return original
			return body
		}
		// For non-SSE, return original body on error
		logger.Warn("Failed to decompress body", "contentEncoding", contentEncoding, "error", err)
		return body
	}
	return decompressed
}
