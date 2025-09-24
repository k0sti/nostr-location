package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cobra"
)

type BTCMapPlace struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Icon         string  `json:"icon"`
	Address      string  `json:"address"`
	Phone        string  `json:"phone"`
	Website      string  `json:"website"`
	Email        string  `json:"email"`
	Twitter      string  `json:"twitter"`
	OpeningHours string  `json:"opening_hours"`
	VerifiedAt   string  `json:"verified_at"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

const (
	btcmapAPIURL = "https://api.btcmap.org/v4/places"
	btcmapFields = "id,lat,lon,name,icon,address,phone,website,email,twitter,opening_hours,verified_at,created_at,updated_at"
)

var btcmapCmd = &cobra.Command{
	Use:   "btcmap",
	Short: "Fetch BTCMap merchant locations and broadcast via Nostr",
	Long: `Fetches Bitcoin merchant location data from the BTCMap API
and broadcasts them as public Nostr location events (kind 30472).`,
	RunE: runBTCMap,
}

func init() {
	rootCmd.AddCommand(btcmapCmd)
	btcmapCmd.Flags().StringP("sender", "s", "", "Sender private key (nsec... or @identity)")
	btcmapCmd.Flags().Int("limit", 0, "Number of places to fetch (0 = all)")
	btcmapCmd.Flags().Int("precision", 6, "Geohash precision (1-12 characters)")
	btcmapCmd.Flags().Int("ttl", 3600, "Event time-to-live in seconds")

	btcmapCmd.MarkFlagRequired("sender")
}

func runBTCMap(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	config, err := validateBTCMapConfig()
	if err != nil {
		return err
	}

	log.Printf("Fetching BTCMap merchant locations...")
	if config.limit > 0 {
		log.Printf("Limit: %d places", config.limit)
	} else {
		log.Printf("Limit: all places")
	}
	log.Printf("Relay: %s", config.relayURL)
	log.Printf("Mode: Public broadcast (kind 30472)")

	places, err := fetchBTCMapPlaces(config.limit)
	if err != nil {
		return fmt.Errorf("failed to fetch BTCMap data: %w", err)
	}

	log.Printf("Fetched %d places from BTCMap", len(places))

	for i, place := range places {
		if err := processBTCMapPlace(config, place); err != nil {
			log.Printf("Error processing place %d (ID: %d): %v", i+1, place.ID, err)
		} else {
			log.Printf("Processed place %d/%d: %s", i+1, len(places), place.Name)
		}

		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("Completed broadcasting %d BTCMap locations", len(places))
	return nil
}

type btcmapConfig struct {
	senderSK  string
	relayURL  string
	limit     int
	precision int
	ttl       int
}

func validateBTCMapConfig() (*btcmapConfig, error) {
	sender := k.String("sender")
	if sender == "" {
		return nil, fmt.Errorf("sender is required (--sender or -s)")
	}


	relayURL := k.String("relay")
	if relayURL == "" {
		return nil, fmt.Errorf("relay URL is required (--relay)")
	}

	if !strings.HasPrefix(sender, "nsec1") {
		return nil, fmt.Errorf("sender must be an nsec private key (starting with 'nsec1') or @identity reference")
	}

	_, senderSK, err := nip19.Decode(sender)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sender nsec: %w", err)
	}


	limit := k.Int("limit")

	precision := k.Int("precision")
	if precision < 1 || precision > 12 {
		precision = 6
	}

	ttl := k.Int("ttl")
	if ttl <= 0 {
		ttl = 3600
	}

	return &btcmapConfig{
		senderSK:  senderSK.(string),
		relayURL:  relayURL,
		limit:     limit,
		precision: precision,
		ttl:       ttl,
	}, nil
}

func fetchBTCMapPlaces(limit int) ([]BTCMapPlace, error) {
	url := fmt.Sprintf("%s?fields=%s", btcmapAPIURL, btcmapFields)
	if limit > 0 {
		url = fmt.Sprintf("%s&limit=%d", url, limit)
	}

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch BTCMap data: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("BTCMap API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var places []BTCMapPlace
	if err := json.Unmarshal(body, &places); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return places, nil
}

func processBTCMapPlace(config *btcmapConfig, place BTCMapPlace) error {
	event, err := createBTCMapLocationEvent(config, place)
	if err != nil {
		return fmt.Errorf("failed to create event: %w", err)
	}

	if err := publishToRelay(config.relayURL, event); err != nil {
		return fmt.Errorf("failed to publish: %w", err)
	}

	return nil
}

func createBTCMapLocationEvent(config *btcmapConfig, place BTCMapPlace) (*nostr.Event, error) {
	gh := geohash.EncodeWithPrecision(place.Lat, place.Lon, uint(config.precision))

	expiration := time.Now().Add(time.Duration(config.ttl) * time.Second).Unix()

	// Build tags for public location event (kind 30472)
	tags := nostr.Tags{
		{"g", gh},
		{"d", fmt.Sprintf("btcmap-%d", place.ID)},
		{"expiration", fmt.Sprintf("%d", expiration)},
		{"title", place.Name},
	}

	// Add optional tags
	if place.Icon != "" {
		tags = append(tags, nostr.Tag{"icon", place.Icon})
	}
	if place.Address != "" {
		tags = append(tags, nostr.Tag{"location", place.Address})
	}
	if place.Phone != "" {
		tags = append(tags, nostr.Tag{"phone", place.Phone})
	}
	if place.Website != "" {
		tags = append(tags, nostr.Tag{"website", place.Website})
	}
	if place.Email != "" {
		tags = append(tags, nostr.Tag{"email", place.Email})
	}
	if place.Twitter != "" {
		tags = append(tags, nostr.Tag{"twitter", place.Twitter})
	}
	if place.OpeningHours != "" {
		tags = append(tags, nostr.Tag{"opening_hours", place.OpeningHours})
	}
	if place.VerifiedAt != "" {
		tags = append(tags, nostr.Tag{"verified_at", place.VerifiedAt})
	}
	if place.CreatedAt != "" {
		tags = append(tags, nostr.Tag{"created_at", place.CreatedAt})
	}
	if place.UpdatedAt != "" {
		tags = append(tags, nostr.Tag{"updated_at", place.UpdatedAt})
	}

	// Add BTCMap specific tags
	tags = append(tags, nostr.Tag{"t", "btcmap"})
	tags = append(tags, nostr.Tag{"t", "bitcoin"})
	tags = append(tags, nostr.Tag{"t", "merchant"})

	senderPubkey, err := nostr.GetPublicKey(config.senderSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender public key: %w", err)
	}

	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30472,
		Tags:      tags,
		Content:   "", // No content for public location events
	}

	if err := event.Sign(config.senderSK); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	return event, nil
}