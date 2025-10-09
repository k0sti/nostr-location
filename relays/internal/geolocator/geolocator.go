package geolocator

import (
	"compress/gzip"
	"encoding/csv"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"

	"relays/pkg/models"
)

type IPRange struct {
	Start uint32
	End   uint32
	Lat   float64
	Lon   float64
	City  string
	Country string
}

type GeoLocator struct {
	ranges []IPRange
	mu     sync.RWMutex
	loaded bool
}

func NewGeoLocator() *GeoLocator {
	return &GeoLocator{
		ranges: make([]IPRange, 0),
	}
}

func (g *GeoLocator) LoadDatabase() error {
	g.mu.Lock()
	defer g.mu.Unlock()

	if g.loaded {
		return nil
	}

	dbURL := "https://raw.githubusercontent.com/sapics/ip-location-db/refs/heads/main/dbip-city/dbip-city-ipv4-num.csv.gz"

	resp, err := http.Get(dbURL)
	if err != nil {
		return fmt.Errorf("failed to download database: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download database: status %d", resp.StatusCode)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	reader := csv.NewReader(gzReader)
	reader.FieldsPerRecord = -1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 9 {
			continue
		}

		start, err1 := strconv.ParseUint(strings.TrimSpace(record[0]), 10, 32)
		end, err2 := strconv.ParseUint(strings.TrimSpace(record[1]), 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}

		latStr := strings.TrimSpace(record[7])
		lonStr := strings.TrimSpace(record[8])

		if latStr == "" || lonStr == "" {
			continue
		}

		lat, err3 := strconv.ParseFloat(latStr, 64)
		lon, err4 := strconv.ParseFloat(lonStr, 64)
		if err3 != nil || err4 != nil {
			continue
		}

		country := ""
		city := ""
		if len(record) > 4 {
			country = strings.TrimSpace(record[4])
		}
		if len(record) > 5 {
			city = strings.TrimSpace(record[5])
		}

		g.ranges = append(g.ranges, IPRange{
			Start:   uint32(start),
			End:     uint32(end),
			Lat:     lat,
			Lon:     lon,
			City:    city,
			Country: country,
		})
	}

	sort.Slice(g.ranges, func(i, j int) bool {
		return g.ranges[i].Start < g.ranges[j].Start
	})

	g.loaded = true
	return nil
}

func (g *GeoLocator) LocateRelay(relayURL string) (*models.GeoLocation, error) {
	if !g.loaded {
		if err := g.LoadDatabase(); err != nil {
			return nil, err
		}
	}

	host, err := extractHost(relayURL)
	if err != nil {
		return nil, err
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host %s: %w", host, err)
	}

	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			location := g.lookupIP(ipv4)
			if location != nil {
				return location, nil
			}
		}
	}

	return nil, fmt.Errorf("no geolocation found for %s", host)
}

func (g *GeoLocator) lookupIP(ip net.IP) *models.GeoLocation {
	g.mu.RLock()
	defer g.mu.RUnlock()

	ipNum := ipToUint32(ip)

	left, right := 0, len(g.ranges)-1
	for left <= right {
		mid := (left + right) / 2
		r := g.ranges[mid]

		if ipNum < r.Start {
			right = mid - 1
		} else if ipNum > r.End {
			left = mid + 1
		} else {
			return &models.GeoLocation{
				Latitude:  r.Lat,
				Longitude: r.Lon,
				City:      r.City,
				Country:   r.Country,
			}
		}
	}

	return nil
}

func extractHost(relayURL string) (string, error) {
	if !strings.HasPrefix(relayURL, "ws://") && !strings.HasPrefix(relayURL, "wss://") {
		return "", fmt.Errorf("invalid relay URL scheme: %s", relayURL)
	}

	u, err := url.Parse(relayURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL: %w", err)
	}

	host := u.Host
	if strings.Contains(host, ":") {
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			return "", fmt.Errorf("failed to split host and port: %w", err)
		}
	}

	return host, nil
}

func ipToUint32(ip net.IP) uint32 {
	ip = ip.To4()
	if ip == nil {
		return 0
	}

	return uint32(ip[0])<<24 + uint32(ip[1])<<16 + uint32(ip[2])<<8 + uint32(ip[3])
}

func (g *GeoLocator) LoadFromFile(filename string) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	if strings.HasSuffix(filename, ".gz") {
		gzReader, err := gzip.NewReader(file)
		if err != nil {
			return fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	csvReader := csv.NewReader(reader)
	csvReader.FieldsPerRecord = -1

	for {
		record, err := csvReader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 9 {
			continue
		}

		start, err1 := strconv.ParseUint(strings.TrimSpace(record[0]), 10, 32)
		end, err2 := strconv.ParseUint(strings.TrimSpace(record[1]), 10, 32)
		if err1 != nil || err2 != nil {
			continue
		}

		latStr := strings.TrimSpace(record[7])
		lonStr := strings.TrimSpace(record[8])

		if latStr == "" || lonStr == "" {
			continue
		}

		lat, err3 := strconv.ParseFloat(latStr, 64)
		lon, err4 := strconv.ParseFloat(lonStr, 64)
		if err3 != nil || err4 != nil {
			continue
		}

		country := ""
		city := ""
		if len(record) > 4 {
			country = strings.TrimSpace(record[4])
		}
		if len(record) > 5 {
			city = strings.TrimSpace(record[5])
		}

		g.ranges = append(g.ranges, IPRange{
			Start:   uint32(start),
			End:     uint32(end),
			Lat:     lat,
			Lon:     lon,
			City:    city,
			Country: country,
		})
	}

	sort.Slice(g.ranges, func(i, j int) bool {
		return g.ranges[i].Start < g.ranges[j].Start
	})

	g.loaded = true
	return nil
}

func (g *GeoLocator) IsLoaded() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.loaded
}

func (g *GeoLocator) GetStats() map[string]interface{} {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return map[string]interface{}{
		"loaded":     g.loaded,
		"ranges":     len(g.ranges),
	}
}