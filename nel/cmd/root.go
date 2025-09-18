package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/knadh/koanf/v2"
	"github.com/knadh/koanf/parsers/dotenv"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var k = koanf.New(".")

var rootCmd = &cobra.Command{
	Use:   "nel",
	Short: "Nostr Encrypted Location - Share encrypted location data via Nostr",
	Long: `NEL (Nostr Encrypted Location) is a CLI tool for sharing encrypted 
location data over the Nostr protocol using NIP-44 encryption.

It supports sending and receiving location events with end-to-end encryption,
managing multiple identities, and includes a demo ISS tracker.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	
	// Global flags
	rootCmd.PersistentFlags().String("relay", "wss://relay.damus.io", "Nostr relay URL")
	rootCmd.PersistentFlags().String("sender-nsec", "", "Sender private key (nsec format)")
	rootCmd.PersistentFlags().String("receiver-npub", "", "Receiver public key (npub format)")
	rootCmd.PersistentFlags().String("receiver-nsec", "", "Receiver private key for listening (nsec format)")
}

func initConfig() {
	// Load defaults from flag definitions
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Value.String() != "" {
			k.Set(normalizeKey(f.Name), f.Value.String())
		}
	})

	// Load config file from home directory
	if home, err := os.UserHomeDir(); err == nil {
		configFile := filepath.Join(home, ".nel.yaml")
		k.Load(file.Provider(configFile), yaml.Parser())
	}

	// Load .env file
	loadEnvFile()

	// Load environment variables (highest priority)
	k.Load(env.Provider("NEL_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(s), "_", ".")
	}), nil)
}

// loadEnvFile loads NEL_ prefixed variables from .env file
func loadEnvFile() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}

	tempK := koanf.New(".")
	if err := tempK.Load(file.Provider(".env"), dotenv.Parser()); err != nil {
		return
	}

	for _, key := range tempK.Keys() {
		if strings.HasPrefix(key, "NEL_") {
			normalizedKey := strings.ToLower(strings.TrimPrefix(key, "NEL_"))
			normalizedKey = strings.ReplaceAll(normalizedKey, "_", ".")
			k.Set(normalizedKey, tempK.Get(key))
		}
	}
}

// LoadFlags merges command flags into config
func LoadFlags(cmd *cobra.Command) {
	// Set defaults from command flags
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if k.Get(normalizeKey(f.Name)) == nil {
			k.Set(normalizeKey(f.Name), f.DefValue)
		}
	})
	
	// Override with explicitly set flags
	cmd.Flags().Visit(func(f *pflag.Flag) {
		k.Set(normalizeKey(f.Name), f.Value.String())
	})
	
	// Override with changed persistent flags
	cmd.PersistentFlags().Visit(func(f *pflag.Flag) {
		if f.Changed {
			k.Set(normalizeKey(f.Name), f.Value.String())
		}
	})
}

// normalizeKey converts flag names to config keys (sender-nsec -> sender.nsec)
func normalizeKey(name string) string {
	return strings.ReplaceAll(name, "-", ".")
}