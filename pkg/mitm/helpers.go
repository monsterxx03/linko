package mitm

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"errors"
	"io"
	"strings"

	"log/slog"

	"github.com/andybalholm/brotli"
)

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

func decompressBody(body string, contentEncoding string, contentType string, logger *slog.Logger) string {
	if contentEncoding == "" {
		return body
	}

	// Check if it's SSE response
	isSSE := strings.Contains(strings.ToLower(contentType), "text/event-stream")

	var decompressed []byte
	var err error

	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(strings.NewReader(body))
		if err != nil {
			logger.Warn("Failed to create gzip reader", "error", err)
			return body
		}
		defer reader.Close()
		decompressed, err = io.ReadAll(reader)
	case "deflate":
		reader := flate.NewReader(strings.NewReader(body))
		defer reader.Close()
		decompressed, err = io.ReadAll(reader)
	case "br":
		reader := brotli.NewReader(strings.NewReader(body))
		decompressed, err = io.ReadAll(reader)
	default:
		return body
	}

	if err != nil {
		// For SSE, try to return whatever we managed to decompress
		if isSSE {
			if len(decompressed) > 0 {
				// Don't warn for EOF errors on SSE
				if !errors.Is(err, io.EOF) {
					logger.Warn("Partial decompression for SSE", "contentEncoding", contentEncoding, "error", err)
				}
				return string(decompressed)
			}
			// No decompressed data, return original
			return body
		}
		// For non-SSE, return original body on error
		logger.Warn("Failed to decompress body", "contentEncoding", contentEncoding, "error", err)
		return body
	}
	return string(decompressed)
}
