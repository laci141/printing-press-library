// Hand-authored novel command: consensus engine. Not generated.
package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
	"github.com/spf13/cobra"
)

type workBrief struct {
	Title      string          `json:"title"`
	Year       int             `json:"year,omitempty"`
	DOI        string          `json:"doi,omitempty"`
	CitedBy    int             `json:"cited_by_count"`
	Design     scengine.Design `json:"design"`
	Stance     scengine.Stance `json:"stance"`
	StanceConf float64         `json:"stance_confidence"`
	// Abstract is the reconstructed OpenAlex abstract, capped at
	// maxAbstractChars so downstream LLM prompts built from this JSON stay
	// bounded. Empty string when the source has no abstract.
	Abstract string `json:"abstract"`
}

type consensusOutput struct {
	Claim            string                    `json:"claim"`
	Verdict          scengine.Verdict          `json:"verdict"`
	ConsensusScore   float64                   `json:"consensus_score"`
	Confidence       float64                   `json:"confidence"`
	EvidenceStrength scengine.EvidenceStrength `json:"evidence_strength"`
	ApexDesign       scengine.Design           `json:"apex_design"`
	StudyCount       int                       `json:"study_count"`
	Supporting       int                       `json:"supporting"`
	Refuting         int                       `json:"refuting"`
	Mixed            int                       `json:"mixed"`
	Inconclusive     int                       `json:"inconclusive"`
	TotalCitations   int                       `json:"total_citations"`
	Method           string                    `json:"stance_method"`
	// RelevantCount is how many fetched works survived the lexical relevance
	// gate and therefore entered scoring. It is the input to the low-evidence
	// safety guard, and consumers need it to judge how thin the corpus was.
	RelevantCount int `json:"relevant_count"`
	// NearUnanimous mirrors scengine.ConsensusResult.NearUnanimous: the result
	// is so one-sided that real dissent was probably filtered out upstream.
	NearUnanimous bool `json:"near_unanimous"`
	// EvidenceGuarded reports that the low-evidence safety guard fired and
	// overwrote EvidenceStrength with "insufficient". Without this flag a
	// consumer cannot tell a guarded label from a measured one.
	EvidenceGuarded bool        `json:"evidence_guarded"`
	TopSupporting   []workBrief `json:"top_supporting"`
	TopRefuting     []workBrief `json:"top_refuting"`
	// AllStudies lists every analyzed work (post relevance gate) in fetch
	// (relevance) order, so content-aware consumers can re-filter by
	// abstract instead of trusting the top lists alone.
	AllStudies []workBrief `json:"all_studies"`
	Note       string      `json:"note,omitempty"`
}

// maxAbstractChars bounds per-study abstract length in JSON output.
const maxAbstractChars = 1500

// clipAbstract caps an abstract at maxAbstractChars characters, cutting on a
// rune boundary so multi-byte text is never split mid-character.
func clipAbstract(s string) string {
	if len(s) <= maxAbstractChars {
		return s
	}
	r := []rune(s)
	if len(r) <= maxAbstractChars {
		return s
	}
	return string(r[:maxAbstractChars])
}

func newNovelConsensusCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var yearFrom int
	var enrich bool

	cmd := &cobra.Command{
		Use:   "consensus <claim>",
		Short: "Answer 'what does the evidence say about X' with a consensus score across sources",
		Long: "Fetch the most relevant works for a claim, classify each study's design and\n" +
			"stance, and compute a tier- and citation-weighted Consensus Score, Confidence,\n" +
			"and Evidence Strength. Stance is heuristic without an AI key. Do NOT treat the\n" +
			"score as a peer-reviewed conclusion; use `evidence` to inspect study designs.\n\n" +
			"Safety guard: on a keyless (heuristic) run, when fewer than 5 works survive the\n" +
			"relevance gate, evidence_strength is forced to \"insufficient\" and\n" +
			"evidence_guarded is set. Study design alone must not certify a corpus that thin.\n\n" +
			"Optional LLM-assisted stance classification: set one of these API keys (checked\n" +
			"in this priority order, first one set wins) to classify stance with that model\n" +
			"instead of the lexical heuristic:\n" +
			"  1. ANTHROPIC_API_KEY  (claude-haiku-4-5-20251001)\n" +
			"  2. OPENAI_API_KEY     (gpt-4o-mini)\n" +
			"  3. GEMINI_API_KEY     (gemini-2.0-flash)\n" +
			"  4. GROQ_API_KEY       (llama-3.3-70b-versatile)\n" +
			"  5. MISTRAL_API_KEY    (mistral-small-latest)\n" +
			"The LLM path is best-effort: a single attempt with a 15s timeout, and any error\n" +
			"silently falls back to the heuristic. The method used is reported as\n" +
			"stance_method (heuristic or llm:<provider>).",
		Example:     "  scientific-consensus-pp-cli consensus \"vitamin D reduces respiratory infections\" --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:no-error-path-probe": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 && cmd.Flags().NFlag() == 0 {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				fmt.Fprintln(cmd.OutOrStdout(), "would analyze consensus for the claim")
				return nil
			}
			claim, err := requireQuery(args)
			if err != nil {
				return err
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()

			c, err := flags.newClient()
			if err != nil {
				return err
			}
			filter := ""
			if yearFrom > 0 {
				filter = fmt.Sprintf("from_publication_date:%d-01-01", yearFrom)
			}
			works, _, err := fetchWorks(ctx, c, claim, filter, "relevance_score:desc", limit)
			if err != nil {
				return classifyAPIError(err, flags)
			}

			// Relevance gate: drop works whose title/topic share no content
			// token with the claim, before enrichment so excluded works cost
			// no PubMed lookups and never enter the score.
			fetched := len(works)
			works = filterRelevant(claim, works)
			dropped := fetched - len(works)

			// --- Phase 3: PICO relevance gate ---
			// The lexical gate above keeps a work that shares ANY content token
			// with the claim. That is too weak for a two-sided claim: a paper
			// about vaccine schedules with no mention of autism passes it. The
			// PICO gate additionally requires the intervention AND the outcome
			// to both appear in the abstract or title, so what survives is
			// evidence about the relation the claim asserts, not about one half
			// of it. Runs before enrichment and scoring so excluded works cost
			// no PubMed lookups and never reach the score.
			ivTokens, outTokens := scengine.PICOTokens(claim)
			picoRelevant := make([]scWork, 0, len(works))
			for _, w := range works {
				if scengine.IsPICORelevant(w.Abstract, w.Title, ivTokens, outTokens) {
					picoRelevant = append(picoRelevant, w)
				}
			}
			picoDropped := len(works) - len(picoRelevant)
			works = picoRelevant
			// relevantCount is the post-gate corpus size — exactly the set that
			// reaches scoring, and the input to the low-evidence safety guard.
			relevantCount := len(works)

			// No work names both sides of the claim: there is nothing to score,
			// and reporting a verdict computed from zero studies would be worse
			// than reporting none. Emitted as a normal result (not an error) so
			// agent consumers get the machine-readable shape they expect.
			if relevantCount == 0 {
				out := consensusOutput{
					Claim:            claim,
					Verdict:          scengine.VerdictInsufficient,
					EvidenceStrength: scengine.StrengthInsufficient,
					Method:           stanceMethodLabel(nil),
					RelevantCount:    0,
					EvidenceGuarded:  true,
					TopSupporting:    []workBrief{},
					TopRefuting:      []workBrief{},
					AllStudies:       []workBrief{},
					Note: fmt.Sprintf(
						"no works matched both the intervention (%v) and outcome (%v) tokens in abstract/title (PICO gate); consensus not available — try restating the claim with the terms the literature uses",
						ivTokens, outTokens),
				}
				return emit(cmd, flags, out, func(w io.Writer) { renderConsensus(w, out) })
			}

			if enrich {
				enrichPubTypes(ctx, works, 50)
			}

			// Status line on stderr while stance classification runs (which can
			// be slow when an AI key drives per-work LLM calls). Auto-disabled
			// for --json/pipes so machine output stays clean.
			prog := newProgress(flags, "analyzing works", len(works))
			prog.update(len(works))
			scored, stances := scoreWorks(ctx, works, claim)
			prog.done()
			result := scengine.Consensus(scored)

			// Low-evidence safety guard. method is resolved BEFORE the guard
			// because the guard's first question is "did an LLM vet relevance?"
			// — with an LLM the strength ladder is trustworthy, without one a
			// corpus under scengine.LowEvidenceThreshold cannot support any
			// strength label at all. The post-guard label is compared against
			// the measured one so the JSON can report that a guard, not a
			// measurement, produced the value.
			method := stanceMethodLabel(stances)
			measuredStrength := result.EvidenceStrength
			result = scengine.ApplyLowEvidenceGuard(result, method, relevantCount)
			evidenceGuarded := result.EvidenceStrength != measuredStrength

			out := consensusOutput{
				Claim: claim, Verdict: result.Verdict, ConsensusScore: result.ConsensusScore,
				Confidence: result.Confidence, EvidenceStrength: result.EvidenceStrength,
				ApexDesign: result.ApexDesign, StudyCount: result.StudyCount,
				Supporting: result.Supporting, Refuting: result.Refuting, Mixed: result.Mixed,
				Inconclusive: result.Inconclusive, TotalCitations: result.TotalCitations,
				Method:          method,
				RelevantCount:   relevantCount,
				NearUnanimous:   result.NearUnanimous,
				EvidenceGuarded: evidenceGuarded,
			}
			out.TopSupporting = topByStance(stances, scengine.StanceSupporting, 3)
			out.TopRefuting = topByStance(stances, scengine.StanceRefuting, 3)
			out.AllStudies = allStudyBriefs(stances)
			if result.StudyCount == 0 {
				out.Note = "no works found; try a broader claim or --data-source live"
			} else if result.Verdict == scengine.VerdictInsufficient {
				out.Note = "fewer than 3 directional studies; treat as preliminary"
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
			if result.NearUnanimous {
				out.Note = appendNote(out.Note,
					"near-unanimous result (no refuting or mixed studies) — check that genuine debate was not filtered out")
			}

			return emit(cmd, flags, out, func(w io.Writer) { renderConsensus(w, out) })
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 40, "number of works to analyze (max 200)")
	cmd.Flags().IntVar(&yearFrom, "year-from", 0, "only include works published from this year onward")
	cmd.Flags().BoolVar(&enrich, "enrich", true, "enrich study-design classification with PubMed publication types")
	return cmd
}

// appendNote joins note fragments with "; ", so callers can add a fragment
// without repeating the empty-string check at every call site.
func appendNote(note, add string) string {
	if note == "" {
		return add
	}
	return note + "; " + add
}

// stanceMethodLabel summarizes how stance was classified across the analyzed
// works. Without an AI key every work is "heuristic"; with a key configured the
// dispatcher uses the LLM and falls back to heuristic per-work on any error, so
// we report the LLM provider when at least one work was classified by it.
func stanceMethodLabel(stances []workStance) string {
	if len(stances) == 0 {
		// No works classified: reflect the configured upgrade path, if any.
		if name := scengine.LLMProviderName(); name != "" {
			return "llm:" + name
		}
		return "heuristic"
	}
	for _, s := range stances {
		if strings.HasPrefix(s.StanceMethod, "llm:") {
			return s.StanceMethod
		}
	}
	return "heuristic"
}

func topByStance(stances []workStance, stance scengine.Stance, n int) []workBrief {
	matches := make([]workStance, 0)
	for _, s := range stances {
		if s.Stance == stance {
			matches = append(matches, s)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Work.CitedBy > matches[j].Work.CitedBy })
	if len(matches) > n {
		matches = matches[:n]
	}
	out := make([]workBrief, 0, len(matches))
	for _, m := range matches {
		out = append(out, workBrief{
			Title: m.Work.Title, Year: m.Work.Year, DOI: m.Work.DOI, CitedBy: m.Work.CitedBy,
			Design: m.Design, Stance: m.Stance, StanceConf: m.Confidence,
			Abstract: clipAbstract(m.Work.Abstract),
		})
	}
	return out
}

// allStudyBriefs converts every analyzed work into a workBrief, preserving the
// input (relevance) order. Always returns a non-nil slice so JSON emits [] for
// zero studies rather than null.
func allStudyBriefs(stances []workStance) []workBrief {
	out := make([]workBrief, 0, len(stances))
	for _, s := range stances {
		out = append(out, workBrief{
			Title: s.Work.Title, Year: s.Work.Year, DOI: s.Work.DOI, CitedBy: s.Work.CitedBy,
			Design: s.Design, Stance: s.Stance, StanceConf: s.Confidence,
			Abstract: clipAbstract(s.Work.Abstract),
		})
	}
	return out
}

func renderConsensus(w io.Writer, o consensusOutput) {
	fmt.Fprintf(w, "Claim: %s\n\n", o.Claim)
	fmt.Fprintf(w, "  Verdict:           %s\n", o.Verdict)
	fmt.Fprintf(w, "  Consensus score:   %+.2f  (-1 refute … +1 support)\n", o.ConsensusScore)
	fmt.Fprintf(w, "  Confidence:        %.0f%%\n", o.Confidence*100)
	if o.EvidenceGuarded {
		fmt.Fprintf(w, "  Evidence strength: %s  ⚠ guarded: only %d relevant work(s), no AI filtering (apex: %s)\n",
			o.EvidenceStrength, o.RelevantCount, o.ApexDesign)
	} else {
		fmt.Fprintf(w, "  Evidence strength: %s (apex: %s)\n", o.EvidenceStrength, o.ApexDesign)
	}
	fmt.Fprintf(w, "  Studies analyzed:  %d  (support %d / refute %d / mixed %d / inconclusive %d)\n",
		o.StudyCount, o.Supporting, o.Refuting, o.Mixed, o.Inconclusive)
	fmt.Fprintf(w, "  Total citations:   %d\n", o.TotalCitations)
	fmt.Fprintf(w, "  Stance method:     %s\n", o.Method)
	if o.NearUnanimous {
		fmt.Fprintln(w, "  ⚠ Near-unanimous:  no refuting or mixed studies — verify debate was not filtered out")
	}
	if len(o.TopSupporting) > 0 {
		fmt.Fprintln(w, "\n  Top supporting:")
		for _, b := range o.TopSupporting {
			fmt.Fprintf(w, "    • [%d, cites=%d, %s] %s\n", b.Year, b.CitedBy, b.Design, truncate(b.Title, 80))
		}
	}
	if len(o.TopRefuting) > 0 {
		fmt.Fprintln(w, "\n  Top refuting:")
		for _, b := range o.TopRefuting {
			fmt.Fprintf(w, "    • [%d, cites=%d, %s] %s\n", b.Year, b.CitedBy, b.Design, truncate(b.Title, 80))
		}
	}
	if o.Note != "" {
		fmt.Fprintf(w, "\n  Note: %s\n", o.Note)
	}
}
