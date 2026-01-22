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
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func generateTestCA(t *testing.T) (*x509.Certificate, crypto.Signer) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA private key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test CA"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, privateKey.Public(), privateKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	return cert, privateKey
}

func TestNewSiteCertManager(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	if scm == nil {
		t.Fatal("SiteCertManager is nil")
	}

	if scm.caCert != caCert {
		t.Error("CA certificate not set correctly")
	}

	if scm.caKey != caKey {
		t.Error("CA key not set correctly")
	}

	if scm.cacheDir != tmpDir {
		t.Error("cache directory not set correctly")
	}

	if scm.validity != time.Hour {
		t.Error("validity not set correctly")
	}
}

func TestNewSiteCertManager_MkdirFail(t *testing.T) {
	caCert, caKey := generateTestCA(t)

	_, err := NewSiteCertManager(caCert, caKey, "/nonexistent/path", time.Hour)
	if err == nil {
		t.Error("expected error when creating cache directory fails")
	}
}

func TestGetCertificate_CacheHit(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "test.example.com"

	cert1, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	if cert1 == nil {
		t.Fatal("certificate is nil")
	}

	cert2, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate from cache: %v", err)
	}

	if cert2 == nil {
		t.Fatal("cached certificate is nil")
	}

	if !certEqual(cert1, cert2) {
		t.Error("cached certificate does not match original")
	}
}

func TestGetCertificate_DiskCacheHit(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "diskcache.example.com"

	cert1, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	scm.ClearCache()

	cert2, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate from disk: %v", err)
	}

	if cert2 == nil {
		t.Fatal("disk cached certificate is nil")
	}

	if !certBytesEqual(cert1.Certificate, cert2.Certificate) {
		t.Error("disk cached certificate does not match original")
	}
}

func TestGetCertificate_NewCertificate(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "new.example.com"

	cert, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	if cert == nil {
		t.Fatal("certificate is nil")
	}

	if len(cert.Certificate) != 2 {
		t.Errorf("expected 2 certificates in chain, got %d", len(cert.Certificate))
	}

	if cert.PrivateKey == nil {
		t.Error("private key is nil")
	}
}

func TestGetCertificate_HostnameNormalization(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	cert1, err := scm.GetCertificate("Test.Example.COM")
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	cert2, err := scm.GetCertificate("test.example.com")
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	if !certEqual(cert1, cert2) {
		t.Error("hostname normalization failed - certificates should be the same")
	}
}

func TestGetCertificate_Wildcard(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "*.example.com"

	cert, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get wildcard certificate: %v", err)
	}

	if cert == nil {
		t.Fatal("wildcard certificate is nil")
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	if len(x509Cert.DNSNames) != 2 {
		t.Errorf("expected 2 DNS names in wildcard cert, got %d", len(x509Cert.DNSNames))
	}
}

func TestGetCertificate_InvalidCA(t *testing.T) {
	tmpDir := t.TempDir()

	scm := &SiteCertManager{
		caCert:   nil,
		caKey:    nil,
		cacheDir: tmpDir,
		cache: &CertCache{
			certs: make(map[string]*CachedCert),
		},
		validity: time.Hour,
	}

	_, err := scm.GetCertificate("test.example.com")
	if err == nil {
		t.Error("expected error when CA is nil")
	}
}

func TestGetFromCache(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	result := scm.getFromCache("nonexistent.example.com")
	if result != nil {
		t.Error("expected nil for nonexistent certificate")
	}
}

func TestGetFromCache_Expired(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	cert, _ := scm.generateCertificate("test.example.com")
	scm.addToCache("test.example.com", cert)

	scm.cache.mu.Lock()
	scm.cache.certs["test.example.com"].ExpiresAt = time.Now().Add(-time.Hour)
	scm.cache.mu.Unlock()

	result := scm.getFromCache("test.example.com")
	if result != nil {
		t.Error("expected nil for expired certificate")
	}

	scm.cache.mu.Lock()
	if _, exists := scm.cache.certs["test.example.com"]; exists {
		t.Error("expired certificate should be removed from cache")
	}
	scm.cache.mu.Unlock()
}

func TestAddToCache(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	cert, _ := scm.generateCertificate("test.example.com")

	scm.addToCache("cached.example.com", cert)

	result := scm.getFromCache("cached.example.com")
	if result == nil {
		t.Fatal("expected cached certificate, got nil")
	}

	if !certEqual(cert, result) {
		t.Error("cached certificate does not match original")
	}
}

func TestLoadFromDisk(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "loadtest.example.com"
	cert, _ := scm.generateCertificate(hostname)

	err = scm.saveToDisk(hostname, cert)
	if err != nil {
		t.Fatalf("failed to save certificate: %v", err)
	}

	loadedCert, err := scm.loadFromDisk(hostname)
	if err != nil {
		t.Fatalf("failed to load certificate: %v", err)
	}

	if loadedCert == nil {
		t.Fatal("loaded certificate is nil")
	}

	if !certBytesEqual(cert.Certificate, loadedCert.Certificate) {
		t.Error("loaded certificate does not match original")
	}
}

func TestLoadFromDisk_Nonexistent(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	result, err := scm.loadFromDisk("nonexistent.example.com")
	if err == nil {
		t.Error("expected error for nonexistent certificate")
	}
	if result != nil {
		t.Error("expected nil for nonexistent certificate")
	}
}

func TestLoadFromDisk_Expired(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, -2*time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "expired.example.com"

	cert, _ := scm.generateCertificate(hostname)
	certPath := filepath.Join(tmpDir, hostname+".crt")
	keyPath := filepath.Join(tmpDir, hostname+".key")

	certFile, _ := os.Create(certPath)
	pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Certificate[0]})
	certFile.Close()

	keyBytes, _ := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	keyFile, _ := os.Create(keyPath)
	pem.Encode(keyFile, &pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	keyFile.Close()

	result, err := scm.loadFromDisk(hostname)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for expired certificate")
	}

	if _, err := os.Stat(certPath); err == nil {
		t.Error("expired certificate file should be deleted")
	}
	if _, err := os.Stat(keyPath); err == nil {
		t.Error("expired key file should be deleted")
	}
}

func TestSaveToDisk(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "savetest.example.com"
	cert, _ := scm.generateCertificate(hostname)

	err = scm.saveToDisk(hostname, cert)
	if err != nil {
		t.Fatalf("failed to save certificate: %v", err)
	}

	certPath := filepath.Join(tmpDir, hostname+".crt")
	keyPath := filepath.Join(tmpDir, hostname+".key")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Error("certificate file not created")
	}
	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		t.Error("key file not created")
	}

	loadedCert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatalf("failed to load saved certificate: %v", err)
	}

	if len(loadedCert.Certificate) == 0 {
		t.Error("loaded certificate has no certificates")
	}

	if string(loadedCert.Certificate[0]) != string(cert.Certificate[0]) {
		t.Error("loaded certificate bytes do not match original")
	}
}

func TestSaveToDisk_OrphanCleanup(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	cert, _ := scm.generateCertificate("cleanup.example.com")

	keyDir := filepath.Join(tmpDir, "cleanup.example.com.key")
	os.MkdirAll(keyDir, 0755)

	err = scm.saveToDisk("cleanup.example.com", cert)
	if err == nil {
		t.Error("expected error when key file creation fails")
	}

	certPath := filepath.Join(tmpDir, "cleanup.example.com.crt")
	if _, err := os.Stat(certPath); err == nil {
		t.Error("orphaned certificate file should be cleaned up")
	}
}

func TestGenerateCertificate(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "gen.example.com"

	cert, err := scm.generateCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to generate certificate: %v", err)
	}

	if cert == nil {
		t.Fatal("generated certificate is nil")
	}

	if len(cert.Certificate) != 2 {
		t.Errorf("expected 2 certificates in chain, got %d", len(cert.Certificate))
	}

	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("failed to parse generated certificate: %v", err)
	}

	if x509Cert.NotAfter.Sub(time.Now()) > time.Hour+time.Second {
		t.Error("certificate validity period exceeds configured validity")
	}

	if len(x509Cert.DNSNames) != 1 || x509Cert.DNSNames[0] != hostname {
		t.Errorf("certificate has unexpected DNS names: %v", x509Cert.DNSNames)
	}
}

func TestGenerateCertificate_NilCA(t *testing.T) {
	tmpDir := t.TempDir()

	scm := &SiteCertManager{
		caCert:   nil,
		caKey:    nil,
		cacheDir: tmpDir,
		cache: &CertCache{
			certs: make(map[string]*CachedCert),
		},
		validity: time.Hour,
	}

	_, err := scm.generateCertificate("test.example.com")
	if err == nil {
		t.Error("expected error when CA is nil")
	}
}

func TestClearCache(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	cert, _ := scm.generateCertificate("test.example.com")
	scm.addToCache("cache.example.com", cert)

	if scm.getFromCache("cache.example.com") == nil {
		t.Fatal("certificate should be in cache")
	}

	scm.ClearCache()

	if scm.getFromCache("cache.example.com") != nil {
		t.Error("certificate should be cleared from cache")
	}
}

func TestClearDiskCache(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "diskcache.example.com"
	cert, _ := scm.generateCertificate(hostname)

	err = scm.saveToDisk(hostname, cert)
	if err != nil {
		t.Fatalf("failed to save certificate: %v", err)
	}

	certPath := filepath.Join(tmpDir, hostname+".crt")
	keyPath := filepath.Join(tmpDir, hostname+".key")

	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		t.Fatal("certificate file should exist")
	}

	err = scm.ClearDiskCache()
	if err != nil {
		t.Fatalf("failed to clear disk cache: %v", err)
	}

	if _, err := os.Stat(certPath); err == nil {
		t.Error("certificate file should be deleted")
	}
	if _, err := os.Stat(keyPath); err == nil {
		t.Error("key file should be deleted")
	}
}

func TestClearDiskCache_Nonexistent(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	err = scm.ClearDiskCache()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConcurrentGetCertificate(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, time.Hour)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "concurrent.example.com"
	numGoroutines := 10

	var wg sync.WaitGroup
	certChan := make(chan *tls.Certificate, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cert, err := scm.GetCertificate(hostname)
			if err != nil {
				t.Errorf("GetCertificate failed: %v", err)
				return
			}
			certChan <- cert
		}()
	}

	certs := make([]*tls.Certificate, 0, numGoroutines)
	for cert := range certChan {
		certs = append(certs, cert)
		if len(certs) == numGoroutines {
			break
		}
	}

	for i := 1; i < len(certs); i++ {
		if !certEqual(certs[0], certs[i]) {
			t.Error("concurrent certificates do not match - possible race condition")
		}
	}
}

func TestCacheExpiry(t *testing.T) {
	caCert, caKey := generateTestCA(t)
	tmpDir := t.TempDir()

	validity := 100 * time.Millisecond
	scm, err := NewSiteCertManager(caCert, caKey, tmpDir, validity)
	if err != nil {
		t.Fatalf("failed to create SiteCertManager: %v", err)
	}

	hostname := "expiry.example.com"

	cert1, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate: %v", err)
	}

	time.Sleep(validity + 50*time.Millisecond)

	cert2, err := scm.GetCertificate(hostname)
	if err != nil {
		t.Fatalf("failed to get certificate after expiry: %v", err)
	}

	if certEqual(cert1, cert2) {
		t.Error("certificate should be regenerated after expiry")
	}
}

func certEqual(a, b *tls.Certificate) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Certificate) != len(b.Certificate) {
		return false
	}
	for i := range a.Certificate {
		if string(a.Certificate[i]) != string(b.Certificate[i]) {
			return false
		}
	}
	return true
}

func certBytesEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if string(a[i]) != string(b[i]) {
			return false
		}
	}
	return true
}
