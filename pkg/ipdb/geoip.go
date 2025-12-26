package ipdb

import (
	"fmt"
	"net"
	"sync"

	"github.com/oschwald/geoip2-golang"
)

// Location represents geographical location information
type Location struct {
	Country     string `json:"country"`
	CountryCode string `json:"country_code"`
	Region      string `json:"region"`
	City        string `json:"city"`
	IsDomestic  bool   `json:"is_domestic"`
}

// GeoIPManager manages GeoIP database operations
type GeoIPManager struct {
	db           *geoip2.Reader
	dbPath       string
	initialized  bool
	initMutex    sync.RWMutex
}

// NewGeoIPManager creates a new GeoIP manager
func NewGeoIPManager(dbPath string) (*GeoIPManager, error) {
	m := &GeoIPManager{
		dbPath: dbPath,
	}

	if err := m.initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize GeoIP manager: %w", err)
	}

	return m, nil
}

// initialize loads the GeoIP database
func (m *GeoIPManager) initialize() error {
	m.initMutex.Lock()
	defer m.initMutex.Unlock()

	// Check if already initialized
	if m.initialized {
		return nil
	}

	// Try to open the database
	db, err := geoip2.Open(m.dbPath)
	if err != nil {
		// Database not found, this is OK for initial setup
		// Will be initialized when database is downloaded
		return fmt.Errorf("GeoIP database not found at %s: %w", m.dbPath, err)
	}

	m.db = db
	m.initialized = true
	return nil
}

// UpdateDatabase updates the GeoIP database
func (m *GeoIPManager) UpdateDatabase(dbPath string) error {
	m.initMutex.Lock()
	defer m.initMutex.Unlock()

	// Close existing database
	if m.db != nil {
		m.db.Close()
	}

	// Open new database
	db, err := geoip2.Open(dbPath)
	if err != nil {
		return fmt.Errorf("failed to open new GeoIP database: %w", err)
	}

	m.db = db
	m.dbPath = dbPath
	m.initialized = true
	return nil
}

// LookupIP looks up geographical information for an IP address
func (m *GeoIPManager) LookupIP(ipStr string) (*Location, error) {
	m.initMutex.RLock()
	defer m.initMutex.RUnlock()

	if !m.initialized || m.db == nil {
		return &Location{
			IsDomestic:  false,
			CountryCode: "ZZ",
		}, nil
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	record, err := m.db.City(ip)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup IP %s: %w", ipStr, err)
	}

	location := &Location{
		Country:     record.Country.Names["en"],
		CountryCode: record.Country.IsoCode,
		Region:      record.Subdivisions[0].Names["en"],
		City:        record.City.Names["en"],
		IsDomestic:  m.isDomesticCountry(record.Country.IsoCode),
	}

	return location, nil
}

// LookupIPBytes looks up geographical information for an IP address from bytes
func (m *GeoIPManager) LookupIPBytes(ipBytes []byte) (*Location, error) {
	ipStr := net.IP(ipBytes).String()
	return m.LookupIP(ipStr)
}

// IsDomesticIP checks if an IP address is domestic (Chinese)
func (m *GeoIPManager) IsDomesticIP(ipStr string) (bool, error) {
	location, err := m.LookupIP(ipStr)
	if err != nil {
		return false, err
	}
	return location.IsDomestic, nil
}

// isDomesticCountry checks if a country code represents China
func (m *GeoIPManager) isDomesticCountry(countryCode string) bool {
	// Mainland China
	if countryCode == "CN" {
		return true
	}

	// Note: You can add more domestic regions if needed
	// e.g., Hong Kong (HK), Macau (MO), Taiwan (TW)
	// Currently only mainland China is considered domestic

	return false
}

// IsInitialized returns whether the database is initialized
func (m *GeoIPManager) IsInitialized() bool {
	m.initMutex.RLock()
	defer m.initMutex.RUnlock()
	return m.initialized && m.db != nil
}

// GetDBPath returns the path to the database file
func (m *GeoIPManager) GetDBPath() string {
	m.initMutex.RLock()
	defer m.initMutex.RUnlock()
	return m.dbPath
}

// Close closes the database
func (m *GeoIPManager) Close() error {
	m.initMutex.Lock()
	defer m.initMutex.Unlock()

	if m.db != nil {
		err := m.db.Close()
		m.db = nil
		m.initialized = false
		return err
	}

	return nil
}

// DownloadGeoIPDatabase downloads the GeoLite2 database
// This is a placeholder for the actual download logic
func DownloadGeoIPDatabase(outputPath string) error {
	// TODO: Implement download logic
	// You can use MaxMind's official API or alternative sources
	// For now, this is a placeholder

	// Example: curl -L -o outputPath \
	//   "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=YOUR_LICENSE_KEY&suffix=tar.gz"

	return fmt.Errorf("download functionality not implemented yet")
}