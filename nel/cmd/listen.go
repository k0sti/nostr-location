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

var listenCmd = &cobra.Command{
	Use:   "listen",
	Short: "Listen for encrypted location messages",
	Long: `Subscribe to a Nostr relay and listen for encrypted location events
addressed to your public key. Automatically decrypts messages using your
private key and displays location data including coordinates.`,
	RunE: runListen,
}

func init() {
	rootCmd.AddCommand(listenCmd)

	// No additional flags needed
}

func runListen(cmd *cobra.Command, args []string) error {
	// Load flags into config
	LoadFlags(cmd)

	// Access config with dot notation
	receiverNsec := k.String("receiver.nsec")
	if receiverNsec == "" {
		return fmt.Errorf("receiver nsec is required (--receiver-nsec or NEL_RECEIVER_NSEC)")
	}

	relayURL := k.String("relay")
	if relayURL == "" {
		return fmt.Errorf("relay URL is required (--relay or NEL_LOCATION_RELAY)")
	}

	_, receiverSKRaw, err := nip19.Decode(receiverNsec)
	if err != nil {
		return fmt.Errorf("failed to decode receiver nsec: %w", err)
	}
	receiverSK := receiverSKRaw.(string)

	receiverPubkey, err := nostr.GetPublicKey(receiverSK)
	if err != nil {
		return fmt.Errorf("failed to get receiver public key: %w", err)
	}

	receiverNpub, err := nip19.EncodePublicKey(receiverPubkey)
	if err != nil {
		return fmt.Errorf("failed to encode receiver npub: %w", err)
	}

	log.Printf("Starting location listener...")
	log.Printf("Receiver npub: %s", receiverNpub)
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

	filters := []nostr.Filter{{
		Kinds: []int{30473},
		Tags: nostr.TagMap{
			"p": []string{receiverPubkey},
		},
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

			outputFormatted(event, receiverSK)
		}
	}
}

func decryptLocationContent(encryptedContent string, receiverSK string, senderPubkey string) ([][]interface{}, error) {
	conversationKey, err := nip44.GenerateConversationKey(senderPubkey, receiverSK)
	if err != nil {
		return nil, fmt.Errorf("failed to generate conversation key: %w", err)
	}

	decryptedContent, err := nip44.Decrypt(encryptedContent, conversationKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt content: %w", err)
	}

	var locationData [][]interface{}
	if err := json.Unmarshal([]byte(decryptedContent), &locationData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal location data: %w", err)
	}

	return locationData, nil
}

func outputFormatted(event *nostr.Event, receiverSK string) {
	fmt.Printf("\n📍 New Location Event Received\n")
	fmt.Printf("Event ID: %s\n", event.ID)
	fmt.Printf("From: %s\n", event.PubKey)
	fmt.Printf("Created: %s\n", event.CreatedAt.Time().Format("2006-01-02 15:04:05"))

	fmt.Printf("\nPublic Tags:\n")
	for _, tag := range event.Tags {
		if len(tag) > 0 {
			fmt.Printf("  - %s", tag[0])
			for i := 1; i < len(tag); i++ {
				fmt.Printf(": %s", tag[i])
			}
			fmt.Println()
		}
	}

	locationData, err := decryptLocationContent(event.Content, receiverSK, event.PubKey)
	if err != nil {
		fmt.Printf("\n❌ Failed to decrypt: %v\n", err)
	} else {
		fmt.Printf("\n🔓 Decrypted Location Data:\n")
		var geohashStr string
		for _, tag := range locationData {
			if len(tag) >= 2 {
				fmt.Printf("  - %v: %v\n", tag[0], tag[1])
				if tag[0] == "g" {
					geohashStr = fmt.Sprintf("%v", tag[1])
				}
				if len(tag) > 2 {
					for i := 2; i < len(tag); i++ {
						fmt.Printf("    + %v\n", tag[i])
					}
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
	}
	fmt.Println("=============================================================")
}

