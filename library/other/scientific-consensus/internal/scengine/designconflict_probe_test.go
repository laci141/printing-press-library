package scengine

// designconflict_probe_test.go records a MEASURED-AND-REJECTED proposal: that
// a work's title should be allowed to override the study design the engine
// assigned to it. Log-only; nothing here asserts.
//
// THE CASE, observed live 2026-08-06 on corpova.pubvera.com:
//
//	"Reasons for Receiving or Not Receiving HPV Vaccination in Primary
//	 Schoolgirls in Tanzania: A Case Control Study"  (PMID 23115621)
//
// was classified randomized-controlled-trial, became the apex design of the
// whole result, set Evidence Strength, and was then repeated back to the user
// by the LLM as "a randomized-controlled trial in Tanzania found no evidence
// that the vaccine causes infertility" — a sentence that is wrong twice over,
// since the paper measured vaccine uptake, not fertility.
//
// MEASURED AT THE SOURCE. PubMed lists exactly four publication types for it:
//
//	Clinical Trial, Phase IV
//	Journal Article
//	Randomized Controlled Trial
//	Research Support, Non-U.S. Gov't
//
// There is NO "Case-Control Studies" tag. ClassifyDesign did not mis-pick
// between competing tags — it was handed one tag and used it. The case-control
// analysis was nested inside a larger phase IV cluster-randomised vaccination
// trial, and the indexer recorded the containing trial.
//
// FIRST LESSON: a PubMed publication type describes the study the paper came
// out of, which is not always the analysis the paper reports.
//
// THE PROPOSAL: when a title names its own design and that contradicts the
// assigned one, trust the title.
//
// THE MEASUREMENT, 2026-08-06, thirteen corpora (twelve calibrated plus
// hpvinfertility):
//
//	agree = 6   title-weaker = 2   title-stronger = 0   unrelated = 0
//
// But seventeen of those eighteen are FALSE ALARMS of the probe itself, not
// engine errors. They all have the shape:
//
//	title=systematic-review  assigned=meta-analysis
//	"... : A Systematic Review and Meta-analysis"
//
// Those papers are both things at once, which is entirely ordinary, and the
// engine correctly files them at the higher tier. The first version of this
// probe matched "systematic review" before it looked for "meta-analysis" and
// reported the pair as a conflict. The regex below now requires the absence of
// a meta-analysis phrase before treating a review label as the title's claim.
//
// VERDICT: REJECTED. After the fix there are exactly TWO genuine conflicts in
// thirteen corpora: the HPV case above, and "Omega-3 Dietary Supplements and
// the Risk of Cardiovascular Events: A Systematic Review", which the engine
// files as meta-analysis. The second one is probably not even an error — a
// paper can run a meta-analysis without saying so in its title, and PubMed
// indexes from the full text. Two cases, one of them doubtful, do not justify
// a rule that reaches into every classification, and the direction column
// shows no counter-pressure to balance it against (title-stronger = 0), so a
// naive override would look safe on paper while resting on thin evidence.
//
// WHERE THE REAL DEFENCE BELONGS: the damage was not the label, it was the LLM
// repeating the label as established fact. A prompt-level instruction — treat
// the design field as source metadata, and prefer the title when the two
// disagree — costs nothing per classification and fails safe. Not implemented
// here; it needs its own measurement.
//
// WHEN TO REVISIT: when the genuine-conflict count reaches five or so, or when
// a title-stronger case appears, since that is the direction where trusting the
// title would actively make classification worse.

import (
	"regexp"
	"testing"
)

// metaAnalysisPhrase marks a title that also calls itself a meta-analysis. A
// systematic review that says so is not contradicting a meta-analysis label.
var metaAnalysisPhrase = regexp.MustCompile(`(?i)\bmeta[- ]analy(sis|ses)\b`)

// titleDesignClaims maps a phrase a title uses to label its OWN design onto the
// design it names. Deliberately conservative: only phrases an author writes to
// classify the paper, not incidental mentions.
var titleDesignClaims = []struct {
	re     *regexp.Regexp
	design Design
	// skipIfMetaAnalysis suppresses the match when the same title also says
	// meta-analysis, which is the seventeen-case false alarm described above.
	skipIfMetaAnalysis bool
}{
	{re: regexp.MustCompile(`(?i)\bcase[ -]control (study|studies)\b`), design: DesignCaseControl},
	{re: regexp.MustCompile(`(?i)\b(a )?cross[ -]sectional (study|survey|analysis)\b`), design: DesignCrossSectional},
	{re: regexp.MustCompile(`(?i)\bcohort (study|analysis)\b`), design: DesignCohort},
	{re: regexp.MustCompile(`(?i)\bcase (report|series)\b`), design: DesignCaseReport},
	{re: regexp.MustCompile(`(?i)\bsystematic review\b`), design: DesignSystematicReview, skipIfMetaAnalysis: true},
	{re: regexp.MustCompile(`(?i)\bscoping review\b`), design: DesignNarrativeReview},
}

// TestTitleVersusAssignedDesign lists every work whose title labels its own
// design and compares that with the design the corpus records, reporting the
// direction of each disagreement.
func TestTitleVersusAssignedDesign(t *testing.T) {
	corpora := append([]string{}, allCorpora...)
	corpora = append(corpora, "hpvinfertility")

	agree, weaker, stronger, unrelated := 0, 0, 0, 0

	for _, name := range corpora {
		full, err := loadFullCorpus(name)
		if err != nil {
			t.Logf("%s: skipped (%v)", name, err)
			continue
		}

		for _, w := range full.AllStudies {
			for _, tc := range titleDesignClaims {
				if !tc.re.MatchString(w.Title) {
					continue
				}
				if tc.skipIfMetaAnalysis && metaAnalysisPhrase.MatchString(w.Title) {
					continue
				}
				assigned := Design(w.Design)
				titled := tc.design

				title := w.Title
				if len(title) > 70 {
					title = title[:70] + "..."
				}

				switch {
				case assigned == titled:
					agree++
				case titled == DesignUnknown || assigned == DesignUnknown:
					unrelated++
					t.Logf("%-14s UNRELATED  title=%v assigned=%v  %s",
						name, titled, assigned, title)
				case TierRank(assigned) < TierRank(titled):
					weaker++
					t.Logf("%-14s TITLE-WEAKER  title=%v assigned=%v  %s",
						name, titled, assigned, title)
				default:
					stronger++
					t.Logf("%-14s TITLE-STRONGER  title=%v assigned=%v  %s",
						name, titled, assigned, title)
				}
				break // first matching phrase wins
			}
		}
	}

	t.Logf("")
	t.Logf("TOTALS: agree=%d  title-weaker=%d  title-stronger=%d  unrelated=%d",
		agree, weaker, stronger, unrelated)
	t.Logf("Measured 2026-08-06 with the meta-analysis suppression in place:")
	t.Logf("two genuine conflicts, one of them doubtful. See the file header for")
	t.Logf("why that is not enough to justify a title override.")
}
