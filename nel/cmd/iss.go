package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/spf13/cobra"
)

type ISSPosition struct {
	ISSPosition struct {
		Latitude  string `json:"latitude"`
		Longitude string `json:"longitude"`
	} `json:"iss_position"`
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

const (
	issAPIURL       = "http://api.open-notify.org/iss-now.json"
	defaultInterval = 5
	issLocationID   = "8jdr" // Fixed ID for ISS updates
)

var issCmd = &cobra.Command{
	Use:   "iss",
	Short: "Track ISS location and broadcast via Nostr",
	Long: `Demo command that fetches the International Space Station's current location
and broadcasts it as encrypted Nostr events using NIP-44 encryption.`,
	RunE: runISS,
}

func init() {
	rootCmd.AddCommand(issCmd)
	issCmd.Flags().IntP("interval", "i", defaultInterval, "Update interval in seconds")
	issCmd.Flags().StringP("sender", "s", "", "Sender private key (nsec... or @identity)")
	issCmd.Flags().StringP("receiver", "r", "", "Receiver public key (npub... or @identity)")
	
	issCmd.MarkFlagRequired("sender")
	issCmd.MarkFlagRequired("receiver")
}

func runISS(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	// Validate configuration
	config, err := validateISSConfig()
	if err != nil {
		return err
	}

	log.Printf("Starting ISS location tracker...")
	log.Printf("Update interval: %d seconds", config.interval)
	log.Printf("Relay: %s", config.relayURL)

	// Main tracking loop
	for {
		processISSUpdate(config)
		time.Sleep(time.Duration(config.interval) * time.Second)
	}
	return nil
}

type issConfig struct {
	senderSK       string
	receiverPubkey string
	relayURL       string
	interval       int
}

func validateISSConfig() (*issConfig, error) {
	sender := k.String("sender")
	if sender == "" {
		return nil, fmt.Errorf("sender is required (--sender or -s)")
	}

	receiver := k.String("receiver")
	if receiver == "" {
		return nil, fmt.Errorf("receiver is required (--receiver or -r)")
	}

	relayURL := k.String("relay")
	if relayURL == "" {
		return nil, fmt.Errorf("relay URL is required (--relay)")
	}

	// Validate sender format (should be nsec after resolution)
	if !strings.HasPrefix(sender, "nsec1") {
		return nil, fmt.Errorf("sender must be an nsec private key (starting with 'nsec1') or @identity reference")
	}

	// Validate receiver format (should be npub after resolution)
	if !strings.HasPrefix(receiver, "npub1") {
		return nil, fmt.Errorf("receiver must be an npub public key (starting with 'npub1') or @identity reference")
	}

	_, senderSK, err := nip19.Decode(sender)
	if err != nil {
		return nil, fmt.Errorf("failed to decode sender nsec: %w", err)
	}

	_, receiverPubkeyRaw, err := nip19.Decode(receiver)
	if err != nil {
		return nil, fmt.Errorf("failed to decode receiver npub: %w", err)
	}

	interval := k.Int("interval")
	if interval == 0 {
		interval = k.Int("update.interval")
		if interval == 0 {
			interval = defaultInterval
		}
	}

	return &issConfig{
		senderSK:       senderSK.(string),
		receiverPubkey: receiverPubkeyRaw.(string),
		relayURL:       relayURL,
		interval:       interval,
	}, nil
}

func processISSUpdate(config *issConfig) {
	position, err := fetchISSLocation(issAPIURL)
	if err != nil {
		log.Printf("Error fetching ISS location: %v", err)
		return
	}

	log.Printf("ISS Position: Lat=%s, Lon=%s",
		position.ISSPosition.Latitude,
		position.ISSPosition.Longitude)

	ttl := 2 * config.interval
	event, err := createLocationEvent(config.senderSK, config.receiverPubkey, position, ttl)
	if err != nil {
		log.Printf("Error creating location event: %v", err)
		return
	}

	if err := publishToRelay(config.relayURL, event); err != nil {
		log.Printf("Error publishing to relay: %v", err)
	} else {
		log.Printf("Successfully published location event (ID: %s)", event.ID)
	}
}

func fetchISSLocation(apiURL string) (*ISSPosition, error) {
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ISS location: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var position ISSPosition
	if err := json.Unmarshal(body, &position); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &position, nil
}

func createLocationEvent(senderSK, receiverPubkey string, position *ISSPosition, ttl int) (*nostr.Event, error) {
	// Parse coordinates
	lat, err := strconv.ParseFloat(position.ISSPosition.Latitude, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(position.ISSPosition.Longitude, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse longitude: %w", err)
	}

	// Create location data
	locationData := [][]interface{}{
		{"g", geohash.Encode(lat, lon)},
		{"name", "ISS"},
	}

	// Encrypt location data
	encryptedContent, err := encryptLocationData(locationData, senderSK, receiverPubkey)
	if err != nil {
		return nil, err
	}

	// Build event
	return buildLocationEvent(senderSK, receiverPubkey, encryptedContent, ttl)
}

func encryptLocationData(locationData [][]interface{}, senderSK, receiverPubkey string) (string, error) {
	locationJSON, err := json.Marshal(locationData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal location data: %w", err)
	}

	conversationKey, err := nip44.GenerateConversationKey(receiverPubkey, senderSK)
	if err != nil {
		return "", fmt.Errorf("failed to generate conversation key: %w", err)
	}

	encryptedContent, err := nip44.Encrypt(string(locationJSON), conversationKey)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt content: %w", err)
	}

	return encryptedContent, nil
}

func buildLocationEvent(senderSK, receiverPubkey, encryptedContent string, ttl int) (*nostr.Event, error) {
	senderPubkey, err := nostr.GetPublicKey(senderSK)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender public key: %w", err)
	}

	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30473,
		Tags: nostr.Tags{
			{"p", receiverPubkey},
			{"d", issLocationID},
			{"expiration", fmt.Sprintf("%d", expiration)},
		},
		Content: encryptedContent,
	}

	if err := event.Sign(senderSK); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	return event, nil
}

func publishToRelay(relayURL string, event *nostr.Event) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	defer relay.Close()

	if err := relay.Publish(ctx, *event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	return nil
}
