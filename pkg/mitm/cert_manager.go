package mitm

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CertManager handles CA certificate generation and management
type CertManager struct {
	caCertPath string
	caKeyPath  string
	caCert     *x509.Certificate
	caKey      *rsa.PrivateKey
	caValidity time.Duration
}

// NewCertManager creates a new certificate manager
func NewCertManager(caCertPath, caKeyPath string, caValidity time.Duration) (*CertManager, error) {
	cm := &CertManager{
		caCertPath: caCertPath,
		caKeyPath:  caKeyPath,
		caValidity: caValidity,
	}

	if err := cm.LoadOrCreateCA(); err != nil {
		return nil, err
	}

	return cm, nil
}

// LoadOrCreateCA loads existing CA certificate or creates a new one
func (cm *CertManager) LoadOrCreateCA() error {
	// Try to load existing CA
	cert, err := tls.LoadX509KeyPair(cm.caCertPath, cm.caKeyPath)
	if err == nil {
		// Parse the certificate
		x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return fmt.Errorf("failed to parse CA certificate: %w", err)
		}

		// Parse the private key - it could be PKCS1 or PKCS8 encoded
		var key *rsa.PrivateKey

		// Try PKCS1 first
		if pkcs1Key, ok := cert.PrivateKey.(*rsa.PrivateKey); ok {
			key = pkcs1Key
		} else {
			// PrivateKey is a crypto.PrivateKey interface, need to marshal to DER
			// For RSA keys in PKCS8 format, use x509.MarshalPKCS8PrivateKey
			// But we need to get the actual key bytes
			// Since we can't easily marshal back, we'll regenerate the CA
			return fmt.Errorf("unsupported private key format, please remove old CA files and restart")
		}

		cm.caKey = key
		cm.caCert = x509Cert
		return nil
	}

	// CA doesn't exist, create new one
	return cm.CreateCA()
}

// CreateCA generates a new self-signed CA certificate
func (cm *CertManager) CreateCA() error {
	// Ensure directory exists
	caDir := filepath.Dir(cm.caCertPath)
	if err := os.MkdirAll(caDir, 0755); err != nil {
		return fmt.Errorf("failed to create CA directory: %w", err)
	}

	// Generate RSA 4096-bit private key
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return fmt.Errorf("failed to generate CA private key: %w", err)
	}
	cm.caKey = privateKey

	// Create CA certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization:  []string{"Linko MITM CA"},
			Country:       []string{"CN"},
			Province:      []string{""},
			Locality:      []string{"Beijing"},
			StreetAddress: []string{""},
			CommonName:    "Linko MITM CA",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(cm.caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	// Create self-signed certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate: %w", err)
	}

	// Save certificate
	certFile, err := os.Create(cm.caCertPath)
	if err != nil {
		return fmt.Errorf("failed to create CA certificate file: %w", err)
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); err != nil {
		return fmt.Errorf("failed to encode CA certificate: %w", err)
	}

	// Save private key
	keyFile, err := os.Create(cm.caKeyPath)
	if err != nil {
		return fmt.Errorf("failed to create CA key file: %w", err)
	}
	defer keyFile.Close()

	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	if err := pem.Encode(keyFile, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return fmt.Errorf("failed to encode CA private key: %w", err)
	}

	// Parse and store certificate
	cm.caCert, err = x509.ParseCertificate(derBytes)
	if err != nil {
		return fmt.Errorf("failed to parse created CA certificate: %w", err)
	}

	return nil
}

// GetCACertificate returns the CA certificate
func (cm *CertManager) GetCACertificate() *x509.Certificate {
	return cm.caCert
}

// GetCAPrivateKey returns the CA private key
func (cm *CertManager) GetCAPrivateKey() *rsa.PrivateKey {
	return cm.caKey
}

// GetCACertPath returns the CA certificate path
func (cm *CertManager) GetCACertPath() string {
	return cm.caCertPath
}

// GetCAKeyPath returns the CA private key path
func (cm *CertManager) GetCAKeyPath() string {
	return cm.caKeyPath
}
