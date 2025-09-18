package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
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
	Use:     "generate [name]",
	Aliases: []string{"gen", "new"},
	Short:   "Generate a new identity",
	RunE:    generateIdentity,
}

func init() {
	rootCmd.AddCommand(idCmd)
	idCmd.AddCommand(idListCmd)
	idCmd.AddCommand(idAddCmd)
	idCmd.AddCommand(idRemoveCmd)
	idCmd.AddCommand(idShowCmd)
	idCmd.AddCommand(idGenerateCmd)
	
	idGenerateCmd.Flags().Bool("save", false, "Save the generated identity")
}

func getIdentityFile() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nel-identities.json")
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
		fmt.Println("No identities found. Use 'nel id add' or 'nel id generate' to create one.")
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

func generateIdentity(cmd *cobra.Command, args []string) error {
	sk := nostr.GeneratePrivateKey()
	pk, _ := nostr.GetPublicKey(sk)
	nsec, _ := nip19.EncodePrivateKey(sk)
	npub, _ := nip19.EncodePublicKey(pk)
	
	fmt.Println("Generated New Identity:")
	fmt.Printf("  Private Key (nsec): %s\n", nsec)
	fmt.Printf("  Public Key (npub):  %s\n", npub)
	fmt.Printf("  Public Key (hex):   %s\n", pk)
	
	shouldSave, _ := cmd.Flags().GetBool("save")
	if shouldSave && len(args) > 0 {
		name := args[0]
		
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
			Hex:   pk,
			Added: time.Now().Format("2006-01-02 15:04:05"),
		}
		
		if err := saveIdentities(identities); err != nil {
			return fmt.Errorf("failed to save identity: %w", err)
		}
		
		fmt.Printf("\n✓ Saved as '%s'\n", name)
	} else if shouldSave {
		fmt.Println("\n⚠️  To save, provide a name: nel id generate --save <name>")
	}
	
	fmt.Println("\n⚠️  Keep your private key (nsec) secret and secure!")
	
	return nil
}