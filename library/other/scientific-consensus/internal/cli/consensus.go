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
	// bounded, unless --full-abstracts is set. Empty string when the source has
	// no abstract.
	Abstract string `json:"abstract"`
	// Retraction is non-empty when the work was withdrawn. Such works are kept
	// in AllStudies and excluded from scoring: the PRISMA convention is that a
	// reader must be able to see what was removed and why, and a work that
	// silently vanishes is indistinguishable from one that was never fetched.
	Retraction scengine.Retraction `json:"retraction,omitempty"`
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
	// FetchedCount is how many works the source returned, before any gate ran.
	// It is the head of the exclusion ledger; without it a reader can see what
	// survived but not what was removed.
	FetchedCount int `json:"fetched_count"`
	// RelevanceExcluded is how many works the lexical relevance gate dropped.
	// On the vitamin C corpus this is 17 of 49 — large enough that leaving it
	// implicit would misrepresent the corpus the score was computed from.
	RelevanceExcluded int `json:"relevance_excluded"`
	// PICOExcluded is how many works the PICO gate dropped: works whose abstract
	// or title fails to name BOTH the claim's intervention and its outcome.
	//
	// A zero here is a measurement. That is worth stating explicitly because it
	// was NOT true until 2026-08-02: PICOTokens used to split claims with
	// claimSides (stance.go), which searches only claimHarmCues, so no benefit
	// verb ever matched. Every benefit-shaped claim yielded empty token lists,
	// IsPICORelevant short-circuited to true for all works, and this field
	// reported 0 because the gate had never run — indistinguishable, to a reader,
	// from a gate that ran and excluded nothing. 4d81ac382 routed PICOTokens
	// through the direction-neutral polarityVerbCues instead; measured on the
	// vitaminc corpus the same field went from 0 to 23 of 49, all 23 verified
	// correct, with no movement on the 12 harm corpora.
	//
	// The one case where a zero still means "not applied" is a claim PICOTokens
	// cannot split at all — it then returns empty sides and the gate deliberately
	// bypasses itself rather than filtering blind. TestPICOGateSplitsBenefitClaims
	// pins that this is no longer the ordinary case for benefit claims.
	PICOExcluded int `json:"pico_excluded"`
	// DuplicateExcluded is how many works were dropped as superseded editions
	// of a work already in the corpus (Cochrane .pubN republications).
	//
	// Measured on the vitamin C corpus 2026-08-03: of 9 works surviving the
	// gates, 4 were the same review CD000980 (.pub4 2013, .pub3 2007, .pub2
	// 2004, original 1998). One review cast 44% of the votes and, because
	// Consensus() weights by citations, carried its authority in four times
	// over. A zero here means the corpus held no multi-edition families.
	DuplicateExcluded int `json:"duplicate_excluded"`
	// RelevantCount is how many fetched works survived BOTH relevance gates and
	// therefore entered scoring. It is the input to the low-evidence safety
	// guard, and consumers need it to judge how thin the corpus was.
	RelevantCount int `json:"relevant_count"`
	// NearUnanimous mirrors scengine.ConsensusResult.NearUnanimous: the result
	// is so one-sided that real dissent was probably filtered out upstream.
	NearUnanimous bool `json:"near_unanimous"`
	// EvidenceGuarded reports that the low-evidence safety guard fired and
	// overwrote EvidenceStrength with "insufficient". Without this flag a
	// consumer cannot tell a guarded label from a measured one.
	EvidenceGuarded bool `json:"evidence_guarded"`
	// RetractedExcluded is how many relevant works were withheld from scoring
	// because they are retracted.
	//
	// Together with the fields above this makes the whole pipeline checkable
	// arithmetic rather than prose:
	//
	//	fetched_count  = relevance_excluded + pico_excluded + duplicate_excluded + relevant_count
	//	relevant_count = retracted_excluded + study_count
	//
	// gateLedger.consistent enforces both, and a run that violates either says
	// so in note rather than reporting a total that does not add up.
	RetractedExcluded int         `json:"retracted_excluded"`
	TopSupporting     []workBrief `json:"top_supporting"`
	TopRefuting       []workBrief `json:"top_refuting"`
	// AllStudies lists every analyzed work (post relevance gate) in fetch
	// (relevance) order, so content-aware consumers can re-filter by
	// abstract instead of trusting the top lists alone.
	AllStudies []workBrief `json:"all_studies"`
	Note       string      `json:"note,omitempty"`
}

// maxAbstractChars bounds per-study abstract length in JSON output.
const maxAbstractChars = 1500

// fullAbstractsEnabled disables the maxAbstractChars cap for one command run.
//
// It exists because the cap is invisible to the engine but destroys the JSON as
// a MEASUREMENT input. Every gate and classifier in this CLI reads the full
// reconstructed abstract (scwork.go builds it, the relevance gate, the PICO
// gate and ClassifyStance all consume it); clipAbstract runs afterwards, only
// while building workBrief for output. So a run is scored on the whole text and
// then serialized as a stump — measured on the archived corpora, 110 of 246
// studies (45%) reach an analyst already truncated, and replaying the gate over
// them produces exclusions that never happened in production.
//
// Package-level rather than a parameter on topByStance/allStudyBriefs so the
// two clipAbstract call sites stay untouched and compare/controversies keep
// their signatures. Same idiom as phase5SortEnabled below and picoGateEnabled
// in scengine. RunE sets it from the flag and resets it on return, so a
// long-lived process (the MCP server runs many commands in one process) cannot
// leak the setting from one invocation into the next.
var fullAbstractsEnabled = false

// clipAbstract caps an abstract at maxAbstractChars characters, cutting on a
// rune boundary so multi-byte text is never split mid-character. Returns the
// input unchanged when fullAbstractsEnabled is set.
func clipAbstract(s string) string {
	if fullAbstractsEnabled {
		return s
	}
	if len(s) <= maxAbstractChars {
		return s
	}
	r := []rune(s)
	if len(r) <= maxAbstractChars {
		return s
	}
	return string(r[:maxAbstractChars])
}

// gateLedger is the PRISMA accounting for one consensus run: how many works each
// gate removed, and the rule each one applied.
//
// It is a plain struct built in RunE and consumed by pure functions so the
// reporting can be tested without a network client. The alternative — making the
// command's API client injectable — would have to be repeated in compare.go,
// which runs its own computeConsensus, so the seam would grow larger than the
// repair.
type gateLedger struct {
	// backfill records the abstract enrichment that ran BEFORE the gates. It
	// removes nothing, so it takes no part in consistent() below — but it
	// changes what the gates were reading, which is why a run that recovered
	// abstracts must say so next to the exclusions those gates produced.
	backfill backfillReport
	// relevance carries the lexical gate's own accounting (stems, thresholds).
	relevance relevanceReport
	// picoExcluded is how many works the PICO gate removed; picoIV/picoOut are
	// the tokens it split the claim into. Both token slices are nil when the
	// claim could not be split, which is when the gate does not run at all.
	picoExcluded    int
	picoIV, picoOut []string
	// dedupe carries the version-collapse accounting: how many superseded
	// editions were dropped and which families they came from. Unlike backfill
	// this one DOES remove works, so it takes part in consistent() below.
	dedupe dedupeReport
	// relevantCount is the corpus size after both gates: the set that reaches
	// scoring.
	relevantCount int
	// retractedExcluded and studyCount close the ledger on the scoring side.
	retractedExcluded int
	studyCount        int
}

// consistent reports whether the ledger adds up. Both chains must hold:
//
//	fetched        = relevance_excluded + pico_excluded + duplicate_excluded + relevant_count
//	relevant_count = retracted_excluded + study_count
//
// The second is the invariant the retraction gate introduced; the first is its
// counterpart for the relevance gates and the version dedupe, and is what makes
// a 17-work exclusion verifiable instead of merely stated.
func (g gateLedger) consistent() bool {
	return g.relevance.Fetched == g.relevance.Excluded+g.picoExcluded+g.dedupe.Excluded+g.relevantCount &&
		g.relevantCount == g.retractedExcluded+g.studyCount
}

// gateNotes renders one fragment per gate that actually removed something, plus
// a self-check fragment when the arithmetic does not close.
//
// Each fragment names the gate AND the rule that fired. "17 off-topic work(s)
// excluded" is a number a reader has to take on faith; "17 excluded — fewer than
// 2 of the claim's 3 stems [vitam commo cold] in abstract+title+topic" is a
// number they can audit against the study list beside it.
func gateNotes(g gateLedger) []string {
	var out []string

	// First, because it happened first: the gates below judged the corpus this
	// step produced, not the one OpenAlex returned.
	if n := backfillNote(g.backfill); n != "" {
		out = append(out, n)
	}

	if g.relevance.Excluded > 0 {
		out = append(out, fmt.Sprintf(
			"%d off-topic work(s) excluded by the relevance gate: fewer than %d of the claim's %d distinct stem(s) %v found in abstract+title+topic (a work with no abstract needs only %d)",
			g.relevance.Excluded, g.relevance.MinTokens, len(g.relevance.Stems),
			g.relevance.Stems, g.relevance.MinTokensNoAbstract))
	}

	if g.picoExcluded > 0 {
		out = append(out, fmt.Sprintf(
			"%d work(s) excluded by PICO gate (missing intervention %v or outcome %v in abstract/title)",
			g.picoExcluded, g.picoIV, g.picoOut))
	}

	if g.dedupe.Excluded > 0 {
		out = append(out, fmt.Sprintf(
			"%d superseded edition(s) collapsed: newest edition kept, citations NOT summed [%s]",
			g.dedupe.Excluded, strings.Join(g.dedupe.Families, "; ")))
	}

	if g.retractedExcluded > 0 {
		out = append(out, fmt.Sprintf(
			"%d retracted work(s) excluded from the score (still listed in all_studies)",
			g.retractedExcluded))
	}

	if !g.consistent() {
		out = append(out, fmt.Sprintf(
			"WARNING: exclusion accounting does not add up (fetched %d; relevance %d + pico %d + duplicate %d + relevant %d; relevant %d = retracted %d + scored %d) — treat the counts as unreliable",
			g.relevance.Fetched, g.relevance.Excluded, g.picoExcluded, g.dedupe.Excluded, g.relevantCount,
			g.relevantCount, g.retractedExcluded, g.studyCount))
	}

	return out
}

// appendGateNotes folds every gate fragment onto an existing note.
func appendGateNotes(note string, g gateLedger) string {
	for _, n := range gateNotes(g) {
		note = appendNote(note, n)
	}
	return note
}

// noSurvivorsNote is the note for a run where no work reached scoring.
//
// A function rather than an inline string in RunE so the branch's text is
// reachable from a test. It is the branch most likely to be read by someone
// who got an empty answer and needs to know whether the literature is silent or
// the gate was: an unexplained empty result is indistinguishable from a broken
// query, and V6 is stricter than the rule it replaces, so this branch fires more
// often than before, not less.
func noSurvivorsNote(g gateLedger) string {
	return appendGateNotes(fmt.Sprintf(
		"none of the %d fetched work(s) survived the relevance gates; consensus not "+
			"available — try restating the claim with the terms the literature uses",
		g.relevance.Fetched), g)
}

func newNovelConsensusCmd(flags *rootFlags) *cobra.Command {
	var limit int
	var yearFrom int
	var enrich bool
	var fullAbstracts bool

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
			// Scoped to this invocation: the MCP server runs many commands in
			// one process, and a leaked "true" would silently uncap every later
			// consensus/compare/controversies result.
			fullAbstractsEnabled = fullAbstracts
			defer func() { fullAbstractsEnabled = false }()

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

			// Abstract backfill, BEFORE both gates because supplying an
			// abstract is what changes their verdicts. 18% of works arrive from
			// OpenAlex with no abstract at all (52 of 295 archived studies), and
			// a work gated on its title alone is gated on a property of
			// OpenAlex's coverage rather than of its own content.
			backfill := backfillAbstracts(ctx, claim, works)

			// Relevance gate: drop works that do not share enough content with
			// the claim, before enrichment so excluded works cost no PubMed
			// lookups and never enter the score. The report is carried to the
			// output rather than reduced to a count, because the count alone
			// cannot say which rule fired.
			works, relReport := filterRelevantReport(claim, works)

			// --- Phase 3: PICO relevance gate ---
			// The lexical gate keeps a work that shares enough tokens with the
			// claim. That is still too weak for a two-sided claim: a paper about
			// vaccine schedules with no mention of autism can pass it. The PICO
			// gate additionally requires the intervention AND the outcome to
			// both appear in the abstract or title.
			//
			// It only does so for HARM-shaped claims. See PICOExcluded's comment
			// above: for a benefit claim the split fails and this loop keeps
			// everything.
			ivTokens, outTokens := scengine.PICOTokens(claim)
			picoRelevant := make([]scWork, 0, len(works))
			for _, w := range works {
				if scengine.IsPICORelevant(w.Abstract, w.Title, ivTokens, outTokens) {
					picoRelevant = append(picoRelevant, w)
				}
			}
			picoDropped := len(works) - len(picoRelevant)
			works = picoRelevant

			// Version dedupe. Runs after both relevance gates so its count is a
			// property of the corpus that survived them, and before
			// relevantCount so the number entering scoring and the low-evidence
			// guard is a count of distinct works rather than of editions.
			works, dedupeRep := dedupeVersions(works)

			// relevantCount is the post-gate corpus size — exactly the set that
			// reaches scoring, and the input to the low-evidence safety guard.
			relevantCount := len(works)

			ledger := gateLedger{
				backfill:      backfill,
				relevance:     relReport,
				picoExcluded:  picoDropped,
				picoIV:        ivTokens,
				picoOut:       outTokens,
				dedupe:        dedupeRep,
				relevantCount: relevantCount,
			}

			// Nothing survived the gates. Reporting a verdict computed from zero
			// studies would be worse than reporting none, so this is emitted as
			// a normal result (not an error) with the machine-readable shape
			// agent consumers expect.
			//
			// The note MUST explain which gate emptied the corpus. Before V6
			// this branch blamed the PICO gate unconditionally, which was wrong
			// twice over: the lexical exclusions vanished from the report
			// entirely, and on a benefit claim the PICO gate had not run at all.
			// V6 is stricter than the rule it replaces, so this branch gets more
			// common, not less.
			if relevantCount == 0 {
				out := consensusOutput{
					Claim:            claim,
					Verdict:          scengine.VerdictInsufficient,
					EvidenceStrength: scengine.StrengthInsufficient,
					// Explicit, for the same reason Consensus() seeds it: an
					// unset Design marshals as "" rather than as a design.
					ApexDesign:        scengine.DesignUnknown,
					Method:            stanceMethodLabel(nil),
					FetchedCount:      relReport.Fetched,
					RelevanceExcluded: relReport.Excluded,
					PICOExcluded:      picoDropped,
					DuplicateExcluded: dedupeRep.Excluded,
					RelevantCount:     0,
					EvidenceGuarded:   true,
					TopSupporting:     []workBrief{},
					TopRefuting:       []workBrief{},
					AllStudies:        []workBrief{},
				}
				out.Note = noSurvivorsNote(ledger)
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

			// Retraction gate. A withdrawn paper is not evidence, and the
			// damage is not hypothetical: a live run on "vitamin C prevents
			// the common cold" scored a meta-analysis that had been retracted
			// for double-counting placebo arms — as SUPPORTING, at the top of
			// the evidence pyramid, where it set apex_design for the whole
			// result.
			//
			// The gate runs HERE, between classification and scoring, and not
			// by giving retracted works their own Stance. Consensus() tallies
			// stances with a switch ending in `default: res.Inconclusive++`,
			// and it collects designs and citations BEFORE that switch — so a
			// retraction stance would still reach ApexDesign and
			// TotalCitations while looking handled.
			//
			// The split lives in scorableWorks (scwork.go) rather than here so
			// compare and batch apply the identical rule; two commands
			// disagreeing about which works count would be worse than no gate.
			scorable, retractedExcluded := scorableWorks(scored, stances)
			result := scengine.Consensus(scorable)

			ledger.retractedExcluded = retractedExcluded
			ledger.studyCount = result.StudyCount

			// Low-evidence safety guard. method is resolved BEFORE the guard
			// because the guard's first question is "did an LLM vet relevance?"
			// — with an LLM the strength ladder is trustworthy, without one a
			// corpus under scengine.LowEvidenceThreshold cannot support any
			// strength label at all. The post-guard label is compared against
			// the measured one so the JSON can report that a guard, not a
			// measurement, produced the value.
			method := stanceMethodLabel(stances)
			measuredStrength := result.EvidenceStrength
			// len(scorable), not relevantCount: the guard asks whether the
			// corpus that was actually SCORED is thick enough to carry a
			// strength label, and retracted works never entered it.
			result = scengine.ApplyLowEvidenceGuard(result, method, len(scorable))
			evidenceGuarded := result.EvidenceStrength != measuredStrength

			out := consensusOutput{
				Claim: claim, Verdict: result.Verdict, ConsensusScore: result.ConsensusScore,
				Confidence: result.Confidence, EvidenceStrength: result.EvidenceStrength,
				ApexDesign: result.ApexDesign, StudyCount: result.StudyCount,
				Supporting: result.Supporting, Refuting: result.Refuting, Mixed: result.Mixed,
				Inconclusive: result.Inconclusive, TotalCitations: result.TotalCitations,
				Method:            method,
				FetchedCount:      relReport.Fetched,
				RelevanceExcluded: relReport.Excluded,
				PICOExcluded:      picoDropped,
				DuplicateExcluded: dedupeRep.Excluded,
				RelevantCount:     relevantCount,
				NearUnanimous:     result.NearUnanimous,
				EvidenceGuarded:   evidenceGuarded,
				RetractedExcluded: retractedExcluded,
			}
			out.TopSupporting = topByStance(stances, scengine.StanceSupporting, 3)
			out.TopRefuting = topByStance(stances, scengine.StanceRefuting, 3)
			out.AllStudies = allStudyBriefs(stances)
			if result.StudyCount == 0 && retractedExcluded > 0 {
				out.Note = fmt.Sprintf(
					"all %d relevant work(s) are retracted or flagged as retracted; nothing left to score",
					retractedExcluded)
			} else if result.StudyCount == 0 {
				out.Note = "no works found; try a broader claim or --data-source live"
			} else if result.Verdict == scengine.VerdictInsufficient {
				out.Note = "fewer than 3 directional studies; treat as preliminary"
			}
			out.Note = appendGateNotes(out.Note, ledger)
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
	cmd.Flags().BoolVar(&fullAbstracts, "full-abstracts", false,
		"Emit untruncated abstracts in JSON output. For measurement and regression testing only — output may reach several MB and can exceed downstream LLM prompt limits.")
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

// phase5SortEnabled controls evidence-tier-first ordering of the top-N stance
// lists. Always true in production.
var phase5SortEnabled = true

// topByStance builds the top-N card list for one stance, strongest evidence
// first: primary key is the design's tier rank (lower = higher on the evidence
// pyramid), secondary key is citation count, descending.
//
// Ordering by citations alone put the wrong studies on the cards. Citation
// counts are strongly age-dependent — a 2024 meta-analysis has had no time to
// accumulate them while a 1998 cohort study has had decades — so a pure
// citation sort systematically surfaces older, weaker evidence. Because the
// cut to N happens AFTER the sort, this changes WHICH works appear, not just
// their order; that is the intent, not a side effect.
//
// The input slice is never reordered: matches is a fresh slice, so the caller's
// stance order (and therefore all_studies' relevance order) is preserved.
func topByStance(stances []workStance, stance scengine.Stance, n int) []workBrief {
	matches := make([]workStance, 0)
	for _, s := range stances {
		// Retracted works are excluded from the score, so presenting one on a
		// "top evidence" card would contradict the number beside it. They stay
		// visible in all_studies with their reason attached.
		if s.Work.Retraction.ExcludeFromScore() {
			continue
		}
		if s.Stance == stance {
			matches = append(matches, s)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if phase5SortEnabled {
			ri, rj := scengine.TierRank(matches[i].Design), scengine.TierRank(matches[j].Design)
			if ri != rj {
				return ri < rj
			}
		}
		return matches[i].Work.CitedBy > matches[j].Work.CitedBy
	})
	if len(matches) > n {
		matches = matches[:n]
	}
	out := make([]workBrief, 0, len(matches))
	for _, m := range matches {
		out = append(out, workBrief{
			Title: m.Work.Title, Year: m.Work.Year, DOI: m.Work.DOI, CitedBy: m.Work.CitedBy,
			Design: m.Design, Stance: m.Stance, StanceConf: m.Confidence,
			Abstract:   clipAbstract(m.Work.Abstract),
			Retraction: m.Work.Retraction,
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
			Abstract:   clipAbstract(s.Work.Abstract),
			Retraction: s.Work.Retraction,
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
	// The exclusion ledger, on the human path too. A reader who sees "32
	// studies analyzed" without being told that 17 were removed first is being
	// shown a filtered corpus as if it were the whole one.
	if o.FetchedCount > 0 && o.FetchedCount != o.RelevantCount {
		fmt.Fprintf(w, "  Works fetched:     %d  (relevance gate removed %d, PICO gate %d, duplicate editions %d)\n",
			o.FetchedCount, o.RelevanceExcluded, o.PICOExcluded, o.DuplicateExcluded)
	}
	if o.RetractedExcluded > 0 {
		fmt.Fprintf(w, "  ⚠ Retracted:       %d work(s) excluded from the score\n", o.RetractedExcluded)
	}
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
