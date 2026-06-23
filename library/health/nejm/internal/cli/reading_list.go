// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Phase 3: personal reading list — queue DOIs locally and track read/unread state.

package cli

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/health/nejm/internal/store"
	"github.com/spf13/cobra"
)

// ensureReadingListTable creates the reading_list table if it doesn't exist.
func ensureReadingListTable(db *store.Store) error {
	_, err := db.DB().Exec(`CREATE TABLE IF NOT EXISTS reading_list (
		doi TEXT PRIMARY KEY,
		added_at TEXT NOT NULL,
		read_at TEXT,
		notes TEXT
	)`)
	return err
}

func newNovelReadingListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "reading-list",
		Short:       "Queue NEJM articles by DOI locally and track read/unread state.",
		Annotations: map[string]string{"mcp:read-only": "true"},
	}

	// add
	addCmd := &cobra.Command{
		Use:     "add <doi>",
		Short:   "Add an article to the reading list",
		Example: "  nejm-pp-cli reading-list add 10.1056/NEJMoa2506905",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			doi := strings.TrimSpace(args[0])
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, err := store.OpenWithContext(ctx, defaultDBPath("nejm-pp-cli"))
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if err := ensureReadingListTable(db); err != nil {
				return err
			}
			_, err = db.DB().ExecContext(ctx,
				`INSERT INTO reading_list (doi, added_at) VALUES (?, ?)
				 ON CONFLICT(doi) DO NOTHING`,
				doi, time.Now().UTC().Format(time.RFC3339))
			if err != nil {
				return fmt.Errorf("adding to reading list: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added %s to reading list\n", doi)
			return nil
		},
	}

	// ls
	lsCmd := &cobra.Command{
		Use:         "ls",
		Short:       "List articles in the reading list",
		Aliases:     []string{"list"},
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  nejm-pp-cli reading-list ls\n  nejm-pp-cli reading-list ls --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, err := store.OpenWithContext(ctx, defaultDBPath("nejm-pp-cli"))
			if err != nil {
				return fmt.Errorf("opening database: %w", err)
			}
			defer db.Close()
			if err := ensureReadingListTable(db); err != nil {
				return err
			}

			rows, err := db.DB().QueryContext(ctx,
				`SELECT r.doi, r.added_at, r.read_at, a.title, a.authors
				 FROM reading_list r
				 LEFT JOIN article a ON a.doi = r.doi
				 ORDER BY r.added_at DESC`)
			if err != nil {
				return fmt.Errorf("querying reading list: %w", err)
			}
			defer rows.Close()

			type rlRow struct {
				DOI     string `json:"doi"`
				AddedAt string `json:"added_at"`
				ReadAt  string `json:"read_at,omitempty"`
				Title   string `json:"title,omitempty"`
				Authors string `json:"authors,omitempty"`
				Read    bool   `json:"read"`
			}
			var results []rlRow
			for rows.Next() {
				// 🔧 JAVÍTÁS: sql.NullString használata a title és authors mezőkhöz
				// hogy kezelni tudjuk a SQL NULL értékeket
				var doi, addedAt string
				var readAtNull *string
				var title sql.NullString
				var authors sql.NullString
				
				if err := rows.Scan(&doi, &addedAt, &readAtNull, &title, &authors); err != nil {
					continue
				}
				
				// 🔧 JAVÍTÁS: NULL esetén üres stringet használunk
				titleStr := ""
				if title.Valid {
					titleStr = title.String
				}
				authorsStr := ""
				if authors.Valid {
					authorsStr = authors.String
				}
				
				readAtStr := ""
				read := false
				if readAtNull != nil {
					readAtStr = *readAtNull
					read = true
				}
				
				results = append(results, rlRow{
					DOI:     doi,
					AddedAt: addedAt,
					ReadAt:  readAtStr,
					Title:   titleStr,
					Authors: authorsStr,
					Read:    read,
				})
			}

			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			if len(results) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "reading list is empty")
				return nil
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Reading List (%d entries)\n", len(results))
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 80))
			for _, r := range results {
				status := "[ ]"
				if r.Read {
					status = "[✓]"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", status, r.DOI)
				if r.Title != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", r.Title)
				}
				if r.Authors != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "    by %s\n", r.Authors)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "    added: %s\n", r.AddedAt)
				if r.Read {
					fmt.Fprintf(cmd.OutOrStdout(), "    read: %s\n", r.ReadAt)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			return nil
		},
	}

	cmd.AddCommand(addCmd, lsCmd)
	return cmd
}
