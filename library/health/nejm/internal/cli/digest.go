// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Phase 3: current-issue reading digest grouped by specialty or type.

package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var groupBy string
	var limit int

	cmd := &cobra.Command{
		Use:         "digest",
		Short:       "Triage the current issue grouped by specialty or type with abstracts and free/paywalled flags.",
		Long:        "Groups current-issue articles by specialty (default) or article type, showing title, authors, abstract snippet, and free/paywalled flag. Requires synced data; enriched articles show richer grouping.",
		Annotations: map[string]string{"mcp:read-only": "true"},
		Example:     "  nejm-pp-cli digest\n  nejm-pp-cli digest --group type --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return nil
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			db, err := nejmOpenStore(ctx)
			if err != nil {
				return err
			}
			defer db.Close()
			maybeEmitSyncHints(cmd, db, "article", flags.maxAge)

			items, err := nejmQueryArticles(db, `feed = 'etoc'`, nil, limit)
			if err != nil {
				return fmt.Errorf("querying articles: %w", err)
			}
			if len(items) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "hint: no current-issue articles found. Run 'nejm-pp-cli sync' to fetch the current issue.")
				return nil
			}

			// Group by specialty or article_type
			groupField := "specialties"
			if strings.ToLower(groupBy) == "type" {
				groupField = "article_type"
			}

			groups := make(map[string][]map[string]any)
			var groupOrder []string
			seen := make(map[string]bool)

			for _, item := range items {
				key, _ := item[groupField].(string)
				if key == "" {
					key = "(ungrouped)"
				}
				if !seen[key] {
					groupOrder = append(groupOrder, key)
					seen[key] = true
				}
				// Trim abstract to 200 chars for digest view
				if abs, ok := item["abstract"].(string); ok && len(abs) > 200 {
					item["abstract"] = abs[:197] + "..."
				}
				groups[key] = append(groups[key], item)
			}

			type digestGroup struct {
				Group    string           `json:"group"`
				Count    int              `json:"count"`
				Articles []map[string]any `json:"articles"`
			}
			var result []digestGroup
			for _, key := range groupOrder {
				result = append(result, digestGroup{
					Group:    key,
					Count:    len(groups[key]),
					Articles: groups[key],
				})
			}

			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage(data), flags)
		},
	}
	cmd.Flags().StringVar(&groupBy, "group", "specialty", "Group by: specialty or type")
	cmd.Flags().IntVar(&limit, "limit", 0, "Maximum number of articles (0 = all)")
	return cmd
}
