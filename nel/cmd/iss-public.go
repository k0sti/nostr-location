package cmd

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cobra"
)

var issPublicCmd = &cobra.Command{
	Use:   "iss-public",
	Short: "Track ISS location and broadcast via public Nostr events",
	Long: `Demo command that fetches the International Space Station's current location
and broadcasts it as public Nostr events using kind 30472 (unencrypted).`,
	RunE: runISSPublic,
}

func init() {
	rootCmd.AddCommand(issPublicCmd)
	issPublicCmd.Flags().IntP("interval", "i", defaultInterval, "Update interval in seconds")
	issPublicCmd.Flags().StringP("sender", "s", "", "Sender private key (nsec... or @identity)")
	issPublicCmd.Flags().Int("accuracy", 0, "Location accuracy in meters")
	issPublicCmd.Flags().Int("precision", 0, "Geohash precision (number of characters, 1-12)")

	issPublicCmd.MarkFlagRequired("sender")
}

func runISSPublic(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	// Validate configuration
	config, err := validateISSPublicConfig()
	if err != nil {
		return err
	}

	log.Printf("Starting ISS public location tracker...")
	log.Printf("Mode: Public broadcast (kind 30472)")
	log.Printf("Update interval: %d seconds", config.interval)
	log.Printf("Relay: %s", config.relayURL)

	// Main tracking loop
	for {
		processISSPublicUpdate(config)
		time.Sleep(time.Duration(config.interval) * time.Second)
	}
	return nil
}

type issPublicConfig struct {
	senderSK   string
	relayURL   string
	interval   int
	accuracy_m int
	precision  int
}

func validateISSPublicConfig() (*issPublicConfig, error) {
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
		interval = k.Int("update.interval")
		if interval == 0 {
			interval = defaultInterval
		}
	}

	accuracy_m := k.Int("accuracy")
	precision := k.Int("precision")

	// Validate precision if provided
	if precision != 0 && (precision < 1 || precision > 12) {
		return nil, fmt.Errorf("precision must be between 1 and 12 characters")
	}

	return &issPublicConfig{
		senderSK:   senderSK.(string),
		relayURL:   relayURL,
		interval:   interval,
		accuracy_m: accuracy_m,
		precision:  precision,
	}, nil
}

func processISSPublicUpdate(config *issPublicConfig) {
	position, err := fetchISSLocation(issAPIURL)
	if err != nil {
		log.Printf("Error fetching ISS location: %v", err)
		return
	}

	log.Printf("ISS Position: Lat=%s, Lon=%s",
		position.ISSPosition.Latitude,
		position.ISSPosition.Longitude)

	ttl := 2 * config.interval
	event, err := createPublicLocationEvent(config.senderSK, position, ttl, config.accuracy_m, config.precision)
	if err != nil {
		log.Printf("Error creating public location event: %v", err)
		return
	}

	if err := publishToRelay(config.relayURL, event); err != nil {
		log.Printf("Error publishing to relay: %v", err)
	} else {
		log.Printf("Successfully published public location event (ID: %s)", event.ID)
	}
}

func createPublicLocationEvent(senderSK string, position *ISSPosition, ttl int, accuracy_m int, precision int) (*nostr.Event, error) {
	// Parse coordinates
	lat, err := strconv.ParseFloat(position.ISSPosition.Latitude, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(position.ISSPosition.Longitude, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse longitude: %w", err)
	}

	// Generate geohash with specified precision or default
	var gh string
	if precision > 0 {
		gh = geohash.EncodeWithPrecision(lat, lon, uint(precision))
	} else {
		gh = geohash.Encode(lat, lon)
	}

	// Get sender public key
	senderPubkey, err := nostr.GetPublicKey(senderSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender public key: %w", err)
	}

	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	// Build tags for public event (kind 30472)
	// According to NostrLocation.md spec: g tag is required, others are optional
	tags := nostr.Tags{
		{"g", gh},                             // Required geohash
		{"d", issLocationID},                  // Identifier for addressable event
		{"expiration", fmt.Sprintf("%d", expiration)}, // Expiration time
		{"title", "ISS"},                      // Optional: name of location
		{"summary", "International Space Station current position"}, // Optional: description
	}

	// Add accuracy tag if specified
	if accuracy_m > 0 {
		tags = append(tags, nostr.Tag{"accuracy", strconv.Itoa(accuracy_m)})
	}

	// Create public location event (kind 30472)
	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30472, // Public location event kind
		Tags:      tags,
		Content:   "",    // No content field for public events
	}

	if err := event.Sign(senderSK); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	return event, nil
}