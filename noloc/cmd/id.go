package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/skip2/go-qrcode"
	"github.com/spf13/cobra"
)

type Identity struct {
	Name  string `json:"name"`
	Nsec  string `json:"nsec"`
	Npub  string `json:"npub"`
	Hex   string `json:"hex"`
	Added string `json:"added"`
}

var idCmd = &cobra.Command{
	Use:   "id",
	Short: "Manage Nostr identities",
	Long:  `Manage known Nostr identities (nsec keys) for easy reference`,
}

var idListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all known identities",
	RunE:  listIdentities,
}

var idAddCmd = &cobra.Command{
	Use:   "add <name> <nsec>",
	Short: "Add a new identity",
	Args:  cobra.ExactArgs(2),
	RunE:  addIdentity,
}

var idRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete", "del"},
	Short:   "Remove an identity",
	Args:    cobra.ExactArgs(1),
	RunE:    removeIdentity,
}

var idShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details of an identity",
	Args:  cobra.ExactArgs(1),
	RunE:  showIdentity,
}

var idGenerateCmd = &cobra.Command{
	Use:     "generate <name>",
	Aliases: []string{"gen", "new"},
	Short:   "Generate a new identity and save it",
	Args:    cobra.ExactArgs(1),
	RunE:    generateIdentity,
}

var idExportCmd = &cobra.Command{
	Use:   "export <@name|nsec>",
	Short: "Export identity as URL with QR code",
	Long:  "Export an identity as a URL and QR code for sharing. Accepts either @name reference or nsec directly.",
	Args:  cobra.ExactArgs(1),
	RunE:  exportIdentity,
}

func init() {
	rootCmd.AddCommand(idCmd)
	idCmd.AddCommand(idListCmd)
	idCmd.AddCommand(idAddCmd)
	idCmd.AddCommand(idRemoveCmd)
	idCmd.AddCommand(idShowCmd)
	idCmd.AddCommand(idGenerateCmd)
	idCmd.AddCommand(idExportCmd)
}

func getIdentityFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".noloc-identities.json")
}

func loadIdentities() (map[string]Identity, error) {
	identities := make(map[string]Identity)

	file := getIdentityFile()
	data, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return identities, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &identities); err != nil {
		return nil, err
	}

	return identities, nil
}

func saveIdentities(identities map[string]Identity) error {
	data, err := json.MarshalIndent(identities, "", "  ")
	if err != nil {
		return err
	}

	file := getIdentityFile()
	return os.WriteFile(file, data, 0600)
}

func listIdentities(cmd *cobra.Command, args []string) error {
	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	if len(identities) == 0 {
		fmt.Println("No identities found. Use 'noloc id add' or 'noloc id generate' to create one.")
		return nil
	}

	fmt.Println("Known Identities:")
	fmt.Println(strings.Repeat("-", 60))

	for name, id := range identities {
		fmt.Printf("Name: %s\n", name)
		fmt.Printf("  Npub: %s\n", id.Npub)
		fmt.Printf("  Added: %s\n", id.Added)
		fmt.Println()
	}

	return nil
}

func addIdentity(cmd *cobra.Command, args []string) error {
	name := args[0]
	nsec := args[1]

	if !strings.HasPrefix(nsec, "nsec1") {
		return fmt.Errorf("invalid nsec format (must start with 'nsec1')")
	}

	_, skRaw, err := nip19.Decode(nsec)
	if err != nil {
		return fmt.Errorf("failed to decode nsec: %w", err)
	}
	sk := skRaw.(string)

	pubkey, err := nostr.GetPublicKey(sk)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	npub, err := nip19.EncodePublicKey(pubkey)
	if err != nil {
		return fmt.Errorf("failed to encode npub: %w", err)
	}

	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	if _, exists := identities[name]; exists {
		return fmt.Errorf("identity '%s' already exists", name)
	}

	identities[name] = Identity{
		Name:  name,
		Nsec:  nsec,
		Npub:  npub,
		Hex:   pubkey,
		Added: time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := saveIdentities(identities); err != nil {
		return fmt.Errorf("failed to save identities: %w", err)
	}

	fmt.Printf("Added identity '%s'\n", name)
	fmt.Printf("  Npub: %s\n", npub)

	return nil
}

func removeIdentity(cmd *cobra.Command, args []string) error {
	name := args[0]

	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	if _, exists := identities[name]; !exists {
		return fmt.Errorf("identity '%s' not found", name)
	}

	delete(identities, name)

	if err := saveIdentities(identities); err != nil {
		return fmt.Errorf("failed to save identities: %w", err)
	}

	fmt.Printf("Removed identity '%s'\n", name)
	return nil
}

func showIdentity(cmd *cobra.Command, args []string) error {
	name := args[0]

	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	id, exists := identities[name]
	if !exists {
		return fmt.Errorf("identity '%s' not found", name)
	}

	fmt.Printf("Identity: %s\n", name)
	fmt.Printf("  Nsec: %s\n", id.Nsec)
	fmt.Printf("  Npub: %s\n", id.Npub)
	fmt.Printf("  Hex:  %s\n", id.Hex)
	fmt.Printf("  Added: %s\n", id.Added)

	return nil
}

// ResolveIdentityReference resolves @name to npub/nsec from stored identities
func ResolveIdentityReference(value string, keyType string) (string, error) {
	// Check if it's an identity reference
	if !strings.HasPrefix(value, "@") {
		return value, nil
	}

	// Extract the identity name
	name := strings.TrimPrefix(value, "@")
	if name == "" {
		return "", fmt.Errorf("invalid identity reference: missing name after @")
	}

	// Load identities
	identities, err := loadIdentities()
	if err != nil {
		return "", fmt.Errorf("failed to load identities: %w", err)
	}

	// Look up the identity
	identity, exists := identities[name]
	if !exists {
		return "", fmt.Errorf("identity '%s' not found", name)
	}

	// Return the appropriate key based on keyType
	switch keyType {
	case "nsec":
		return identity.Nsec, nil
	case "npub":
		return identity.Npub, nil
	default:
		return "", fmt.Errorf("invalid key type: %s", keyType)
	}
}

func generateIdentity(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Load existing identities
	identities, err := loadIdentities()
	if err != nil {
		return fmt.Errorf("failed to load identities: %w", err)
	}

	// Check if name already exists
	if _, exists := identities[name]; exists {
		return fmt.Errorf("identity '%s' already exists", name)
	}

	// Generate new keys
	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)
	nsec, _ := nip19.EncodePrivateKey(sk)
	npub, _ := nip19.EncodePublicKey(pk)

	// Save identity
	identities[name] = Identity{
		Name:  name,
		Nsec:  nsec,
		Npub:  npub,
		Hex:   pk,
		Added: time.Now().Format("2006-01-02 15:04:05"),
	}

	if err := saveIdentities(identities); err != nil {
		return fmt.Errorf("failed to save identity: %w", err)
	}

	// Display results
	fmt.Printf("Generated and saved new identity '%s':\n", name)
	fmt.Printf("  Private Key (nsec): %s\n", nsec)
	fmt.Printf("  Public Key (npub):  %s\n", npub)
	fmt.Printf("  Public Key (hex):   %s\n", pk)
	fmt.Println("\n⚠️  Keep your private key (nsec) secret and secure!")

	return nil
}

func exportIdentity(cmd *cobra.Command, args []string) error {
	input := args[0]
	var nsec string
	var name string

	// Check if input is an identity reference or nsec
	if strings.HasPrefix(input, "@") {
		// It's an identity reference
		name = strings.TrimPrefix(input, "@")
		identities, err := loadIdentities()
		if err != nil {
			return fmt.Errorf("failed to load identities: %w", err)
		}

		identity, exists := identities[name]
		if !exists {
			return fmt.Errorf("identity '%s' not found", name)
		}
		nsec = identity.Nsec
	} else if strings.HasPrefix(input, "nsec1") {
		// It's a direct nsec
		nsec = input
		// Try to find the name for this nsec
		identities, _ := loadIdentities()
		for idName, identity := range identities {
			if identity.Nsec == nsec {
				name = idName
				break
			}
		}
		// If no name found, use "unknown"
		if name == "" {
			name = "unknown"
		}
	} else {
		return fmt.Errorf("invalid input: must be @name reference or nsec")
	}

	// Decode nsec to get the private key hex
	_, skRaw, err := nip19.Decode(nsec)
	if err != nil {
		return fmt.Errorf("failed to decode nsec: %w", err)
	}
	sk := skRaw.(string)

	// Build the URL
	params := url.Values{}
	params.Add("g", sk)
	params.Add("n", name)
	exportURL := fmt.Sprintf("https://spotstr.nexel.space?%s", params.Encode())

	// Generate QR code ASCII art
	qr, err := qrcode.New(exportURL, qrcode.Medium)
	if err != nil {
		return fmt.Errorf("failed to generate QR code: %w", err)
	}

	// Print the results
	fmt.Println("Export URL:")
	fmt.Println(exportURL)
	fmt.Println("\nQR Code:")
	fmt.Println(qr.ToSmallString(false))

	return nil
}
