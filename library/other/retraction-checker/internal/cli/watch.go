// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-authored novel feature for retraction-checker-pp-cli.

package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

type watchNotice struct {
	DOI         string `json:"doi"`
	Title       string `json:"title,omitempty"`
	RetractedTo string `json:"retracted_doi,omitempty"`
	Date        string `json:"date,omitempty"`
}

type watchBaseline struct {
	Query     string   `json:"query"`
	UpdatedAt string   `json:"updated_at"`
	Seen      []string `json:"seen"`
}

type watchOutput struct {
	Query        string        `json:"query"`
	FirstRun     bool          `json:"first_run"`
	BaselineDate string        `json:"baseline_date,omitempty"`
	NewCount     int           `json:"new_count"`
	TrackedTotal int           `json:"tracked_total"`
	New          []watchNotice `json:"new"`
	Note         string        `json:"note,omitempty"`
}

func watchDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		base = os.TempDir()
	}
	dir := filepath.Join(base, "retraction-checker-pp-cli", "watch")
	return dir, os.MkdirAll(dir, 0o755)
}

func watchPath(query string) (string, error) {
	dir, err := watchDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(query))
	return filepath.Join(dir, hex.EncodeToString(sum[:8])+".json"), nil
}

func loadWatchBaseline(path string) watchBaseline {
	var b watchBaseline
	data, err := os.ReadFile(path)
	if err != nil {
		return b
	}
	_ = json.Unmarshal(data, &b)
	return b
}

func saveWatchBaseline(path string, b watchBaseline) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// fetchRetractionNotices lists recent retraction notices for a topic query.
func fetchRetractionNotices(cmd *cobra.Command, flags *rootFlags, mailto, query string, rows int) ([]watchNotice, error) {
	ctx, cancel := boundCtx(cmd.Context(), flags)
	defer cancel()
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	params := map[string]string{
		"filter": "update-type:retraction",
		"sort":   "updated",
		"order":  "desc",
		"rows":   fmt.Sprintf("%d", rows),
		"select": "DOI,title,update-to",
	}
	if query != "" {
		params["query"] = query
	}
	if mailto != "" {
		params["mailto"] = mailto
	}
	raw, err := c.Get(ctx, "/works", params)
	if err != nil {
		return nil, err
	}
	var envelope struct {
		Message struct {
			Items []crossrefWorkMessage `json:"items"`
		} `json:"message"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	notices := make([]watchNotice, 0, len(envelope.Message.Items))
	for _, it := range envelope.Message.Items {
		n := watchNotice{DOI: it.DOI}
		if len(it.Title) > 0 {
			n.Title = it.Title[0]
		}
		if len(it.UpdateTo) > 0 {
			n.RetractedTo = it.UpdateTo[0].DOI
			n.Date = it.UpdateTo[0].Updated.iso()
		}
		notices = append(notices, n)
	}
	return notices, nil
}

func newNovelWatchCmd(flags *rootFlags) *cobra.Command {
	var (
		mailto string
		rows   int
		reset  bool
	)
	cmd := &cobra.Command{
		Use:   "watch <topic>",
		Short: "Monitor a topic or reading list for newly-announced retractions since the last run.",
		Long: "Persist a baseline of retraction notices for a topic and, on each subsequent run,\n" +
			"report notices that are new since the baseline. The first run establishes the\n" +
			"baseline and reports nothing as new. Use --reset to clear the stored baseline.\n" +
			"State is kept under your user config directory. Keyless.",
		Example:     "  retraction-checker-pp-cli watch \"machine learning\" --json",
		Args:        cobra.ArbitraryArgs,
		// watch's positional arg is a free-text search topic, not an ID: any
		// string is a valid topic that returns an empty baseline on first run,
		// so there is no "invalid argument" that should exit non-zero. Exempt
		// it from the dogfood error-path probe (see SKILL.md dogfood opt-out).
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return nil
			}
			if len(args) == 0 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("a topic argument is required"))
			}
			query := args[0]
			path, err := watchPath(query)
			if err != nil {
				return err
			}
			if reset {
				_ = os.Remove(path)
			}
			base := loadWatchBaseline(path)
			firstRun := base.UpdatedAt == ""

			notices, err := fetchRetractionNotices(cmd, flags, mailto, query, rows)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			seenSet := map[string]bool{}
			for _, d := range base.Seen {
				seenSet[d] = true
			}
			out := watchOutput{Query: query, FirstRun: firstRun, BaselineDate: base.UpdatedAt, New: make([]watchNotice, 0)}
			allSeen := make([]string, 0, len(notices))
			for _, n := range notices {
				allSeen = append(allSeen, n.DOI)
				if !firstRun && !seenSet[n.DOI] {
					out.New = append(out.New, n)
				}
			}
			out.NewCount = len(out.New)
			out.TrackedTotal = len(notices)
			if firstRun {
				out.Note = fmt.Sprintf("baseline established with %d notices; new retractions will be reported on the next run", len(notices))
			}

			// Persist the union of previously-seen and currently-seen notices.
			for _, d := range allSeen {
				seenSet[d] = true
			}
			merged := make([]string, 0, len(seenSet))
			for d := range seenSet {
				merged = append(merged, d)
			}
			if err := saveWatchBaseline(path, watchBaseline{
				Query:     query,
				UpdatedAt: time.Now().UTC().Format(time.RFC3339),
				Seen:      merged,
			}); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not save watch baseline: %v\n", err)
			}

			if flags.asJSON || flags.agent || !isTerminal(cmd.OutOrStdout()) {
				return printJSONFiltered(cmd.OutOrStdout(), out, flags)
			}
			if firstRun {
				fmt.Fprintf(cmd.OutOrStdout(), "Baseline established for %q: %d notices tracked.\n", query, out.TrackedTotal)
				return nil
			}
			if out.NewCount == 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "No new retractions for %q since %s.\n", query, base.UpdatedAt)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d new retraction(s) for %q:\n\n", out.NewCount, query)
			for _, n := range out.New {
				fmt.Fprintf(cmd.OutOrStdout(), "  %s  %s\n    %s\n", n.Date, n.Title, n.DOI)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&mailto, "mailto", "", "Contact email for the Crossref polite pool (better rate limits)")
	cmd.Flags().IntVar(&rows, "rows", 50, "Number of recent retraction notices to track")
	cmd.Flags().BoolVar(&reset, "reset", false, "Clear the stored baseline for this topic before running")
	return cmd
}
