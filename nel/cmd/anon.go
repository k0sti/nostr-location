package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/spf13/cobra"
)

var anonCmd = &cobra.Command{
	Use:   "anon",
	Short: "Listen for location messages from all known identities",
	Long: `Subscribe to a Nostr relay and listen for encrypted location events
from all known npubs. For events without p-tag, attempts to decrypt using
all known nsecs. Shows one line for each failed attempt and full event
contents on successful decode.`,
	RunE: runAnon,
}

func init() {
	rootCmd.AddCommand(anonCmd)
}

func runAnon(cmd *cobra.Command, args []string) error {
	// Load flags into config
	LoadFlags(cmd)

	relayURL := k.String("relay")
	if relayURL == "" {
		return fmt.Errorf("relay URL is required (--relay)")
	}

	// Load all known identities
	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	if len(identities) == 0 {
		return fmt.Errorf("no identities found. Use 'nel id generate' to create identities")
	}

	// Collect all npubs and nsecs
	var npubs []string
	nsecs := make(map[string]string) // map[name]nsec
	for name, id := range identities {
		npubs = append(npubs, id.Hex) // Use hex pubkey for filter
		nsecs[name] = id.Nsec
	}

	log.Printf("Starting anonymous location listener...")
	log.Printf("Monitoring %d known identities", len(identities))
	log.Printf("Relay: %s", relayURL)
	log.Println("Listening for encrypted location messages...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	defer relay.Close()

	// Create filter for location events from known pubkeys
	filters := []nostr.Filter{{
		Kinds:   []int{30473},
		Authors: npubs,
	}}

	sub, err := relay.Subscribe(ctx, filters)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Println("Subscribed to location events. Press Ctrl+C to exit.")
	fmt.Println("=============================================================")

	for {
		select {
		case <-ctx.Done():
			return nil
		case event := <-sub.Events:
			if event == nil {
				continue
			}

			processAnonEvent(event, identities, nsecs)
		}
	}
}

func processAnonEvent(event *nostr.Event, identities map[string]Identity, nsecs map[string]string) {
	fmt.Printf("\n📍 Location Event Received\n")
	fmt.Printf("Event ID: %s\n", event.ID)
	fmt.Printf("From: %s", event.PubKey)
	
	// Find sender name if known
	for name, id := range identities {
		if id.Hex == event.PubKey {
			fmt.Printf(" (%s)", name)
			break
		}
	}
	fmt.Println()
	
	fmt.Printf("Created: %s\n", event.CreatedAt.Time().Format("2006-01-02 15:04:05"))

	// Check if event has p-tag
	var hasP bool
	var pTagValue string
	for _, tag := range event.Tags {
		if len(tag) > 0 && tag[0] == "p" {
			hasP = true
			if len(tag) > 1 {
				pTagValue = tag[1]
			}
			break
		}
	}

	if hasP {
		// Event has p-tag, try to decrypt with matching identity
		fmt.Printf("Has p-tag: %s\n", pTagValue)
		
		// Find matching identity
		var matchingNsec string
		var matchingName string
		for name, id := range identities {
			if id.Hex == pTagValue {
				matchingNsec = id.Nsec
				matchingName = name
				break
			}
		}

		if matchingNsec != "" {
			fmt.Printf("Attempting decrypt with identity: %s\n", matchingName)
			if tryDecryptLocation(event, matchingNsec, matchingName) {
				// Success, already printed
			} else {
				fmt.Printf("  ❌ Failed to decrypt with %s\n", matchingName)
			}
		} else {
			fmt.Printf("⚠️  No matching identity for p-tag\n")
		}
	} else {
		// No p-tag, try all known nsecs
		fmt.Println("No p-tag (anonymous). Trying all known identities...")
		
		successCount := 0
		for name, nsec := range nsecs {
			if tryDecryptLocation(event, nsec, name) {
				successCount++
				break // Stop after first successful decrypt
			} else {
				fmt.Printf("  ❌ Failed with %s\n", name)
			}
		}
		
		if successCount == 0 {
			fmt.Println("  ⚠️  Could not decrypt with any known identity")
		}
	}

	fmt.Println("=============================================================")
}

func tryDecryptLocation(event *nostr.Event, nsec string, identityName string) bool {
	// Decode nsec to get secret key
	_, skRaw, err := nip19.Decode(nsec)
	if err != nil {
		return false
	}
	sk := skRaw.(string)

	// Try to decrypt
	conversationKey, err := nip44.GenerateConversationKey(event.PubKey, sk)
	if err != nil {
		return false
	}

	decryptedContent, err := nip44.Decrypt(event.Content, conversationKey)
	if err != nil {
		return false
	}

	// Try to parse as location data
	var locationData [][]interface{}
	if err := json.Unmarshal([]byte(decryptedContent), &locationData); err != nil {
		return false
	}

	// Success! Print the decrypted data
	fmt.Printf("\n🔓 Successfully decrypted with %s:\n", identityName)
	
	var geohashStr string
	for _, tag := range locationData {
		if len(tag) >= 2 {
			fmt.Printf("  - %v: %v\n", tag[0], tag[1])
			if tag[0] == "g" {
				geohashStr = fmt.Sprintf("%v", tag[1])
			}
		}
	}

	if geohashStr != "" {
		lat, lon := geohash.Decode(geohashStr)
		fmt.Printf("\n📌 Converted Coordinates:\n")
		fmt.Printf("  - Latitude:  %.6f\n", lat)
		fmt.Printf("  - Longitude: %.6f\n", lon)
		fmt.Printf("  - Map: https://www.openstreetmap.org/?mlat=%.6f&mlon=%.6f&zoom=4\n", lat, lon)
	}

	return true
}