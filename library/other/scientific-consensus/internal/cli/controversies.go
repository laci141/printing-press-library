// Hand-authored novel command: controversy detector. Not generated.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
	"github.com/spf13/cobra"
)

type controversyOutput struct {
	Query            string      `json:"query"`
	StudyCount       int         `json:"study_count"`
	Supporting       int         `json:"supporting"`
	Refuting         int         `json:"refuting"`
	Mixed            int         `json:"mixed"`
	ControversyScore float64     `json:"controversy_score"` // 0 (settled) .. 1 (highly contested)
	Contested        bool        `json:"contested"`
	SupportingSide   []workBrief `json:"supporting_side"`
	RefutingSide     []workBrief `json:"refuting_side"`

	// Exclusion ledger. fetched is what OpenAlex returned; the three counts
	// and StudyCount must sum back to it, so a reader can audit where every
	// work went instead of wondering why 10 became 3.
	Fetched           int      `json:"fetched"`
	RelevanceExcluded int      `json:"relevance_excluded"`
	PICOExcluded      int      `json:"pico_excluded"`
	DuplicateExcluded int      `json:"duplicate_excluded"`
	DuplicateFamilies []string `json:"duplicate_families,omitempty"`

	Note string `json:"note,omitempty"`
}

func newNovelControversiesCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var enrich bool

	cmd := &cobra.Command{
		Use:   "controversies <query>",
		Short: "Surface conflicting studies and contradictory conclusions for a topic",
		Long: "Classify the stance of each retrieved study and measure how contested the topic\n" +
			"is. A high controversy score means supporting and refuting evidence are both\n" +
			"substantial. Use this to answer 'is this settled or disputed?'",
		Example:     "  scientific-consensus-pp-cli controversies \"saturated fat heart disease\" --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would detect controversies for the topic")
				return nil
			}
			query, err := requireQuery(args)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}

			// Same narrowing switch as consensus. Measured 2026-08-05: the
			// fulltext path returns works that merely mention the terms
			// somewhere in the body, and this command classifies stance, so
			// an off-topic work does not just pad a count — it votes.
			search := query
			filter := ""
			if narrowSearchEnabled() {
				if narrowed := buildNarrowedSearch(query); narrowed != "" {
					filter = "title_and_abstract.search:" + narrowed
					search = ""
				}
			}
			works, _, err := fetchWorks(ctx, c, search, filter, "relevance_score:desc", limit)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			fetched := len(works)

			// The four gates below are the consensus pipeline, in the same
			// order and with the same calls. Measured 2026-08-05 on "HPV
			// vaccine causes infertility": with no gates at all, this command
			// classified a study of vaccine ACCEPTABILITY in Kenya and a study
			// of HPV INFECTION and sperm as refuting at 0.90 confidence each.
			// Only the AI filter caught them, and the AI is optional.
			//
			// Two commands disagreeing about which works count is worse than
			// no gate at all, which is why this is a copy of the pipeline
			// rather than a variation on it.
			backfillAbstracts(ctx, query, works)

			works, relReport := filterRelevantReport(query, works)
			relevanceExcluded := fetched - len(works)

			ivTokens, outTokens := scengine.PICOTokens(query)
			picoRelevant := make([]scWork, 0, len(works))
			for _, w := range works {
				if scengine.IsPICORelevant(w.Abstract, w.Title, ivTokens, outTokens) {
					picoRelevant = append(picoRelevant, w)
				}
			}
			picoExcluded := len(works) - len(picoRelevant)
			works = picoRelevant

			works, dedupeRep := dedupeVersions(works)

			if enrich {
				enrichPubTypes(ctx, works, 50)
			}
			prog := newProgress(flags, "analyzing works", len(works))
			prog.update(len(works))
			_, stances := scoreWorks(ctx, works, query)
			prog.done()

			out := controversyOutput{
				Query:             query,
				StudyCount:        len(works),
				Fetched:           fetched,
				RelevanceExcluded: relevanceExcluded,
				PICOExcluded:      picoExcluded,
				DuplicateExcluded: dedupeRep.Excluded,
			}
			for _, s := range stances {
				switch s.Stance {
				case scengine.StanceSupporting:
					out.Supporting++
				case scengine.StanceRefuting:
					out.Refuting++
				case scengine.StanceMixed:
					out.Mixed++
				}
			}
			directional := out.Supporting + out.Refuting
			minor := out.Supporting
			if out.Refuting < minor {
				minor = out.Refuting
			}
			if directional > 0 {
				// 2*minority/total => 1.0 when evenly split, 0 when one-sided.
				out.ControversyScore = round2f(2 * float64(minor) / float64(directional))
			}
			// A minority of one is not a controversy. Measured 2026-08-05 on
			// "HPV vaccine causes infertility": supporting 1, refuting 3, so
			// 2*1/4 lands exactly on 0.5 and directional lands exactly on 4 —
			// both thresholds met at their boundary by a single work, and that
			// work was the social-media sentiment study the AI filter then
			// excluded as off-topic. One misclassification was enough to label
			// a settled question contested, on the one topic where that label
			// does real harm.
			//
			// Measured across 8 live harm claims: this changes HPV only. cell
			// phones (2/3), aspartame (3/2) and red meat (4/4) all keep
			// contested; vaccines/autism (1/6), statins (1/5) and 5G (0/6)
			// were already false. Across the 12 archived corpora it changes
			// nothing at all — no archived corpus has a minority of exactly
			// one above the score threshold.
			//
			// Two is the floor rather than three because redmeat sits at 5/2
			// and is genuinely contested; a floor of three would erase it.
			out.Contested = out.ControversyScore >= 0.5 && directional >= 4 && minor >= 2

			out.SupportingSide = topByStance(stances, scengine.StanceSupporting, 3)
			out.RefutingSide = topByStance(stances, scengine.StanceRefuting, 3)

			out.Note = controversyNote(out, relReport)

			return emit(cmd, flags, out, func(w io.Writer) { renderControversy(w, out) })
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "number of works to analyze (max 200)")
	cmd.Flags().BoolVar(&enrich, "enrich", true, "enrich classification with PubMed publication types")
	return cmd
}

// controversyNote states what the gates removed and whether the surviving
// corpus can carry a controversy verdict at all.
//
// The exclusion sentence comes first and is unconditional whenever anything
// was dropped. A reader who sees "Studies: 3" after asking for 10 is owed the
// reason in the same breath, not in a separate panel they may not open.
func controversyNote(o controversyOutput, rel relevanceReport) string {
	var parts []string

	excluded := o.RelevanceExcluded + o.PICOExcluded + o.DuplicateExcluded
	if excluded > 0 {
		parts = append(parts, fmt.Sprintf(
			"%d of %d fetched works excluded before scoring (%d off-topic, %d failed the intervention/outcome gate, %d superseded editions)",
			excluded, o.Fetched, o.RelevanceExcluded, o.PICOExcluded, o.DuplicateExcluded))
	}
	if len(o.DuplicateFamilies) > 0 {
		parts = append(parts, "collapsed editions: "+strings.Join(o.DuplicateFamilies, "; "))
	}

	directional := o.Supporting + o.Refuting
	switch {
	case o.StudyCount == 0:
		parts = append(parts, "no works survived the relevance gates; try a broader query")
	case directional < 4:
		parts = append(parts, "too few directional studies to assess controversy reliably")
	}

	return strings.Join(parts, "; ")
}

func renderControversy(w io.Writer, o controversyOutput) {
	fmt.Fprintf(w, "Controversy analysis: %s\n\n", o.Query)
	verdict := "settled / one-sided"
	if o.Contested {
		verdict = "CONTESTED"
	}
	fmt.Fprintf(w, "  Controversy score: %.2f  (%s)\n", o.ControversyScore, verdict)
	fmt.Fprintf(w, "  Studies: %d  (support %d / refute %d / mixed %d)\n", o.StudyCount, o.Supporting, o.Refuting, o.Mixed)
	if o.Fetched != o.StudyCount {
		fmt.Fprintf(w, "  Fetched %d, excluded %d off-topic / %d PICO / %d duplicate\n",
			o.Fetched, o.RelevanceExcluded, o.PICOExcluded, o.DuplicateExcluded)
	}
	fmt.Fprintln(w)
	if len(o.SupportingSide) > 0 {
		fmt.Fprintln(w, "  Supporting side:")
		for _, b := range o.SupportingSide {
			fmt.Fprintf(w, "    • [%d, cites=%d] %s\n", b.Year, b.CitedBy, truncate(b.Title, 78))
		}
	}
	if len(o.RefutingSide) > 0 {
		fmt.Fprintln(w, "  Refuting side:")
		for _, b := range o.RefutingSide {
			fmt.Fprintf(w, "    • [%d, cites=%d] %s\n", b.Year, b.CitedBy, truncate(b.Title, 78))
		}
	}
	if o.Note != "" {
		fmt.Fprintf(w, "\n  Note: %s\n", o.Note)
	}
}

func round2f(v float64) float64 { return float64(int(v*100+0.5)) / 100 }
