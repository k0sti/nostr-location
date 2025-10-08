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
	Use:   "noloc",
	Short: "Nostr Location - Handle location-first Nostr events (kind 30472 and 30473)",
	Long: `noloc is a CLI tool for handling location-first Nostr events including
both public location events (kind 30472) and encrypted location events (kind 30473).

It supports sending and receiving location events with optional end-to-end encryption,
managing multiple identities, and includes demos for ISS tracking and other location sources.`,
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
		configFile := filepath.Join(home, ".noloc.yaml")
		k.Load(file.Provider(configFile), yaml.Parser())
	}

	// Load .env file
	loadEnvFile()

	// Load environment variables (highest priority)
	k.Load(env.Provider("NOLOC_", ".", func(s string) string {
		return strings.ReplaceAll(strings.ToLower(s), "_", ".")
	}), nil)
}

// loadEnvFile loads NOLOC_ prefixed variables from .env file
func loadEnvFile() {
	if _, err := os.Stat(".env"); err != nil {
		return
	}

	tempK := koanf.New(".")
	if err := tempK.Load(file.Provider(".env"), dotenv.Parser()); err != nil {
		return
	}

	for _, key := range tempK.Keys() {
		if strings.HasPrefix(key, "NOLOC_") {
			normalizedKey := strings.ToLower(strings.TrimPrefix(key, "NOLOC_"))
			normalizedKey = strings.ReplaceAll(normalizedKey, "_", ".")
			k.Set(normalizedKey, tempK.Get(key))
		}
	}
}

// LoadFlags merges command flags into config and resolves identity references
func LoadFlags(cmd *cobra.Command) {
	// Set defaults from command flags
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if k.Get(normalizeKey(f.Name)) == nil {
			k.Set(normalizeKey(f.Name), f.DefValue)
		}
	})
	
	// Override with explicitly set flags
	cmd.Flags().Visit(func(f *pflag.Flag) {
		value := f.Value.String()
		// Resolve identity references for specific flags
		if f.Name == "sender" {
			// For iss command, sender is nsec
			if resolved, err := ResolveIdentityReference(value, "nsec"); err == nil {
				value = resolved
			}
		} else if f.Name == "receiver" {
			// Determine if this is for iss (npub) or listen (nsec) command
			if cmd.Name() == "iss" {
				if resolved, err := ResolveIdentityReference(value, "npub"); err == nil {
					value = resolved
				}
			} else if cmd.Name() == "listen" {
				if resolved, err := ResolveIdentityReference(value, "nsec"); err == nil {
					value = resolved
				}
			}
		}
		k.Set(normalizeKey(f.Name), value)
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