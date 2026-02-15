package mitm

import (
	"errors"
	"testing"
)

// buildTLSClientHello creates a minimal TLS ClientHello with SNI extension
func buildTLSClientHello(hostname string) []byte {
	// TLS record header: 1 byte type + 2 bytes version + 2 bytes length
	recordHeader := []byte{0x16, 0x03, 0x03} // handshake, TLS 1.2
	recordLen := []byte{0x00, 0x00} // length placeholder

	// Handshake header: 1 byte type + 3 bytes length
	handshakeType := []byte{0x01} // ClientHello
	handshakeLen := []byte{0x00, 0x00, 0x00} // length placeholder

	// ClientHello body:
	// Version: 2 bytes
	// Random: 32 bytes
	// Session ID: 1 byte length + variable
	// Cipher suites: 2 bytes length + variable
	// Compression: 1 byte length + variable
	// Extensions: 2 bytes length + variable

	clientVersion := []byte{0x03, 0x03} // TLS 1.2
	random := make([]byte, 32)
	for i := range random {
		random[i] = byte(i)
	}

	// Session ID: 1 byte length (0 = empty)
	sessionIDLen := []byte{0x00}

	// Cipher suites: 2 bytes length + cipher suites
	// Need even number, so we use 4 bytes (2 cipher suites)
	cipherSuitesLen := []byte{0x00, 0x04}
	cipherSuites := []byte{0x00, 0x0a, 0x00, 0x09} // TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384, TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256

	// Compression: 1 byte length + methods
	compressionLen := []byte{0x01}
	compression := []byte{0x00} // null compression

	// Build SNI extension
	sniExt := buildSNIExtension(hostname)

	// Extensions: 2 bytes length + extensions
	extensionsLen := make([]byte, 2)
	totalExtLen := len(sniExt)
	extensionsLen[0] = byte(totalExtLen >> 8)
	extensionsLen[1] = byte(totalExtLen & 0xff)

	// Assemble ClientHello body
	clientHelloBody := []byte{}
	clientHelloBody = append(clientHelloBody, clientVersion...)
	clientHelloBody = append(clientHelloBody, random...)
	clientHelloBody = append(clientHelloBody, sessionIDLen...)
	clientHelloBody = append(clientHelloBody, cipherSuitesLen...)
	clientHelloBody = append(clientHelloBody, cipherSuites...)
	clientHelloBody = append(clientHelloBody, compressionLen...)
	clientHelloBody = append(clientHelloBody, compression...)
	clientHelloBody = append(clientHelloBody, extensionsLen...)
	clientHelloBody = append(clientHelloBody, sniExt...)

	// Set handshake length (total - 4 bytes for handshake header)
	helloLen := len(clientHelloBody)
	handshakeLen[0] = byte(helloLen >> 16)
	handshakeLen[1] = byte(helloLen >> 8)
	handshakeLen[2] = byte(helloLen & 0xff)

	// Set record length (handshake length + 4)
	recLen := helloLen + 4
	recordLen[0] = byte(recLen >> 8)
	recordLen[1] = byte(recLen & 0xff)

	// Assemble final ClientHello
	result := []byte{}
	result = append(result, recordHeader...)
	result = append(result, recordLen...)
	result = append(result, handshakeType...)
	result = append(result, handshakeLen...)
	result = append(result, clientHelloBody...)

	return result
}

// buildSNIExtension builds a SNI extension with the given hostname
func buildSNIExtension(hostname string) []byte {
	// SNI extension format:
	// 2 bytes: extension type (0x0000 for SNI)
	// 2 bytes: extension length
	// 2 bytes: list length
	// For each name:
	//   1 byte: name type (0 = hostname)
	//   2 bytes: name length
	//   N bytes: name data

	hostnameBytes := []byte(hostname)

	// Name entry: type (1) + length (2) + hostname
	nameEntry := []byte{0x00} // name type = hostname
	nameEntry = append(nameEntry, byte(len(hostnameBytes)>>8))
	nameEntry = append(nameEntry, byte(len(hostnameBytes)&0xff))
	nameEntry = append(nameEntry, hostnameBytes...)

	// SNI data: list length + name entry
	sniData := []byte{}
	sniData = append(sniData, byte(len(nameEntry)>>8))
	sniData = append(sniData, byte(len(nameEntry)&0xff))
	sniData = append(sniData, nameEntry...)

	// Extension: type + length + data
	extLen := len(sniData)
	sniExt := []byte{0x00, 0x00} // extension type SNI
	sniExt = append(sniExt, byte(extLen>>8))
	sniExt = append(sniExt, byte(extLen&0xff))
	sniExt = append(sniExt, sniData...)

	return sniExt
}

func TestParseSNI_Valid(t *testing.T) {
	hostname := "example.com"
	data := buildTLSClientHello(hostname)

	sniInfo, err := parseSNI(data)
	if err != nil {
		t.Fatalf("parseSNI failed: %v", err)
	}

	if !sniInfo.IsValid {
		t.Error("Expected IsValid to be true")
	}

	if sniInfo.Hostname != hostname {
		t.Errorf("Expected hostname %s, got %s", hostname, sniInfo.Hostname)
	}

	if sniInfo.ServerName != hostname {
		t.Errorf("Expected ServerName %s, got %s", hostname, sniInfo.ServerName)
	}
}

func TestParseSNI_EmptyHostname(t *testing.T) {
	data := buildTLSClientHello("")

	sniInfo, err := parseSNI(data)
	if err != nil {
		t.Fatalf("parseSNI failed: %v", err)
	}

	// Empty hostname is still valid from parsing perspective
	if sniInfo.Hostname != "" {
		t.Errorf("Expected empty hostname, got %s", sniInfo.Hostname)
	}
}

func TestParseSNI_InvalidData_TooShort(t *testing.T) {
	data := []byte{0x16, 0x03, 0x01}

	_, err := parseSNI(data)
	if err == nil {
		t.Error("Expected error for too short data")
	}
}

func TestParseSNI_InvalidData_NotHandshake(t *testing.T) {
	data := []byte{0x17, 0x03, 0x01, 0x00, 0x00} // Not a handshake record

	_, err := parseSNI(data)
	if err == nil {
		t.Error("Expected error for non-handshake record")
	}
}

func TestParseSNI_InvalidData_NotClientHello(t *testing.T) {
	// Build a valid record but with wrong handshake type
	data := []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x02, 0x00, 0x00, 0x01} // ServerHello type

	_, err := parseSNI(data)
	if err == nil {
		t.Error("Expected error for non-ClientHello")
	}
}

func TestParseSNI_InvalidData_ClientHelloTooShort(t *testing.T) {
	data := []byte{0x16, 0x03, 0x01, 0x00, 0x05, 0x01, 0x00, 0x00, 0x01, 0x00} // Incomplete ClientHello

	_, err := parseSNI(data)
	if err == nil {
		t.Error("Expected error for truncated ClientHello")
	}
}

func TestParseSNI_NoSNIExtension(t *testing.T) {
	// Build a ClientHello without SNI extension
	recordHeader := []byte{0x16, 0x03, 0x01, 0x00, 0x00}
	handshakeHeader := []byte{0x01, 0x00, 0x00, 0x00}

	clientVersion := []byte{0x03, 0x01}
	random := make([]byte, 32)
	sessionID := []byte{0x00}
	cipherSuites := []byte{0x00, 0x02, 0x00, 0x0a} // 1 cipher suite
	compression := []byte{0x01, 0x00}               // 1 compression method

	// Empty extensions
	extensions := []byte{0x00, 0x00}

	clientHello := append(clientVersion, random...)
	clientHello = append(clientHello, sessionID...)
	clientHello = append(clientHello, cipherSuites...)
	clientHello = append(clientHello, compression...)
	clientHello = append(clientHello, extensions...)

	helloLen := len(clientHello)
	handshakeHeader[1] = byte(helloLen >> 16)
	handshakeHeader[2] = byte(helloLen >> 8)
	handshakeHeader[3] = byte(helloLen & 0xff)

	recordLen := helloLen + 4
	recordHeader[3] = byte(recordLen >> 8)
	recordHeader[4] = byte(recordLen & 0xff)

	data := append(recordHeader, handshakeHeader...)
	data = append(data, clientHello...)

	sniInfo, err := parseSNI(data)
	if err != nil {
		t.Fatalf("parseSNI failed: %v", err)
	}

	// No SNI should result in invalid
	if sniInfo.IsValid {
		t.Error("Expected IsValid to be false when no SNI extension")
	}

	if sniInfo.Hostname != "" {
		t.Errorf("Expected empty hostname, got %s", sniInfo.Hostname)
	}
}

func TestParseSNIExtension_Valid(t *testing.T) {
	hostname := "test.example.com"

	// parseSNIExtension receives just the SNI data (after extension type and length)
	// Format: 2 bytes list length + name entries
	hostnameBytes := []byte(hostname)
	nameEntry := []byte{0x00} // name type = hostname
	nameEntry = append(nameEntry, byte(len(hostnameBytes)>>8))
	nameEntry = append(nameEntry, byte(len(hostnameBytes)&0xff))
	nameEntry = append(nameEntry, hostnameBytes...)

	extData := []byte{}
	extData = append(extData, byte(len(nameEntry)>>8))
	extData = append(extData, byte(len(nameEntry)&0xff))
	extData = append(extData, nameEntry...)

	result, err := parseSNIExtension(extData)
	if err != nil {
		t.Fatalf("parseSNIExtension failed: %v", err)
	}

	if result != hostname {
		t.Errorf("Expected hostname %s, got %s", hostname, result)
	}
}

func TestParseSNIExtension_TooShort(t *testing.T) {
	extData := []byte{0x00}

	_, err := parseSNIExtension(extData)
	if err == nil {
		t.Error("Expected error for too short SNI extension")
	}
}

func TestParseSNIExtension_EmptyList(t *testing.T) {
	extData := []byte{0x00, 0x00} // empty list

	_, err := parseSNIExtension(extData)
	if err == nil {
		t.Error("Expected error for empty SNI list")
	}
}

func TestParseSNIExtension_InvalidNameType(t *testing.T) {
	// Build SNI extension with unsupported name type
	hostname := "example.com"
	hostnameBytes := []byte(hostname)

	// Name entry with type 1 (IP address, not supported)
	nameEntry := []byte{0x01} // name type = IP
	nameEntry = append(nameEntry, byte(len(hostnameBytes)>>8))
	nameEntry = append(nameEntry, byte(len(hostnameBytes)&0xff))
	nameEntry = append(nameEntry, hostnameBytes...)

	listLen := len(nameEntry)
	sniData := []byte{byte(listLen >> 8), byte(listLen & 0xff)}
	sniData = append(sniData, nameEntry...)

	_, err := parseSNIExtension(sniData)
	if err == nil {
		t.Error("Expected error for unsupported name type")
	}
}

func TestExtractSNIFromConn_Valid(t *testing.T) {
	hostname := "api.example.com"
	data := buildTLSClientHello(hostname)

	sniInfo, err := ExtractSNIFromConn(data)
	if err != nil {
		t.Fatalf("ExtractSNIFromConn failed: %v", err)
	}

	if sniInfo.Hostname != hostname {
		t.Errorf("Expected hostname %s, got %s", hostname, sniInfo.Hostname)
	}
}

func TestExtractSNIFromConn_NoSNI(t *testing.T) {
	// Build ClientHello without SNI
	recordHeader := []byte{0x16, 0x03, 0x01, 0x00, 0x00}
	handshakeHeader := []byte{0x01, 0x00, 0x00, 0x00}

	clientVersion := []byte{0x03, 0x01}
	random := make([]byte, 32)
	sessionID := []byte{0x00}
	cipherSuites := []byte{0x00, 0x02, 0x00, 0x0a}
	compression := []byte{0x01, 0x00}
	extensions := []byte{0x00, 0x00}

	clientHello := append(clientVersion, random...)
	clientHello = append(clientHello, sessionID...)
	clientHello = append(clientHello, cipherSuites...)
	clientHello = append(clientHello, compression...)
	clientHello = append(clientHello, extensions...)

	helloLen := len(clientHello)
	handshakeHeader[1] = byte(helloLen >> 16)
	handshakeHeader[2] = byte(helloLen >> 8)
	handshakeHeader[3] = byte(helloLen & 0xff)

	recordLen := helloLen + 4
	recordHeader[3] = byte(recordLen >> 8)
	recordHeader[4] = byte(recordLen & 0xff)

	data := append(recordHeader, handshakeHeader...)
	data = append(data, clientHello...)

	_, err := ExtractSNIFromConn(data)
	if err == nil {
		t.Error("Expected error when no SNI in connection")
	}

	if !errors.Is(err, errors.New("SNI not found in ClientHello")) {
		t.Logf("Got expected error: %v", err)
	}
}

func TestExtractSNIFromConn_InvalidData(t *testing.T) {
	data := []byte{0x16} // Too short

	_, err := ExtractSNIFromConn(data)
	if err == nil {
		t.Error("Expected error for invalid data")
	}
}

func TestParseSNI_LongHostname(t *testing.T) {
	// Test with a very long hostname (subdomain max is 253 chars)
	hostname := "a.b.c.d.e.f.g.h.i.j.k.l.m.n.o.p.q.r.s.t.u.v.w.x.y.z.example.com"
	data := buildTLSClientHello(hostname)

	sniInfo, err := parseSNI(data)
	if err != nil {
		t.Fatalf("parseSNI failed: %v", err)
	}

	if sniInfo.Hostname != hostname {
		t.Errorf("Expected hostname %s, got %s", hostname, sniInfo.Hostname)
	}
}

func TestParseSNI_WithOtherExtensions(t *testing.T) {
	// Build ClientHello with SNI and other extensions
	hostname := "secure.example.com"

	recordHeader := []byte{0x16, 0x03, 0x01, 0x00, 0x00}
	handshakeHeader := []byte{0x01, 0x00, 0x00, 0x00}

	clientVersion := []byte{0x03, 0x01}
	random := make([]byte, 32)
	sessionID := []byte{0x00}
	cipherSuites := []byte{0x00, 0x02, 0x00, 0x0a}
	compression := []byte{0x01, 0x00}

	// Build multiple extensions: SNI + some other extension (e.g., supported_versions)
	sniExt := buildSNIExtension(hostname)

	// Add a dummy extension (supported_versions = 0x002b, length 2)
	dummyExt := []byte{0x00, 0x2b, 0x00, 0x02, 0x01, 0x01}

	extensions := []byte{0x00, 0x00} // placeholder
	extensions = append(extensions, sniExt...)
	extensions = append(extensions, dummyExt...)

	extLen := len(extensions) - 2
	extensions[0] = byte(extLen >> 8)
	extensions[1] = byte(extLen & 0xff)

	clientHello := append(clientVersion, random...)
	clientHello = append(clientHello, sessionID...)
	clientHello = append(clientHello, cipherSuites...)
	clientHello = append(clientHello, compression...)
	clientHello = append(clientHello, extensions...)

	helloLen := len(clientHello)
	handshakeHeader[1] = byte(helloLen >> 16)
	handshakeHeader[2] = byte(helloLen >> 8)
	handshakeHeader[3] = byte(helloLen & 0xff)

	recordLen := helloLen + 4
	recordHeader[3] = byte(recordLen >> 8)
	recordHeader[4] = byte(recordLen & 0xff)

	data := append(recordHeader, handshakeHeader...)
	data = append(data, clientHello...)

	sniInfo, err := parseSNI(data)
	if err != nil {
		t.Fatalf("parseSNI failed: %v", err)
	}

	if sniInfo.Hostname != hostname {
		t.Errorf("Expected hostname %s, got %s", hostname, sniInfo.Hostname)
	}
}
