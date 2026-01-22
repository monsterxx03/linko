package mitm

import (
	"errors"
	"fmt"
	"net"
)

// SNIInfo contains parsed SNI information
type SNIInfo struct {
	Hostname    string
	ServerName  string
	IsValid     bool
	ParseError  error
}

// parseSNI extracts SNI hostname from TLS ClientHello
func parseSNI(data []byte) (*SNIInfo, error) {
	if len(data) < 5 {
		return nil, errors.New("TLS record too short")
	}

	// Check TLS record header
	if data[0] != 0x16 { // Handshake record
		return nil, errors.New("not a TLS handshake record")
	}

	// Parse TLS handshake
	pos := 5 // Skip record header (1 byte type + 2 bytes version + 2 bytes length)

	if len(data) < pos+4 {
		return nil, errors.New("TLS handshake too short")
	}

	// Handshake type and length
	// Handshake msg: 1 byte type + 3 bytes length
	handshakeType := data[pos]
	if handshakeType != 0x01 { // ClientHello
		return nil, errors.New("not a ClientHello")
	}

	pos += 4 // Skip handshake header

	// Parse ClientHello
	// ProtocolVersion: 2 bytes (major + minor)
	// Random: 32 bytes
	// SessionID: 1 byte length + variable
	// CipherSuites: 2 bytes length + variable
	// CompressionMethods: 1 byte length + variable
	// Extensions: 2 bytes length + variable

	if len(data) < pos+34 {
		return nil, errors.New("ClientHello too short")
	}

	pos += 2  // Skip client_version
	pos += 32 // Skip random

	// Session ID
	sessionIDLen := int(data[pos])
	pos++
	if len(data) < pos+sessionIDLen {
		return nil, errors.New("ClientHello session ID truncated")
	}
	pos += sessionIDLen

	// Cipher suites
	if len(data) < pos+2 {
		return nil, errors.New("ClientHello cipher suites truncated")
	}
	cipherSuitesLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2
	if len(data) < pos+cipherSuitesLen {
		return nil, errors.New("ClientHello cipher suites truncated")
	}
	pos += cipherSuitesLen

	// Compression methods
	if len(data) < pos+1 {
		return nil, errors.New("ClientHello compression methods truncated")
	}
	compressionMethodsLen := int(data[pos])
	pos++
	if len(data) < pos+compressionMethodsLen {
		return nil, errors.New("ClientHello compression methods truncated")
	}
	pos += compressionMethodsLen

	// Extensions
	if len(data) < pos+2 {
		return nil, errors.New("ClientHello extensions truncated")
	}
	extensionsLen := int(data[pos])<<8 | int(data[pos+1])
	pos += 2
	extensionsEnd := pos + extensionsLen

	if len(data) < extensionsEnd {
		return nil, errors.New("ClientHello extensions truncated")
	}

	// Parse extensions looking for SNI (extension type 0x0000)
	sniInfo := &SNIInfo{}

	for pos < extensionsEnd {
		if len(data) < pos+4 {
			break
		}

		extType := int(data[pos])<<8 | int(data[pos+1])
		extLen := int(data[pos+2])<<8 | int(data[pos+3])
		pos += 4

		if extType == 0x0000 { // SNI extension
			sniData := data[pos : pos+extLen]
			hostname, err := parseSNIExtension(sniData)
			if err != nil {
				return nil, fmt.Errorf("failed to parse SNI extension: %w", err)
			}
			sniInfo.Hostname = hostname
			sniInfo.ServerName = hostname
			sniInfo.IsValid = true
			return sniInfo, nil
		}

		pos += extLen
	}

	return sniInfo, nil
}

// parseSNIExtension parses the SNI extension data
func parseSNIExtension(data []byte) (string, error) {
	if len(data) < 2 {
		return "", errors.New("SNI extension too short")
	}

	// SNI extension format:
	// 2 bytes: list length (not including this length)
	// Then list of name entries:
	//   1 byte: name type (0 = hostname)
	//   2 bytes: name length
	//   N bytes: name data

	listLen := int(data[0])<<8 | int(data[1])
	pos := 2

	for pos < 2+listLen {
		if pos+3 > len(data) {
			break
		}

		nameType := data[pos]
		nameLen := int(data[pos+1])<<8 | int(data[pos+2])
		pos += 3

		if pos+nameLen > len(data) {
			break
		}

		if nameType == 0 { // hostname
			return string(data[pos : pos+nameLen]), nil
		}

		pos += nameLen
	}

	return "", errors.New("SNI hostname not found")
}

// ExtractSNIFromConn extracts SNI from a TLS connection
func ExtractSNIFromConn(conn []byte) (*SNIInfo, error) {
	sniInfo, err := parseSNI(conn)
	if err != nil {
		return nil, err
	}

	if !sniInfo.IsValid {
		return nil, errors.New("SNI not found in ClientHello")
	}

	return sniInfo, nil
}

// ExtractSNIFromPeek performs a peek on the connection to extract SNI
func ExtractSNIFromPeek(data []byte) string {
	sniInfo, err := parseSNI(data)
	if err != nil {
		return ""
	}

	if sniInfo.IsValid {
		return sniInfo.Hostname
	}

	return ""
}

// IsValidIP checks if a string is a valid IP address
func IsValidIP(s string) bool {
	return net.ParseIP(s) != nil
}

// ExtractHostnameFromTarget extracts the hostname from the target connection info
// This is used when SNI is not available
func ExtractHostnameFromTarget(host string, port int) string {
	if IsValidIP(host) {
		return host
	}
	return host
}

// ExtractSNIFromConnReader reads from a net.Conn and extracts SNI
// This consumes data from the connection, so should only be used for checking
func ExtractSNIFromConnReader(conn net.Conn) (string, error) {
	// Read enough data for TLS ClientHello
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}

	sniInfo, err := parseSNI(buf[:n])
	if err != nil {
		return "", err
	}

	if sniInfo.IsValid && sniInfo.Hostname != "" {
		return sniInfo.Hostname, nil
	}

	return "", errors.New("SNI not found")
}
