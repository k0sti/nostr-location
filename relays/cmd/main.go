package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"relays/internal/crawler"
	"relays/internal/database"
	"relays/internal/geolocator"
	"relays/pkg/models"
)

var (
	dbPath     string
	seedRelay  string
	maxDepth   int
	batchSize  int
	timeout    time.Duration
	outputFile string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "relays",
		Short: "Nostr relay discovery and geolocation tool",
		Long:  "A tool to discover Nostr relays and find their geographic locations based on IP addresses",
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "relays.db", "SQLite database path")
	rootCmd.PersistentFlags().StringVar(&seedRelay, "seed", "wss://relay.damus.io", "Seed relay to start discovery")
	rootCmd.PersistentFlags().IntVar(&maxDepth, "depth", 3, "Maximum discovery depth")
	rootCmd.PersistentFlags().IntVar(&batchSize, "batch", 10, "Batch size for concurrent processing")
	rootCmd.PersistentFlags().DurationVar(&timeout, "timeout", 10*time.Second, "Timeout for relay connections")
	rootCmd.PersistentFlags().StringVar(&outputFile, "output", "", "Output file for results (JSON or CSV)")

	viper.AutomaticEnv()
	viper.SetEnvPrefix("RELAYS")

	discoverCmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover Nostr relays",
		Run:   runDiscover,
	}

	geolocateCmd := &cobra.Command{
		Use:   "geolocate",
		Short: "Geolocate discovered relays",
		Run:   runGeolocate,
	}

	fullCmd := &cobra.Command{
		Use:   "full",
		Short: "Run full discovery and geolocation process",
		Run:   runFull,
	}

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show database statistics",
		Run:   runStats,
	}

	exportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export relay data",
		Run:   runExport,
	}

	rootCmd.AddCommand(discoverCmd, geolocateCmd, fullCmd, statsCmd, exportCmd)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func runDiscover(cmd *cobra.Command, args []string) {
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	c := crawler.NewCrawler(maxDepth, batchSize, timeout)
	c.AddSeedRelay(seedRelay)

	log.Printf("Starting relay discovery with seed: %s", seedRelay)
	log.Printf("Max depth: %d, Batch size: %d, Timeout: %v", maxDepth, batchSize, timeout)

	ctx := context.Background()
	relays, err := c.DiscoverRelays(ctx)
	if err != nil {
		log.Fatalf("Discovery failed: %v", err)
	}

	log.Printf("Discovered %d functioning relays", len(relays))

	for _, relayURL := range relays {
		relay := &models.Relay{
			URL:         relayURL,
			IsAlive:     true,
			LastChecked: time.Now(),
		}

		if err := db.SaveRelay(relay); err != nil {
			log.Printf("Failed to save relay %s: %v", relayURL, err)
		}
	}

	stats := c.GetStats()
	log.Printf("Discovery completed:")
	log.Printf("- Total relays found: %d", stats.TotalRelaysFound)
	log.Printf("- Functioning relays: %d", stats.FunctioningRelays)
	log.Printf("- Events processed: %d", stats.EventsProcessed)
	log.Printf("- Duration: %v", stats.Duration)
}

func runGeolocate(cmd *cobra.Command, args []string) {
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	geolocator := geolocator.NewGeoLocator()
	log.Println("Loading geolocation database...")
	if err := geolocator.LoadDatabase(); err != nil {
		log.Fatalf("Failed to load geolocation database: %v", err)
	}

	relays, err := db.GetFunctioningRelays()
	if err != nil {
		log.Fatalf("Failed to get functioning relays: %v", err)
	}

	log.Printf("Geolocating %d relays...", len(relays))

	geolocatedCount := 0
	for i, relay := range relays {
		if relay.Latitude != nil && relay.Longitude != nil {
			continue
		}

		location, err := geolocator.LocateRelay(relay.URL)
		if err != nil {
			log.Printf("Failed to geolocate %s: %v", relay.URL, err)
			continue
		}

		if err := db.UpdateRelayLocation(relay.URL, location); err != nil {
			log.Printf("Failed to update location for %s: %v", relay.URL, err)
			continue
		}

		geolocatedCount++
		log.Printf("(%d/%d) Geolocated %s: %.4f, %.4f (%s, %s)",
			i+1, len(relays), relay.URL, location.Latitude, location.Longitude,
			location.Country, location.City)
	}

	log.Printf("Geolocation completed: %d relays geolocated", geolocatedCount)
}

func runFull(cmd *cobra.Command, args []string) {
	log.Println("Starting full discovery and geolocation process...")

	runDiscover(cmd, args)
	log.Println("Discovery phase completed")

	runGeolocate(cmd, args)
	log.Println("Geolocation phase completed")

	runStats(cmd, args)
}

func runStats(cmd *cobra.Command, args []string) {
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	stats, err := db.GetStats()
	if err != nil {
		log.Fatalf("Failed to get stats: %v", err)
	}

	fmt.Println("=== Relay Database Statistics ===")
	for key, value := range stats {
		fmt.Printf("%s: %v\n", key, value)
	}
}

func runExport(cmd *cobra.Command, args []string) {
	if outputFile == "" {
		log.Fatal("Output file must be specified with --output flag")
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	relays, err := db.GetAllRelays()
	if err != nil {
		log.Fatalf("Failed to get relays: %v", err)
	}

	file, err := os.Create(outputFile)
	if err != nil {
		log.Fatalf("Failed to create output file: %v", err)
	}
	defer file.Close()

	if outputFile[len(outputFile)-4:] == ".csv" {
		fmt.Fprintln(file, "URL,Host,IsAlive,Latitude,Longitude,Country,City,LastChecked")
		for _, relay := range relays {
			lat := ""
			lon := ""
			if relay.Latitude != nil {
				lat = fmt.Sprintf("%.6f", *relay.Latitude)
			}
			if relay.Longitude != nil {
				lon = fmt.Sprintf("%.6f", *relay.Longitude)
			}

			country := ""
			city := ""
			if relay.Country != nil {
				country = *relay.Country
			}
			if relay.City != nil {
				city = *relay.City
			}

			fmt.Fprintf(file, "%s,%s,%t,%s,%s,%s,%s,%s\n",
				relay.URL, relay.Host, relay.IsAlive, lat, lon,
				country, city, relay.LastChecked.Format(time.RFC3339))
		}
	} else {
		encoder := json.NewEncoder(file)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(relays); err != nil {
			log.Fatalf("Failed to encode JSON: %v", err)
		}
	}

	log.Printf("Exported %d relays to %s", len(relays), outputFile)
}