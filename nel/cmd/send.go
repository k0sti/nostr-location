package cmd

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/nbd-wtf/go-nostr/nip44"
	"github.com/spf13/cobra"
)

var sendCmd = &cobra.Command{
	Use:   "send <geohash>",
	Short: "Send an encrypted location message",
	Long:  "Send an encrypted location message to a receiver via a Nostr relay",
	Args:  cobra.ExactArgs(1),
	RunE:  runSend,
}

func init() {
	rootCmd.AddCommand(sendCmd)

	// Required flags
	sendCmd.Flags().String("sender", "", "Sender identity (@name) or nsec")
	sendCmd.Flags().String("receiver", "", "Receiver npub or @name")

	// Optional flags with defaults
	sendCmd.Flags().Int("accuracy", 0, "Accuracy radius in meters (optional)")
	sendCmd.Flags().Int("precision", 0, "Geohash precision override (optional)")
	sendCmd.Flags().Bool("anon", false, "Send as anonymous message (omit p-tag)")
	sendCmd.Flags().String("name", "", "Name for the location (added to encrypted content)")
	sendCmd.Flags().Int("ttl", 3600, "Time to live in seconds (default 1 hour)")

	// Mark required flags
	sendCmd.MarkFlagRequired("sender")
	sendCmd.MarkFlagRequired("receiver")
}

func runSend(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	geohashInput := args[0]

	// Get and validate sender
	senderInput := k.String("sender")
	if senderInput == "" {
		return fmt.Errorf("sender is required (--sender)")
	}

	// Resolve sender identity to nsec
	senderNsec, err := ResolveIdentityReference(senderInput, "nsec")
	if err != nil {
		return fmt.Errorf("failed to resolve sender: %w", err)
	}

	// Decode sender nsec to get private key
	_, skRaw, err := nip19.Decode(senderNsec)
	if err != nil {
		return fmt.Errorf("failed to decode sender nsec: %w", err)
	}
	senderSK := skRaw.(string)
	senderPubkey, err := nostr.GetPublicKey(senderSK)
	if err != nil {
		return fmt.Errorf("failed to get sender public key: %w", err)
	}

	// Get and validate receiver
	receiverInput := k.String("receiver")
	if receiverInput == "" {
		return fmt.Errorf("receiver is required (--receiver)")
	}

	// Resolve receiver identity to npub
	receiverNpub, err := ResolveIdentityReference(receiverInput, "npub")
	if err != nil {
		return fmt.Errorf("failed to resolve receiver: %w", err)
	}

	// Decode receiver npub to get public key
	_, pubkeyRaw, err := nip19.Decode(receiverNpub)
	if err != nil {
		return fmt.Errorf("failed to decode receiver npub: %w", err)
	}
	receiverPubkey := pubkeyRaw.(string)

	// Get relay URL
	relayURL := k.String("relay")
	if relayURL == "" {
		return fmt.Errorf("relay URL is required (--relay)")
	}

	// Get optional parameters
	accuracy := k.Int("accuracy")
	precision := k.Int("precision")
	anon := k.Bool("anon")
	locationName := k.String("name")
	ttl := k.Int("ttl")

	// Validate and potentially modify geohash based on precision
	if precision > 0 && precision < len(geohashInput) {
		geohashInput = geohashInput[:precision]
	}

	// Decode geohash to verify it's valid
	lat, lon := geohash.Decode(geohashInput)
	// Validate decoded values aren't zero (which would indicate invalid geohash)
	if lat == 0 && lon == 0 && geohashInput != "s00000000000" {
		return fmt.Errorf("invalid geohash: %s", geohashInput)
	}

	// Create location data
	locationData := [][]interface{}{
		{"g", geohashInput},
	}

	// Add optional fields
	if accuracy > 0 {
		locationData = append(locationData, []interface{}{"accuracy", strconv.Itoa(accuracy)})
	}

	if locationName != "" {
		locationData = append(locationData, []interface{}{"name", locationName})
	}

	// Determine d-tag
	var dTag string
	if locationName != "" {
		// Hash the name and sender pubkey to create deterministic d-tag
		h := sha256.New()
		h.Write([]byte(locationName))
		h.Write([]byte(senderPubkey))
		dTag = hex.EncodeToString(h.Sum(nil))[:8] // Use first 8 chars of hash
	} else {
		// Generate random d-tag
		dTag = nostr.GeneratePrivateKey()[:8] // Use first 8 chars of random key
	}

	// Encrypt location data
	locationJSON, err := json.Marshal(locationData)
	if err != nil {
		return fmt.Errorf("failed to marshal location data: %w", err)
	}

	conversationKey, err := nip44.GenerateConversationKey(receiverPubkey, senderSK)
	if err != nil {
		return fmt.Errorf("failed to generate conversation key: %w", err)
	}

	encryptedContent, err := nip44.Encrypt(string(locationJSON), conversationKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt content: %w", err)
	}

	// Build event tags
	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	tags := nostr.Tags{
		{"d", dTag},
		{"expiration", fmt.Sprintf("%d", expiration)},
	}

	// Only add p-tag if not anonymous
	if !anon {
		tags = append(nostr.Tags{{"p", receiverPubkey}}, tags...)
	}

	// Create the event
	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(time.Now().Unix()),
		Kind:      30473,
		Tags:      tags,
		Content:   encryptedContent,
	}

	// Sign the event
	if err := event.Sign(senderSK); err != nil {
		return fmt.Errorf("failed to sign event: %w", err)
	}

	// Connect to relay and publish
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

	// Print confirmation
	fmt.Printf("Location message sent successfully!\n")
	fmt.Printf("  Geohash: %s\n", geohashInput)
	if locationName != "" {
		fmt.Printf("  Name: %s\n", locationName)
	}
	fmt.Printf("  D-tag: %s\n", dTag)
	fmt.Printf("  Receiver: %s\n", receiverNpub)
	if anon {
		fmt.Printf("  Mode: Anonymous (no p-tag)\n")
	} else {
		fmt.Printf("  Mode: Direct message\n")
	}
	fmt.Printf("  Relay: %s\n", relayURL)
	fmt.Printf("  Event ID: %s\n", event.ID)
	fmt.Printf("  Expires: %s\n", time.Unix(expiration, 0).Format(time.RFC3339))

	return nil
}