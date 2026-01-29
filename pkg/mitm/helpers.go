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
			if len(decompressed) > 0 && !errors.Is(err, io.ErrUnexpectedEOF) {
				logger.Warn("Partial decompression for SSE", "contentEncoding", contentEncoding, "error", err)
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
