package proxy

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const apnicURL = "http://ftp.apnic.net/apnic/stats/apnic/delegated-apnic-latest"

var (
	reservedCIDRs = []string{
		"127.0.0.0/8",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"240.0.0.0/4",
	}

	chinaCIDRs     []string
	chinaIPDataDir = "data"
	chinaIPFile    = "china_ip_ranges.json"
)

func FetchChinaIPRanges() error {
	resp, err := http.Get(apnicURL)
	if err != nil {
		return fmt.Errorf("failed to fetch APNIC data: %w", err)
	}
	defer resp.Body.Close()

	var ranges = make([]string, 0, 1024)
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "apnic|CN|ipv4") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) < 5 {
			continue
		}
		ip := parts[3]
		count, err := strconv.Atoi(parts[4])
		if err != nil {
			continue
		}
		prefix := int(math.Log2(float64(count)))
		cidr := fmt.Sprintf("%s/%d", ip, 32-prefix)
		ranges = append(ranges, cidr)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to parse APNIC data: %w", err)
	}

	if err := os.MkdirAll(chinaIPDataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	filePath := filepath.Join(chinaIPDataDir, chinaIPFile)
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(ranges); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

func LoadChinaIPRanges() error {
	filePath := filepath.Join(chinaIPDataDir, chinaIPFile)
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open cached data: %w", err)
	}
	defer file.Close()

	var ranges []string
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&ranges); err != nil {
		return fmt.Errorf("failed to parse cached data: %w", err)
	}

	chinaCIDRs = ranges
	return nil
}

func GetChinaCIDRs() []string {
	return chinaCIDRs
}

func netmaskToPrefixLen(mask string) (int, error) {
	switch mask {
	case "255.0.0.0":
		return 8, nil
	case "255.128.0.0":
		return 9, nil
	case "255.192.0.0":
		return 10, nil
	case "255.224.0.0":
		return 11, nil
	case "255.240.0.0":
		return 12, nil
	case "255.248.0.0":
		return 13, nil
	case "255.252.0.0":
		return 14, nil
	case "255.254.0.0":
		return 15, nil
	case "255.255.0.0":
		return 16, nil
	case "255.255.128.0":
		return 17, nil
	case "255.255.192.0":
		return 18, nil
	case "255.255.224.0":
		return 19, nil
	case "255.255.240.0":
		return 20, nil
	case "255.255.248.0":
		return 21, nil
	case "255.255.252.0":
		return 22, nil
	case "255.255.254.0":
		return 23, nil
	case "255.255.255.0":
		return 24, nil
	case "255.255.255.128":
		return 25, nil
	case "255.255.255.192":
		return 26, nil
	case "255.255.255.224":
		return 27, nil
	case "255.255.255.240":
		return 28, nil
	case "255.255.255.248":
		return 29, nil
	case "255.255.255.252":
		return 30, nil
	case "255.255.255.254":
		return 31, nil
	case "255.255.255.255":
		return 32, nil
	default:
		return 0, fmt.Errorf("unknown netmask: %s", mask)
	}
}

func isReservedAddress(ip string) bool {
	for _, cidr := range reservedCIDRs {
		if ip == cidr {
			return true
		}
	}
	return false
}

type FirewallRule struct {
	Protocol string
	SrcIP    string
	SrcPort  string
	DstIP    string
	DstPort  string
	Target   string
}

// FirewallManager manages firewall rules for transparent proxy
type FirewallManager struct {
	proxyPort     string
	dnsServerPort string
	redirectDNS   bool
	redirectHTTP  bool
	redirectHTTPS bool
}

// NewFirewallManager creates a new firewall manager
func NewFirewallManager(proxyPort string, dnsServerPort string, redirectDNS, redirectHTTP, redirectHTTPS bool) *FirewallManager {
	return &FirewallManager{
		proxyPort:     proxyPort,
		dnsServerPort: dnsServerPort,
		redirectDNS:   redirectDNS,
		redirectHTTP:  redirectHTTP,
		redirectHTTPS: redirectHTTPS,
	}
}
