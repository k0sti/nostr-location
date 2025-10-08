package cmd

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nbd-wtf/go-nostr"
	"github.com/nbd-wtf/go-nostr/nip19"
	"github.com/spf13/cobra"
)

var resetCmd = &cobra.Command{
	Use:   "reset <@name|nsec>",
	Short: "Delete all events created by a user",
	Long:  "Query all events created by a user and send delete request events (kind 5) for each one",
	Args:  cobra.ExactArgs(1),
	RunE:  runReset,
}

func init() {
	rootCmd.AddCommand(resetCmd)

	// Optional flags
	resetCmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	resetCmd.Flags().Int("limit", 0, "Maximum number of events to delete per batch (0 = no limit)")
	resetCmd.Flags().Bool("all-kinds", false, "Delete all event kinds (default: only location events kinds 30472, 30473)")
}

func runReset(cmd *cobra.Command, args []string) error {
	LoadFlags(cmd)

	input := args[0]
	var nsec string

	// Check if input is an identity reference or nsec
	if strings.HasPrefix(input, "@") {
		// It's an identity reference
		name := strings.TrimPrefix(input, "@")
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
	} else {
		return fmt.Errorf("invalid input: must be @name reference or nsec")
	}

	// Decode nsec to get private key
	_, skRaw, err := nip19.Decode(nsec)
	if err != nil {
		return fmt.Errorf("failed to decode nsec: %w", err)
	}
	sk := skRaw.(string)

	// Get public key
	pubkey, err := nostr.GetPublicKey(sk)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Get relay URL
	relayURL := k.String("relay")
	if relayURL == "" {
		return fmt.Errorf("relay URL is required (--relay)")
	}

	// Get flags
	dryRun := cmd.Flags().Lookup("dry-run").Value.String() == "true"
	limit := k.Int("limit")
	allKinds := cmd.Flags().Lookup("all-kinds").Value.String() == "true"

	// Connect to relay - use longer timeout for potentially many deletions
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	relay, err := nostr.RelayConnect(ctx, relayURL)
	if err != nil {
		return fmt.Errorf("failed to connect to relay: %w", err)
	}
	defer relay.Close()

	// Create filter for user's events
	filter := nostr.Filter{
		Authors: []string{pubkey},
	}

	// Only set limit if specified (non-zero)
	if limit > 0 {
		filter.Limit = limit
	}

	// If not all kinds, only query location events (both public and private)
	if !allKinds {
		filter.Kinds = []int{30472, 30473}
	}

	fmt.Printf("Querying events from %s...\n", relayURL)

	totalDeletedCount := 0
	totalFailedCount := 0
	batchNumber := 0

	// Loop until no more events are found
	for {
		batchNumber++

		// Subscribe to get events
		sub, err := relay.Subscribe(ctx, []nostr.Filter{filter})
		if err != nil {
			return fmt.Errorf("failed to subscribe: %w", err)
		}

		// Collect events
		var eventsToDelete []*nostr.Event
		timeout := time.After(5 * time.Second)

	collectLoop:
		for {
			select {
			case event := <-sub.Events:
				if event == nil {
					break collectLoop
				}
				eventsToDelete = append(eventsToDelete, event)

			case <-timeout:
				break collectLoop

			case <-ctx.Done():
				sub.Close()
				return fmt.Errorf("context cancelled")
			}
		}

		sub.Close()

		if len(eventsToDelete) == 0 {
			if batchNumber == 1 {
				fmt.Println("No events found to delete.")
			} else {
				fmt.Println("No more events to delete.")
			}
			break
		}

		fmt.Printf("\nBatch %d: Found %d events to delete.\n", batchNumber, len(eventsToDelete))

		if dryRun {
			fmt.Println("Dry run mode - showing events that would be deleted:")
			for i, event := range eventsToDelete {
				fmt.Printf("%d. Kind: %d, ID: %s, Created: %s\n",
					i+1,
					event.Kind,
					event.ID,
					time.Unix(int64(event.CreatedAt), 0).Format(time.RFC3339))

				// Show d-tag if it's an addressable event
				for _, tag := range event.Tags {
					if len(tag) >= 2 && tag[0] == "d" {
						fmt.Printf("   D-tag: %s\n", tag[1])
						break
					}
				}
			}

			if limit > 0 && len(eventsToDelete) == limit {
				fmt.Println("\nMore events may exist (dry run mode, not fetching next batch).")
			}
			break // Exit loop in dry-run mode after showing first batch
		}

		// Create and publish delete events
		fmt.Println("Sending delete requests...")
		deletedCount := 0
		failedCount := 0

		for i, eventToDelete := range eventsToDelete {
			// Show progress for large batches
			if len(eventsToDelete) >= 100 && i > 0 && i%100 == 0 {
				fmt.Printf("  Progress: %d/%d events processed...\n", i, len(eventsToDelete))
			}

			// Create delete request event (kind 5)
			deleteEvent := &nostr.Event{
				PubKey:    pubkey,
				CreatedAt: nostr.Timestamp(time.Now().Unix()),
				Kind:      5, // Delete request
				Tags: nostr.Tags{
					{"e", eventToDelete.ID},
					{"k", fmt.Sprintf("%d", eventToDelete.Kind)},
				},
				Content: "Deleted via noloc reset command",
			}

			// Sign the delete event
			if err := deleteEvent.Sign(sk); err != nil {
				if len(eventsToDelete) < 20 {
					fmt.Printf("Failed to sign delete event for %s: %v\n", eventToDelete.ID, err)
				}
				failedCount++
				continue
			}

			// Publish the delete event with a timeout
			publishCtx, publishCancel := context.WithTimeout(ctx, 5*time.Second)
			err := relay.Publish(publishCtx, *deleteEvent)
			publishCancel()

			if err != nil {
				if len(eventsToDelete) < 20 {
					fmt.Printf("Failed to publish delete event for %s: %v\n", eventToDelete.ID, err)
				}
				failedCount++
				continue
			}

			deletedCount++
			// Only show individual deletions for small batches
			if len(eventsToDelete) < 20 {
				fmt.Printf("Deleted event: %s (kind %d)\n", eventToDelete.ID, eventToDelete.Kind)
			}
		}

		// Show summary for this batch
		fmt.Printf("  Batch %d complete: %d deleted, %d failed\n", batchNumber, deletedCount, failedCount)

		totalDeletedCount += deletedCount
		totalFailedCount += failedCount

		// If we got fewer events than the limit, we've reached the end
		if limit > 0 && len(eventsToDelete) < limit {
			break
		}

		// Small delay between batches to avoid overwhelming the relay
		time.Sleep(1 * time.Second)
	}

	// Summary
	if !dryRun {
		fmt.Printf("\nReset complete:\n")
		fmt.Printf("  Total events deleted: %d\n", totalDeletedCount)
		if totalFailedCount > 0 {
			fmt.Printf("  Total failed deletes: %d\n", totalFailedCount)
		}
		fmt.Printf("  Relay: %s\n", relayURL)
	} else {
		fmt.Println("\nNo events were deleted (dry run mode).")
	}

	return nil
}
