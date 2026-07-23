// Hand-authored novel command: side-by-side claim comparison. Not generated.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
	"github.com/spf13/cobra"
)

type compareOutput struct {
	ClaimA   consensusOutput `json:"claim_a"`
	ClaimB   consensusOutput `json:"claim_b"`
	Stronger string          `json:"stronger_support"`
	Note     string          `json:"note,omitempty"`
}

// computeConsensus runs the full fetch -> filter -> classify -> score ->
// consensus pass for one claim. Shared by `compare` and `batch`.
//
// The relevance gate and the low-evidence safety guard are applied here for the
// same reason the `consensus` command applies them: without an LLM the only
// relevance signal is lexical, so off-topic works must be dropped before they
// reach the score, and a corpus too thin to survive that gate must not be
// certified by a study-design strength label. Before this, compare and batch
// scored EVERY fetched work — the exact condition that let a false claim
// (vaccines/autism) come back "supported" with a confident-looking badge.
func computeConsensus(ctx context.Context, c apiGetter, claim string, limit, yearFrom int, enrich bool) (consensusOutput, error) {
	filter := ""
	if yearFrom > 0 {
		filter = fmt.Sprintf("from_publication_date:%d-01-01", yearFrom)
	}
	works, _, err := fetchWorks(ctx, c, claim, filter, "relevance_score:desc", limit)
	if err != nil {
		return consensusOutput{}, err
	}

	// Relevance gate, before enrichment so excluded works cost no PubMed
	// lookups and never enter the score.
	fetched := len(works)
	works = filterRelevant(claim, works)
	dropped := fetched - len(works)

	// Phase 3 PICO relevance gate, same wiring as the `consensus` command: a
	// work must name the intervention AND the outcome to count as evidence
	// about the relation the claim asserts. compare and batch have to measure
	// the same subset the consensus command does, or the two commands report
	// different corpora — and confidence, which is computed from that subset,
	// would not mean the same thing across them.
	ivTokens, outTokens := scengine.PICOTokens(claim)
	picoRelevant := make([]scWork, 0, len(works))
	for _, w := range works {
		if scengine.IsPICORelevant(w.Abstract, w.Title, ivTokens, outTokens) {
			picoRelevant = append(picoRelevant, w)
		}
	}
	picoDropped := len(works) - len(picoRelevant)
	works = picoRelevant
	relevantCount := len(works)

	if enrich {
		enrichPubTypes(ctx, works, 50)
	}
	scored, stances := scoreWorks(ctx, works, claim)
	r := scengine.Consensus(scored)

	method := stanceMethodLabel(stances)
	measuredStrength := r.EvidenceStrength
	r = scengine.ApplyLowEvidenceGuard(r, method, relevantCount)
	evidenceGuarded := r.EvidenceStrength != measuredStrength

	out := consensusOutput{
		Claim: claim, Verdict: r.Verdict, ConsensusScore: r.ConsensusScore, Confidence: r.Confidence,
		EvidenceStrength: r.EvidenceStrength, ApexDesign: r.ApexDesign, StudyCount: r.StudyCount,
		Supporting: r.Supporting, Refuting: r.Refuting, Mixed: r.Mixed, Inconclusive: r.Inconclusive,
		TotalCitations: r.TotalCitations, Method: method,
		RelevantCount:   relevantCount,
		NearUnanimous:   r.NearUnanimous,
		EvidenceGuarded: evidenceGuarded,
		TopSupporting:   topByStance(stances, scengine.StanceSupporting, 2),
		TopRefuting:     topByStance(stances, scengine.StanceRefuting, 2),
		AllStudies:      allStudyBriefs(stances),
	}
	if dropped > 0 {
		out.Note = appendNote(out.Note, fmt.Sprintf("%d off-topic work(s) excluded by relevance gate", dropped))
	}
	if picoDropped > 0 {
		out.Note = appendNote(out.Note, fmt.Sprintf(
			"%d work(s) excluded by PICO gate (missing intervention %v or outcome %v in abstract/title)",
			picoDropped, ivTokens, outTokens))
	}
	if evidenceGuarded {
		out.Note = appendNote(out.Note, fmt.Sprintf(
			"evidence strength forced to %q: keyless run with only %d relevant work(s) (threshold %d) — no AI relevance filtering ran, so listed studies may be off-topic",
			scengine.StrengthInsufficient, relevantCount, scengine.LowEvidenceThreshold))
	}
	if r.NearUnanimous {
		out.Note = appendNote(out.Note,
			"near-unanimous result (no refuting or mixed studies) — check that genuine debate was not filtered out")
	}
	return out, nil
}

func newNovelCompareCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var enrich bool

	cmd := &cobra.Command{
		Use:   "compare <claim-a> <claim-b>",
		Short: "Run two consensus analyses side-by-side to compare competing claims",
		Long: "Run the consensus engine for two claims and present them side by side, with the\n" +
			"more strongly supported claim flagged. Use this to weigh competing interventions\n" +
			"or contradictory claims.\n\n" +
			"Each claim goes through the same relevance gate and low-evidence safety guard as\n" +
			"the `consensus` command: off-topic works are dropped before scoring, and on a\n" +
			"keyless run with fewer than 5 relevant works evidence_strength is forced to\n" +
			"\"insufficient\" (evidence_guarded is set). A comparison where either side is\n" +
			"guarded is not a fair comparison — check relevant_count on both.",
		Example:     "  scientific-consensus-pp-cli compare \"statins reduce mortality\" \"statins increase diabetes risk\" --agent",
		Annotations: map[string]string{"mcp:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would compare two claims")
				return nil
			}
			if len(args) < 2 {
				_ = cmd.Usage()
				return usageErr(fmt.Errorf("two claim arguments are required"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			prog := newProgress(flags, "analyzing claims", 2)
			prog.update(1)
			a, err := computeConsensus(ctx, c, args[0], limit, 0, enrich)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			prog.update(2)
			b, err := computeConsensus(ctx, c, args[1], limit, 0, enrich)
			if err != nil {
				return classifyAPIError(err, flags)
			}
			prog.done()
			out := compareOutput{ClaimA: a, ClaimB: b}
			switch {
			case a.ConsensusScore > b.ConsensusScore+0.05:
				out.Stronger = a.Claim
			case b.ConsensusScore > a.ConsensusScore+0.05:
				out.Stronger = b.Claim
			default:
				out.Stronger = "comparable"
			}
			if a.StudyCount == 0 || b.StudyCount == 0 {
				out.Note = appendNote(out.Note, "one or both claims returned no works; comparison may be unreliable")
			}
			// A side-by-side verdict is only meaningful when both sides rest on
			// a corpus the guard did not veto; say so rather than letting the
			// "stronger support" line imply a fair fight.
			if a.EvidenceGuarded || b.EvidenceGuarded {
				out.Note = appendNote(out.Note,
					"low-evidence guard fired on at least one claim; the side-by-side comparison is not reliable")
			}
			return emit(cmd, flags, out, func(w io.Writer) { renderCompare(w, out) })
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 40, "number of works to analyze per claim (max 200)")
	cmd.Flags().BoolVar(&enrich, "enrich", true, "enrich classification with PubMed publication types")
	return cmd
}

// guardedLabel renders an evidence-strength cell, marking the value when the
// low-evidence guard produced it rather than the strength ladder.
func guardedLabel(o consensusOutput) string {
	if o.EvidenceGuarded {
		return string(o.EvidenceStrength) + " ⚠guarded"
	}
	return string(o.EvidenceStrength)
}

func renderCompare(w io.Writer, o compareOutput) {
	row := func(label, av, bv string) { fmt.Fprintf(w, "  %-18s %-32s %-32s\n", label, av, bv) }
	fmt.Fprintln(w, "Claim comparison:")
	row("", truncate(o.ClaimA.Claim, 30), truncate(o.ClaimB.Claim, 30))
	row("verdict", string(o.ClaimA.Verdict), string(o.ClaimB.Verdict))
	row("consensus score", fmt.Sprintf("%+.2f", o.ClaimA.ConsensusScore), fmt.Sprintf("%+.2f", o.ClaimB.ConsensusScore))
	row("confidence", fmt.Sprintf("%.0f%%", o.ClaimA.Confidence*100), fmt.Sprintf("%.0f%%", o.ClaimB.Confidence*100))
	row("evidence strength", guardedLabel(o.ClaimA), guardedLabel(o.ClaimB))
	row("studies", fmt.Sprintf("%d", o.ClaimA.StudyCount), fmt.Sprintf("%d", o.ClaimB.StudyCount))
	row("relevant works", fmt.Sprintf("%d", o.ClaimA.RelevantCount), fmt.Sprintf("%d", o.ClaimB.RelevantCount))
	fmt.Fprintf(w, "\n  Stronger support: %s\n", o.Stronger)
	if o.Note != "" {
		fmt.Fprintf(w, "  Note: %s\n", o.Note)
	}
}
