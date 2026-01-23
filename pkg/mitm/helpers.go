package mitm

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"io"
	"strings"

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

func decompressBody(body string, contentEncoding string) string {
	if contentEncoding == "" {
		return body
	}

	var decompressed []byte
	var err error

	switch strings.ToLower(contentEncoding) {
	case "gzip":
		reader, err := gzip.NewReader(strings.NewReader(body))
		if err != nil {
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
		return body
	}
	return string(decompressed)
}
