package cmd

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cobra"
)

var randomCmd = &cobra.Command{
	Use:   "random",
	Short: "Send randomly moving public location events",
	Long: `Generates and broadcasts random location movements as public Nostr events.
Creates multiple concurrent moving objects, each with its own location that
moves independently in small increments to simulate movement patterns.`,
	RunE: runRandom,
}

func init() {
	rootCmd.AddCommand(randomCmd)
	randomCmd.Flags().IntP("count", "c", 3, "Number of concurrent moving objects")
	randomCmd.Flags().IntP("interval", "i", defaultInterval, "Update interval in seconds")
	randomCmd.Flags().StringP("sender", "s", "", "Sender private key (nsec... or @identity)")
	randomCmd.Flags().Int("accuracy", 0, "Location accuracy in meters")
	randomCmd.Flags().Int("precision", 0, "Geohash precision (number of characters, 1-12)")
	randomCmd.Flags().String("identifier", "walker", "Base identifier for addressable events (will be suffixed with number)")

	randomCmd.MarkFlagRequired("sender")
}

type walker struct {
	index int    // Walker index for d-tag (0-based)
	name  string // Display name
	lat   float64
	lon   float64
}

func runRandom(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	// Validate configuration
	config, err := validateRandomConfig()
	if err != nil {
		return err
	}

	log.Printf("Starting random location broadcaster...")
	log.Printf("Mode: Public broadcast (kind 30472)")
	log.Printf("Concurrent walkers: %d", config.count)
	log.Printf("Update interval: %d seconds", config.interval)
	log.Printf("Relay: %s", config.relayURL)
	log.Printf("Base identifier: %s", config.identifier)

	// Create walkers with random starting positions
	walkers := make([]walker, config.count)
	for i := 0; i < config.count; i++ {
		walkers[i] = walker{
			index: i, // Use index as d-tag (0-based)
			name:  fmt.Sprintf("%s-%d", config.identifier, i+1),
			lat:   rand.Float64()*180 - 90,  // -90 to 90
			lon:   rand.Float64()*360 - 180, // -180 to 180
		}
		log.Printf("Walker #%d (%s) starting at: Lat=%.6f, Lon=%.6f",
			walkers[i].index, walkers[i].name, walkers[i].lat, walkers[i].lon)
	}

	// Main update loop - runs indefinitely
	iteration := 0
	for {
		iteration++
		log.Printf("\n--- Iteration %d ---", iteration)

		// Update all walkers
		for i := range walkers {
			// Move the walker randomly
			walkers[i].lat, walkers[i].lon = moveRandomly(walkers[i].lat, walkers[i].lon)

			// Send the location event
			processWalkerUpdate(config, &walkers[i], iteration)
		}

		time.Sleep(time.Duration(config.interval) * time.Second)
	}
}

type randomConfig struct {
	senderSK   string
	relayURL   string
	interval   int
	count      int
	accuracy_m int
	precision  int
	identifier string
}

func validateRandomConfig() (*randomConfig, error) {
	sender := k.String("sender")
	if sender == "" {
		return nil, fmt.Errorf("sender is required (--sender or -s)")
	}

	relayURL := k.String("relay")
	if relayURL == "" {
		return nil, fmt.Errorf("relay URL is required (--relay)")
	}

	// Validate sender format (should be nsec after resolution)
	if !strings.HasPrefix(sender, "nsec1") {
		return nil, fmt.Errorf("sender must be an nsec private key (starting with 'nsec1') or @identity reference")
	}

	_, senderSK, err := nip19.Decode(sender)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sender nsec: %w", err)
	}

	interval := k.Int("interval")
	if interval == 0 {
		interval = defaultInterval
	}

	count := k.Int("count")
	if count <= 0 {
		return nil, fmt.Errorf("count must be positive (number of concurrent walkers)")
	}

	accuracy_m := k.Int("accuracy")
	precision := k.Int("precision")

	// Validate precision if provided
	if precision != 0 && (precision < 1 || precision > 12) {
		return nil, fmt.Errorf("precision must be between 1 and 12 characters")
	}

	identifier := k.String("identifier")
	if identifier == "" {
		identifier = "walker"
	}

	return &randomConfig{
		senderSK:   senderSK.(string),
		relayURL:   relayURL,
		interval:   interval,
		count:      count,
		accuracy_m: accuracy_m,
		precision:  precision,
		identifier: identifier,
	}, nil
}

func moveRandomly(lat, lon float64) (float64, float64) {
	// Random walk with small steps
	// Move up to 0.1 degrees in each direction (approximately 11km at equator)
	maxStep := 0.1

	latDelta := (rand.Float64() - 0.5) * 2 * maxStep
	lonDelta := (rand.Float64() - 0.5) * 2 * maxStep

	// Update position
	newLat := lat + latDelta
	newLon := lon + lonDelta

	// Wrap around longitude if needed
	if newLon > 180 {
		newLon -= 360
	} else if newLon < -180 {
		newLon += 360
	}

	// Clamp latitude to valid range
	newLat = math.Max(-90, math.Min(90, newLat))

	return newLat, newLon
}

func processWalkerUpdate(config *randomConfig, w *walker, iteration int) {
	log.Printf("  Walker #%d (%s): Lat=%.6f, Lon=%.6f", w.index, w.name, w.lat, w.lon)

	ttl := 2 * config.interval
	event, err := createWalkerLocationEvent(config, w, ttl, iteration)
	if err != nil {
		log.Printf("  Error creating location event for walker #%d: %v", w.index, err)
		return
	}

	if err := publishToRelay(config.relayURL, event); err != nil {
		log.Printf("  Error publishing event for walker #%d: %v", w.index, err)
	} else {
		log.Printf("  Successfully published event for walker #%d (ID: %s)", w.index, event.ID)
	}
}

func createWalkerLocationEvent(config *randomConfig, w *walker, ttl int, iteration int) (*nostr.Event, error) {
	// Generate geohash with specified precision or default
	var gh string
	if config.precision > 0 {
		gh = geohash.EncodeWithPrecision(w.lat, w.lon, uint(config.precision))
	} else {
		gh = geohash.Encode(w.lat, w.lon)
	}

	// Get sender public key
	senderPubkey, err := nostr.GetPublicKey(config.senderSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender public key: %w", err)
	}

	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	// Build tags for public event (kind 30472)
	// Use walker index as d-tag so events replace each other
	tags := nostr.Tags{
		{"g", gh},                                      // Required geohash
		{"d", strconv.Itoa(w.index)},                  // Use index as d-tag (0-based)
		{"expiration", fmt.Sprintf("%d", expiration)}, // Expiration time
		{"title", w.name},                              // Walker name
		{"summary", fmt.Sprintf("Iteration %d: %.6f, %.6f", iteration, w.lat, w.lon)}, // Location summary
	}

	// Add accuracy tag if specified
	if config.accuracy_m > 0 {
		tags = append(tags, nostr.Tag{"accuracy", strconv.Itoa(config.accuracy_m)})
	}

	// Add hashtags for discoverability
	tags = append(tags, nostr.Tag{"t", "random"})
	tags = append(tags, nostr.Tag{"t", "test"})
	tags = append(tags, nostr.Tag{"t", "location"})

	// Create public location event (kind 30472)
	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30472, // Public location event kind
		Tags:      tags,
		Content:   "",    // No content field for public events
	}

	if err := event.Sign(config.senderSK); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	return event, nil
}