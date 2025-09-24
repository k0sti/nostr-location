package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/mmcloughlin/geohash"
	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cobra"
)

type TrainLocation struct {
	TrainNumber   int      `json:"trainNumber"`
	DepartureDate string   `json:"departureDate"`
	Timestamp     string   `json:"timestamp"`
	Location      GeoPoint `json:"location"`
	Speed         int      `json:"speed"`
	Accuracy      int      `json:"accuracy"`
}

type GeoPoint struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}

var trainsCmd = &cobra.Command{
	Use:   "trains",
	Short: "Listen to train locations and broadcast via public Nostr events",
	Long: `Connect to Finnish train location WebSocket/MQTT service and broadcast
real-time train positions as public Nostr location events (kind 30472).

Each train is identified by its train number and updates are sent as
replaceable events using the train number as the d-tag.`,
	RunE: runTrains,
}

func init() {
	rootCmd.AddCommand(trainsCmd)
	trainsCmd.Flags().StringP("sender", "s", "", "Sender private key (nsec... or @identity)")
	trainsCmd.Flags().IntP("ttl", "t", 3600, "Time-to-live for events in seconds")
	trainsCmd.Flags().IntP("precision", "p", 7, "Geohash precision (1-12)")

	trainsCmd.MarkFlagRequired("sender")
}

func runTrains(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	senderNsec := k.String("sender")
	relayURL := k.String("relay")
	ttl := k.Int("ttl")
	precision := k.Int("precision")

	if precision < 1 || precision > 12 {
		return fmt.Errorf("precision must be between 1 and 12")
	}

	// Decode sender private key
	_, skRaw, err := nip19.Decode(senderNsec)
	if err != nil {
		return fmt.Errorf("failed to decode sender nsec: %w", err)
	}
	senderSK := skRaw.(string)

	// Get sender public key
	senderPubkey, err := nostr.GetPublicKey(senderSK)
	if err != nil {
		return fmt.Errorf("failed to get sender public key: %w", err)
	}

	fmt.Printf("🚂 Train Location Tracker\n")
	fmt.Printf("  Sender: %s\n", senderPubkey[:8]+"...")
	fmt.Printf("  Relay: %s\n", relayURL)
	fmt.Printf("  TTL: %d seconds\n", ttl)
	fmt.Printf("  Geohash precision: %d\n", precision)

	// Connect to Nostr relay
	ctx := context.Background()
	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	defer relay.Close()

	fmt.Printf("✅ Connected to Nostr relay\n\n")

	// Create MQTT client
	clientID := fmt.Sprintf("nel_train_%d", rand.Intn(10000))
	opts := mqtt.NewClientOptions()
	// Use TCP connection to rata-mqtt.digitraffic.fi
	opts.AddBroker("tcp://rata-mqtt.digitraffic.fi:1883")
	opts.SetClientID(clientID)
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)

	// Message handler
	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		// Parse train location
		var trainLoc TrainLocation
		if err := json.Unmarshal(msg.Payload(), &trainLoc); err != nil {
			fmt.Printf("❌ Failed to parse message: %v\n", err)
			return
		}

		// Create and send Nostr event
		event, err := createTrainLocationEvent(trainLoc, senderSK, senderPubkey, ttl, precision)
		if err != nil {
			fmt.Printf("❌ Failed to create event for train %d: %v\n", trainLoc.TrainNumber, err)
			return
		}

		if err := relay.Publish(ctx, *event); err != nil {
			fmt.Printf("❌ Failed to publish event for train %d: %v\n", trainLoc.TrainNumber, err)
			return
		}

		fmt.Printf("🚂 Train %d: %.4f, %.4f (speed: %d km/h, accuracy: %d m)\n",
			trainLoc.TrainNumber,
			trainLoc.Location.Coordinates[1], // lat
			trainLoc.Location.Coordinates[0], // lon
			trainLoc.Speed,
			trainLoc.Accuracy,
		)
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		fmt.Printf("⚠️  MQTT connection lost: %v\n", err)
	})

	opts.SetOnConnectHandler(func(client mqtt.Client) {
		fmt.Printf("✅ Connected to MQTT broker\n")
		fmt.Printf("📡 Subscribing to train locations...\n\n")

		// Subscribe to train locations
		if token := client.Subscribe("train-locations/#", 0, nil); token.Wait() && token.Error() != nil {
			fmt.Printf("❌ Failed to subscribe: %v\n", token.Error())
		}
	})

	// Connect to MQTT broker
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}
	defer client.Disconnect(250)

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	<-sigChan

	fmt.Println("\n👋 Shutting down...")
	return nil
}

func createTrainLocationEvent(trainLoc TrainLocation, senderSK, senderPubkey string, ttl, precision int) (*nostr.Event, error) {
	// Get coordinates
	if len(trainLoc.Location.Coordinates) < 2 {
		return nil, fmt.Errorf("invalid coordinates")
	}
	lon := trainLoc.Location.Coordinates[0]
	lat := trainLoc.Location.Coordinates[1]

	// Create geohash
	gh := geohash.EncodeWithPrecision(lat, lon, uint(precision))

	// Calculate expiration
	expiration := time.Now().Add(time.Duration(ttl) * time.Second).Unix()

	// Build tags for public event (kind 30472)
	tags := nostr.Tags{
		{"g", gh}, // Required geohash
		{"d", fmt.Sprintf("train-%d", trainLoc.TrainNumber)},     // Train identifier
		{"expiration", fmt.Sprintf("%d", expiration)},            // Expiration time
		{"title", fmt.Sprintf("Train %d", trainLoc.TrainNumber)}, // Train number as title
		{"speed", strconv.Itoa(trainLoc.Speed)},                  // Speed in summary
		{"accuracy", strconv.Itoa(trainLoc.Accuracy)},            // Accuracy in meters
	}

	// Add hashtags
	tags = append(tags, nostr.Tag{"t", "train"})
	tags = append(tags, nostr.Tag{"t", "finland"})
	tags = append(tags, nostr.Tag{"t", "railway"})

	// Parse timestamp and use it for created_at
	timestamp, err := time.Parse(time.RFC3339, trainLoc.Timestamp)
	if err != nil {
		timestamp = time.Now()
	}

	// Create public location event (kind 30472)
	event := &nostr.Event{
		PubKey:    senderPubkey,
		CreatedAt: nostr.Timestamp(timestamp.Unix()),
		Kind:      30472, // Public location event kind
		Tags:      tags,
		Content:   "", // No content field for public events
	}

	if err := event.Sign(senderSK); err != nil {
		return nil, fmt.Errorf("failed to sign event: %w", err)
	}

	return event, nil
}
