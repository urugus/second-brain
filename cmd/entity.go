package cmd

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/urugus/second-brain/internal/model"
	"github.com/urugus/second-brain/internal/store"
)

var entityCmd = &cobra.Command{
	Use:   "entity",
	Short: "Inspect learned entities",
}

var entityListCmd = &cobra.Command{
	Use:   "list",
	Short: "List learned entities",
	RunE: func(cmd *cobra.Command, args []string) error {
		noteID, _ := cmd.Flags().GetInt64("note")
		kind, _ := cmd.Flags().GetString("kind")
		status, _ := cmd.Flags().GetString("status")
		limit, _ := cmd.Flags().GetInt("limit")

		var entities []model.Entity
		var err error
		if cmd.Flags().Changed("note") {
			entities, err = appStore.ListEntitiesByNote(noteID)
			if err != nil {
				return err
			}
			entities = filterEntities(entities, kind, status)
			if limit > 0 && len(entities) > limit {
				entities = entities[:limit]
			}
		} else {
			var kindPtr *string
			var statusPtr *string
			if strings.TrimSpace(kind) != "" {
				k := kind
				kindPtr = &k
			}
			if strings.TrimSpace(status) != "" {
				s := status
				statusPtr = &s
			}
			entities, err = appStore.ListEntities(store.EntityFilter{
				Kind:   kindPtr,
				Status: statusPtr,
				Limit:  limit,
			})
			if err != nil {
				return err
			}
		}

		if len(entities) == 0 {
			fmt.Println("No entities found.")
			return nil
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "ID\tKIND\tNAME\tSTATUS\tSTRENGTH\tSALIENCE")
		for _, entity := range entities {
			name := entity.CanonicalName
			if len(name) > 40 {
				name = name[:40] + "..."
			}
			fmt.Fprintf(
				w,
				"%d\t%s\t%s\t%s\t%.3f\t%.3f\n",
				entity.ID,
				entity.Kind,
				name,
				entity.Status,
				entity.Strength,
				entity.Salience,
			)
		}
		return w.Flush()
	},
}

func filterEntities(entities []model.Entity, kind string, status string) []model.Entity {
	kind = strings.ToLower(strings.TrimSpace(kind))
	status = strings.ToLower(strings.TrimSpace(status))
	if kind == "" && status == "" {
		return entities
	}

	filtered := make([]model.Entity, 0, len(entities))
	for _, entity := range entities {
		if kind != "" && strings.ToLower(entity.Kind) != kind {
			continue
		}
		if status != "" && strings.ToLower(entity.Status) != status {
			continue
		}
		filtered = append(filtered, entity)
	}
	return filtered
}

var entityShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a learned entity by ID",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid entity ID: %s", args[0])
		}

		entity, err := appStore.GetEntity(id)
		if err != nil {
			return fmt.Errorf("entity %d not found: %w", id, err)
		}
		fmt.Printf("Entity #%d\n", entity.ID)
		fmt.Printf("  Kind:     %s\n", entity.Kind)
		fmt.Printf("  Name:     %s\n", entity.CanonicalName)
		fmt.Printf("  Status:   %s\n", entity.Status)
		fmt.Printf("  Strength: %.3f\n", entity.Strength)
		fmt.Printf("  Salience: %.3f\n", entity.Salience)
		fmt.Printf("  Updated:  %s\n", entity.UpdatedAt.Local().Format("2006-01-02 15:04"))
		return nil
	},
}

var entitySetStatusCmd = &cobra.Command{
	Use:   "set-status <id> <status>",
	Short: "Update status of a learned entity",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid entity ID: %s", args[0])
		}
		status := strings.TrimSpace(args[1])
		if status == "" {
			return fmt.Errorf("status must not be empty")
		}

		if err := appStore.UpdateEntityStatus(id, status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("entity %d not found", id)
			}
			return err
		}
		fmt.Printf("Entity #%d status updated to %s.\n", id, strings.ToLower(status))
		return nil
	},
}

func init() {
	entityListCmd.Flags().Int64("note", 0, "Filter entities by note ID")
	entityListCmd.Flags().String("kind", "", "Filter by entity kind (person|concept|org|project)")
	entityListCmd.Flags().String("status", "", "Filter by entity status (candidate|confirmed|rejected|archived)")
	entityListCmd.Flags().Int("limit", 20, "Maximum number of entities")

	entityCmd.AddCommand(entityListCmd, entityShowCmd, entitySetStatusCmd)
	rootCmd.AddCommand(entityCmd)
}
