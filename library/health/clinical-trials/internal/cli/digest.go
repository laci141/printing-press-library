// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.

// digest.go implements the `digest` command: a compact "what's notable" snapshot
// for a condition or free-text search term. It reports total trials found,
// recruiting count, the newest N trials, and any recently-terminated ones —
// using only ClinicalTrials.gov registry signals.
//
// This is a read-only point-in-time snapshot. It does not persist state and
// does not diff against a previous run (see `watch` for change-feed behaviour).
package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// digestTrial is a condensed trial row in the digest output.
type digestTrial struct {
	NCTID      string `json:"id"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	Phase      string `json:"phase,omitempty"`
	Sponsor    string `json:"sponsor,omitempty"`
	LastUpdate string `json:"last_update,omitempty"`
}

// digestView is the JSON shape `digest` emits.
type digestView struct {
	Term        string        `json:"term"`
	Total       int           `json:"total"`
	Recruiting  int           `json:"recruiting"`
	Newest      []digestTrial `json:"newest,omitempty"`
	RecentlyTerminated []digestTrial `json:"recently_terminated,omitempty"`
	Note        string        `json:"note,omitempty"`
}

// pp:data-source live
func newNovelDigestCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "digest <term>",
		Short: "Compact 'what's notable' snapshot for a condition or search term: total, recruiting count, newest trials, recently-terminated ones.",
		Long: "Fetches trials matching a condition or free-text term and produces a compact\n" +
			"digest: total matched, how many are actively recruiting, the N most recently\n" +
			"updated trials (newest first), and any that have been terminated or withdrawn.\n\n" +
			"This is a READ-ONLY point-in-time snapshot. No state is stored between runs;\n" +
			"use the `watch` command for a persistent change feed.",
		Example: "  clinical-trials-pp-cli digest \"type 2 diabetes\"\n" +
			"  clinical-trials-pp-cli digest alzheimer --limit 5 --json",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if limit < 1 {
				return usageErr(fmt.Errorf("--limit must be at least 1"))
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would produce a condition digest from ClinicalTrials.gov")
				return nil
			}
			term := strings.TrimSpace(strings.Join(args, " "))
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Fetch up to 200 trials for the term; one page is enough for a digest.
			trials, err := ctgovFetch(ctx, c, ctgovParams("term", term), 200, 1)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			if len(trials) == 0 {
				return noResultsErr(term)
			}

			// Partition: all trials, recruiting subset, terminated subset.
			recruitingCount := 0
			var terminated []Trial
			for _, t := range trials {
				upper := strings.ToUpper(t.Status)
				if upper == "RECRUITING" || upper == "ENROLLING_BY_INVITATION" {
					recruitingCount++
				}
				if isTerminalStatus(t.Status) {
					terminated = append(terminated, t)
				}
			}

			// Sort all trials by LastUpdate descending to find newest.
			sorted := make([]Trial, len(trials))
			copy(sorted, trials)
			sort.SliceStable(sorted, func(i, j int) bool {
				return sorted[i].LastUpdate > sorted[j].LastUpdate
			})

			// Newest N (up to limit).
			n := limit
			if n > len(sorted) {
				n = len(sorted)
			}
			newest := make([]digestTrial, 0, n)
			for _, t := range sorted[:n] {
				newest = append(newest, trialToDigest(t))
			}

			// Recently-terminated: sort terminated by LastUpdate descending, take up to limit.
			sort.SliceStable(terminated, func(i, j int) bool {
				return terminated[i].LastUpdate > terminated[j].LastUpdate
			})
			nt := limit
			if nt > len(terminated) {
				nt = len(terminated)
			}
			recentlyTerminated := make([]digestTrial, 0, nt)
			for _, t := range terminated[:nt] {
				recentlyTerminated = append(recentlyTerminated, trialToDigest(t))
			}

			note := ""
			if len(trials) == 200 {
				note = "result set capped at 200 trials; totals reflect the first 200 matched"
			}

			view := digestView{
				Term:               term,
				Total:              len(trials),
				Recruiting:         recruitingCount,
				Newest:             newest,
				RecentlyTerminated: recentlyTerminated,
				Note:               note,
			}

			if flags.asJSON || !isTerminal(cmd.OutOrStdout()) {
				return printJSONFiltered(cmd.OutOrStdout(), view, flags)
			}
			return renderDigestHuman(cmd, view)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 5, "Max trials to show in newest and recently-terminated lists (min 1)")
	return cmd
}

func trialToDigest(t Trial) digestTrial {
	return digestTrial{
		NCTID:      t.NCTID,
		Title:      t.Title,
		Status:     t.Status,
		Phase:      t.Phase,
		Sponsor:    t.Sponsor,
		LastUpdate: t.LastUpdate,
	}
}

func renderDigestHuman(cmd *cobra.Command, v digestView) error {
	w := cmd.OutOrStdout()
	fmt.Fprintf(w, "Digest: %q\n", v.Term)
	fmt.Fprintf(w, "  Total matched: %d    Recruiting: %d\n\n", v.Total, v.Recruiting)

	if len(v.Newest) > 0 {
		fmt.Fprintln(w, "Newest (by last update):")
		for _, t := range v.Newest {
			fmt.Fprintf(w, "  %-14s %-18s %-10s %s\n",
				t.NCTID, truncate(t.Status, 18), truncate(t.Phase, 10), truncate(t.Title, 60))
		}
		fmt.Fprintln(w)
	}

	if len(v.RecentlyTerminated) > 0 {
		fmt.Fprintln(w, "Recently terminated/withdrawn:")
		for _, t := range v.RecentlyTerminated {
			fmt.Fprintf(w, "  %-14s %-18s %s\n",
				t.NCTID, truncate(t.Status, 18), truncate(t.Title, 60))
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "No recently terminated/withdrawn trials in this result set.")
	}

	if v.Note != "" {
		fmt.Fprintf(w, "note: %s\n", v.Note)
	}
	return nil
}
