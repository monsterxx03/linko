package mitm

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SiteCertManager handles dynamic site certificate generation and caching
type SiteCertManager struct {
	caCert      *x509.Certificate
	caKey       crypto.Signer
	cacheDir    string
	cache       *CertCache
	validity    time.Duration
	validityMut sync.RWMutex
}

// CertCache stores cached certificates in memory
type CertCache struct {
	mu     sync.RWMutex
	certs  map[string]*CachedCert
}

// CachedCert represents a cached certificate with its key
type CachedCert struct {
	Cert      *tls.Certificate
	ExpiresAt time.Time
}

// NewSiteCertManager creates a new site certificate manager
func NewSiteCertManager(caCert *x509.Certificate, caKey crypto.Signer, cacheDir string, validity time.Duration) (*SiteCertManager, error) {
	// Ensure cache directory exists
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	scm := &SiteCertManager{
		caCert:   caCert,
		caKey:    caKey,
		cacheDir: cacheDir,
		cache: &CertCache{
			certs: make(map[string]*CachedCert),
		},
		validity: validity,
	}

	return scm, nil
}

// SetValidity sets the certificate validity duration
func (scm *SiteCertManager) SetValidity(d time.Duration) {
	scm.validityMut.Lock()
	defer scm.validityMut.Unlock()
	scm.validity = d
}

// GetValidity gets the certificate validity duration
func (scm *SiteCertManager) GetValidity() time.Duration {
	scm.validityMut.RLock()
	defer scm.validityMut.RUnlock()
	return scm.validity
}

// GetCertificate obtains a certificate for the given hostname
func (scm *SiteCertManager) GetCertificate(hostname string) (*tls.Certificate, error) {
	// Normalize hostname
	hostname = strings.ToLower(hostname)

	// Check memory cache first
	cert := scm.getFromCache(hostname)
	if cert != nil {
		return cert, nil
	}

	// Check disk cache
	cert, err := scm.loadFromDisk(hostname)
	if err == nil && cert != nil {
		// Add to memory cache
		scm.addToCache(hostname, cert)
		return cert, nil
	}

	// Generate new certificate
	cert, err = scm.generateCertificate(hostname)
	if err != nil {
		return nil, err
	}

	// Save to disk
	if err := scm.saveToDisk(hostname, cert); err != nil {
		// Log error but don't fail - we have the cert in memory
		fmt.Printf("Warning: failed to save certificate to disk: %v\n", err)
	}

	// Add to memory cache
	scm.addToCache(hostname, cert)

	return cert, nil
}

// getFromCache retrieves a certificate from memory cache
func (scm *SiteCertManager) getFromCache(hostname string) *tls.Certificate {
	scm.cache.mu.RLock()
	defer scm.cache.mu.RUnlock()

	cached, ok := scm.cache.certs[hostname]
	if !ok {
		return nil
	}

	if time.Now().After(cached.ExpiresAt) {
		return nil
	}

	return cached.Cert
}

// addToCache adds a certificate to memory cache
func (scm *SiteCertManager) addToCache(hostname string, cert *tls.Certificate) {
	scm.cache.mu.Lock()
	defer scm.cache.mu.Unlock()

	scm.cache.certs[hostname] = &CachedCert{
		Cert:      cert,
		ExpiresAt: time.Now().Add(scm.validity),
	}
}

// loadFromDisk loads a certificate from disk cache
func (scm *SiteCertManager) loadFromDisk(hostname string) (*tls.Certificate, error) {
	certPath := filepath.Join(scm.cacheDir, hostname+".crt")
	keyPath := filepath.Join(scm.cacheDir, hostname+".key")

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	// Check if certificate is expired
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, err
	}

	if time.Now().After(x509Cert.NotAfter) {
		// Certificate expired, delete it
		os.Remove(certPath)
		os.Remove(keyPath)
		return nil, nil
	}

	return &cert, nil
}

// saveToDisk saves a certificate to disk cache
func (scm *SiteCertManager) saveToDisk(hostname string, cert *tls.Certificate) error {
	certPath := filepath.Join(scm.cacheDir, hostname+".crt")
	keyPath := filepath.Join(scm.cacheDir, hostname+".key")

	// Save certificate
	certFile, err := os.Create(certPath)
	if err != nil {
		return err
	}
	defer certFile.Close()

	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]}); err != nil {
		return err
	}

	// Save private key
	keyFile, err := os.Create(keyPath)
	if err != nil {
		return err
	}
	defer keyFile.Close()

	keyBytes, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return err
	}

	if err := pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	return nil
}

// generateCertificate creates a new certificate for the given hostname
func (scm *SiteCertManager) generateCertificate(hostname string) (*tls.Certificate, error) {
	// Check if CA key is properly set
	if scm.caKey == nil {
		return nil, fmt.Errorf("CA key is nil")
	}
	if scm.caCert == nil {
		return nil, fmt.Errorf("CA certificate is nil")
	}

	// Generate ECDSA P-256 private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			Organization: []string{"Linko MITM"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(scm.validity),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{hostname},
	}

	// Handle wildcard certificates
	if strings.HasPrefix(hostname, "*.") {
		base := strings.TrimPrefix(hostname, "*.")
		template.DNSNames = []string{hostname, base}
	}

	// Create certificate signed by CA
	// Parameters: (rand, template, parent, publicKey, signer)
	// - template: the certificate to create
	// - parent: the CA certificate that signs this cert
	// - publicKey: the public key for the new certificate (from privateKey)
	// - signer: the CA's private key to sign with
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, scm.caCert, privateKey.Public(), scm.caKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	return &tls.Certificate{
		Certificate: [][]byte{derBytes, scm.caCert.Raw},
		PrivateKey:  privateKey,
		Leaf:        nil, // Will be parsed on first use
	}, nil
}

// ClearCache clears the in-memory certificate cache
func (scm *SiteCertManager) ClearCache() {
	scm.cache.mu.Lock()
	defer scm.cache.mu.Unlock()
	scm.cache.certs = make(map[string]*CachedCert)
}

// ClearDiskCache clears all cached certificates from disk
func (scm *SiteCertManager) ClearDiskCache() error {
	entries, err := os.ReadDir(scm.cacheDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".crt") {
			hostname := strings.TrimSuffix(entry.Name(), ".crt")
			certPath := filepath.Join(scm.cacheDir, entry.Name())
			keyPath := filepath.Join(scm.cacheDir, hostname+".key")

			os.Remove(certPath)
			os.Remove(keyPath)
		}
	}

	return nil
}
