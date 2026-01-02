package ipdb

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yl2chen/cidranger"
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

	chinaIPDataDir = "pkg/ipdb"
	chinaIPFile    = "china_ip_data.json"
	chinaRanger    atomic.Value
	chinaCIDRsList []string
	chinaCIDRsOnce sync.Once
	chinaCIDRsErr  error
)

//go:embed china_ip_data.json
var embeddedData []byte

func getEmbeddedChinaIPRanges() (string, error) {
	if len(embeddedData) == 0 {
		return "", fmt.Errorf("no embedded China IP data found, run 'linko update-cn-ip' to generate")
	}
	return string(embeddedData), nil
}

func isReservedIP(cidr string) bool {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}
	for _, reserved := range reservedCIDRs {
		_, reservedNet, err := net.ParseCIDR(reserved)
		if err != nil {
			continue
		}
		if reservedNet.Contains(ipNet.IP) || ipNet.Contains(reservedNet.IP) {
			return true
		}
	}
	return false
}

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
		if isReservedIP(cidr) {
			continue
		}
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
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

func LoadChinaIPRanges() error {
	chinaCIDRsOnce.Do(func() {
		chinaCIDRsErr = loadChinaIPRangesFromEmbed()
	})
	return chinaCIDRsErr
}

func loadChinaIPRangesFromEmbed() error {
	rangesJSON, err := getEmbeddedChinaIPRanges()
	if err != nil {
		return fmt.Errorf("failed to get embedded China IP ranges: %w", err)
	}

	var ranges []string
	if err := json.Unmarshal([]byte(rangesJSON), &ranges); err != nil {
		return fmt.Errorf("failed to parse embedded China IP ranges: %w", err)
	}

	ranger := cidranger.NewPCTrieRanger()
	for _, cidr := range ranges {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		ranger.Insert(cidranger.NewBasicRangerEntry(*ipNet))
	}

	chinaRanger.Store(ranger)
	chinaCIDRsList = ranges

	return nil
}

func GetChinaCIDRs() ([]string, error) {
	chinaCIDRsOnce.Do(func() {
		chinaCIDRsErr = loadChinaIPRangesFromEmbed()
	})
	if chinaCIDRsErr != nil {
		return nil, chinaCIDRsErr
	}

	return chinaCIDRsList, nil
}

func GetReservedCIDRs() []string {
	return reservedCIDRs
}

func IsChinaIP(ipStr string) bool {
	GetChinaCIDRs()
	ranger := chinaRanger.Load().(cidranger.Ranger)
	if ranger == nil {
		return false
	}

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	contains, err := ranger.Contains(ip)
	if err != nil {
		return false
	}
	return contains
}
