package crawler

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"relays/pkg/models"
)

type Crawler struct {
	visitedRelays map[string]bool
	relayQueue    []string
	mu            sync.RWMutex
	maxDepth      int
	batchSize     int
	timeout       time.Duration
	stats         *models.DiscoveryStats
}

func NewCrawler(maxDepth, batchSize int, timeout time.Duration) *Crawler {
	return &Crawler{
		visitedRelays: make(map[string]bool),
		relayQueue:    make([]string, 0),
		maxDepth:      maxDepth,
		batchSize:     batchSize,
		timeout:       timeout,
		stats: &models.DiscoveryStats{
			StartTime: time.Now(),
		},
	}
}

func (c *Crawler) AddSeedRelay(relayURL string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.visitedRelays[relayURL] {
		c.relayQueue = append(c.relayQueue, relayURL)
		c.visitedRelays[relayURL] = true
	}
}

func (c *Crawler) DiscoverRelays(ctx context.Context) ([]string, error) {
	var functioningRelays []string
	depth := 0

	for depth < c.maxDepth && len(c.relayQueue) > 0 {
		log.Printf("Starting depth %d with %d relays to process", depth, len(c.relayQueue))

		currentBatch := c.getBatch()
		if len(currentBatch) == 0 {
			break
		}

		batchResults := c.processBatch(ctx, currentBatch)

		for _, result := range batchResults {
			if result.IsAlive {
				functioningRelays = append(functioningRelays, result.URL)
				c.stats.FunctioningRelays++

				newRelays := c.extractRelaysFromEvents(result.Events)
				c.addNewRelays(newRelays)
			}
		}

		c.stats.TotalRelaysFound += len(currentBatch)
		depth++
	}

	c.stats.Duration = time.Since(c.stats.StartTime)
	return functioningRelays, nil
}

func (c *Crawler) getBatch() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	batchSize := c.batchSize
	if len(c.relayQueue) < batchSize {
		batchSize = len(c.relayQueue)
	}

	batch := make([]string, batchSize)
	copy(batch, c.relayQueue[:batchSize])
	c.relayQueue = c.relayQueue[batchSize:]

	return batch
}

type RelayTestResult struct {
	URL     string
	IsAlive bool
	Events  []models.NostrEvent
	Error   error
}

func (c *Crawler) processBatch(ctx context.Context, batch []string) []RelayTestResult {
	results := make([]RelayTestResult, len(batch))
	var wg sync.WaitGroup

	for i, relayURL := range batch {
		wg.Add(1)
		go func(idx int, url string) {
			defer wg.Done()
			results[idx] = c.testRelay(ctx, url)
		}(i, relayURL)
	}

	wg.Wait()
	return results
}

func (c *Crawler) testRelay(ctx context.Context, relayURL string) RelayTestResult {
	result := RelayTestResult{URL: relayURL}

	timeoutCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	conn, _, err := websocket.DefaultDialer.DialContext(timeoutCtx, relayURL, nil)
	if err != nil {
		result.Error = err
		return result
	}
	defer conn.Close()

	conn.SetReadDeadline(time.Now().Add(c.timeout))
	conn.SetWriteDeadline(time.Now().Add(c.timeout))

	subID := generateRandomID()
	filter := models.NostrFilter{
		Kinds: []int{3, 10002},
		Limit: intPtr(100),
	}

	reqMsg := []interface{}{"REQ", subID, filter}
	reqJSON, err := json.Marshal(reqMsg)
	if err != nil {
		result.Error = err
		return result
	}

	if err := conn.WriteMessage(websocket.TextMessage, reqJSON); err != nil {
		result.Error = err
		return result
	}

	events := make([]models.NostrEvent, 0)
	hasEOSE := false

	for {
		select {
		case <-timeoutCtx.Done():
			result.Error = timeoutCtx.Err()
			return result
		default:
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			result.Error = err
			return result
		}

		var msgArray []json.RawMessage
		if err := json.Unmarshal(message, &msgArray); err != nil {
			continue
		}

		if len(msgArray) < 2 {
			continue
		}

		var msgType string
		if err := json.Unmarshal(msgArray[0], &msgType); err != nil {
			continue
		}

		switch msgType {
		case "EVENT":
			if len(msgArray) >= 3 {
				var event models.NostrEvent
				if err := json.Unmarshal(msgArray[2], &event); err == nil {
					events = append(events, event)
					c.stats.EventsProcessed++
				}
			}
		case "EOSE":
			hasEOSE = true
		case "NOTICE":
			log.Printf("Notice from %s: %s", relayURL, string(msgArray[1]))
		}

		if hasEOSE {
			break
		}
	}

	result.IsAlive = hasEOSE
	result.Events = events
	return result
}

func (c *Crawler) extractRelaysFromEvents(events []models.NostrEvent) []string {
	relaySet := make(map[string]bool)

	for _, event := range events {
		for _, tag := range event.Tags {
			if len(tag) >= 2 {
				switch tag[0] {
				case "r":
					if url := normalizeRelayURL(tag[1]); url != "" {
						relaySet[url] = true
					}
				case "p":
					if len(tag) >= 3 {
						if url := normalizeRelayURL(tag[2]); url != "" {
							relaySet[url] = true
						}
					}
				}
			}
		}
	}

	relays := make([]string, 0, len(relaySet))
	for relay := range relaySet {
		relays = append(relays, relay)
	}

	return relays
}

func (c *Crawler) addNewRelays(newRelays []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, relay := range newRelays {
		if !c.visitedRelays[relay] {
			c.relayQueue = append(c.relayQueue, relay)
			c.visitedRelays[relay] = true
		}
	}
}

func (c *Crawler) GetStats() *models.DiscoveryStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := *c.stats
	stats.Duration = time.Since(c.stats.StartTime)
	return &stats
}

func normalizeRelayURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	rawURL = strings.TrimSpace(rawURL)

	if !strings.HasPrefix(rawURL, "ws://") && !strings.HasPrefix(rawURL, "wss://") {
		if strings.Contains(rawURL, "localhost") || strings.Contains(rawURL, "127.0.0.1") {
			rawURL = "ws://" + rawURL
		} else {
			rawURL = "wss://" + rawURL
		}
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}

	if u.Scheme != "ws" && u.Scheme != "wss" {
		return ""
	}

	if u.Host == "" {
		return ""
	}

	if isValidDomain(u.Host) || isValidIP(u.Host) {
		return u.String()
	}

	return ""
}

func isValidDomain(host string) bool {
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	domainRegex := regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$`)
	return domainRegex.MatchString(host)
}

func isValidIP(host string) bool {
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	ipRegex := regexp.MustCompile(`^(?:[0-9]{1,3}\.){3}[0-9]{1,3}$`)
	return ipRegex.MatchString(host)
}

func generateRandomID() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 32)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

func intPtr(i int) *int {
	return &i
}