// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Phase 3: personal reading list — queue DOIs locally and track read/unread state.

package cli

import (
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
				var doi, addedAt, title, authors string
				var readAtNull *string
				if err := rows.Scan(&doi, &addedAt, &readAtNull, &title, &authors); err != nil {
					continue
				}
				readAt := ""
				if readAtNull != nil {
					readAt = *readAtNull
				}
				results = append(results, rlRow{
					DOI:     doi,
					AddedAt: addedAt,
					ReadAt:  readAt,
					Title:   title,
					Authors: authors,
					Read:    readAt != "",
				})
			}
			// Surface any error from row iteration before treating the result set as complete.
			if err := rows.Err(); err != nil {
				return fmt.Errorf("reading list iteration error: %w", err)
			}
			if len(results) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "Reading list is empty. Use 'nejm-pp-cli reading-list add <doi>' to add articles.")
				if flags.asJSON {
					return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage("[]"), flags)
				}
				return nil
			}
			data, _ := json.Marshal(results)
			return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage(data), flags)
		},
	}

	// read (mark as read)
	readCmd := &cobra.Command{
		Use:     "read <doi>",
		Short:   "Mark an article as read",
		Example: "  nejm-pp-cli reading-list read 10.1056/NEJMoa2506905",
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
			res, err := db.DB().ExecContext(ctx,
				`UPDATE reading_list SET read_at = ? WHERE doi = ?`,
				time.Now().UTC().Format(time.RFC3339), doi)
			if err != nil {
				return fmt.Errorf("marking as read: %w", err)
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("DOI %s not in reading list; use 'reading-list add' first", doi)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "marked %s as read\n", doi)
			return nil
		},
	}

	// rm
	rmCmd := &cobra.Command{
		Use:     "rm <doi>",
		Short:   "Remove an article from the reading list",
		Aliases: []string{"remove"},
		Example: "  nejm-pp-cli reading-list rm 10.1056/NEJMoa2506905",
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
			res, err := db.DB().ExecContext(ctx, `DELETE FROM reading_list WHERE doi = ?`, doi)
			if err != nil {
				return fmt.Errorf("removing from reading list: %w", err)
			}
			n, _ := res.RowsAffected()
			if n == 0 {
				return fmt.Errorf("%s not found in reading list", doi)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "removed %s from reading list\n", doi)
			return nil
		},
	}

	addCmd.Annotations = map[string]string{}
	readCmd.Annotations = map[string]string{}
	rmCmd.Annotations = map[string]string{}
	cmd.AddCommand(addCmd, lsCmd, readCmd, rmCmd)
	return cmd
}
