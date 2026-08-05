package scengine

import (
	"math"
	"testing"
)

// TestApplyLowEvidenceGuard pins the guard's full effect: when stance
// classification ran without an LLM and too few studies survived the relevance
// gate, BOTH the strength label and the verdict are neutralised, and the
// numbers underneath them are left exactly as they were.
//
// The verdict half is the newer half, and it exists because the headline is
// louder than the footnote. Measured in production on "HPV vaccine causes
// infertility" (keyless, limit 20): one refuting work out of three relevant
// ones produced a perfect -1.00 with the guard already active, and the run
// still reported "evidence-refutes" with "insufficient" printed underneath in
// smaller type.
//
// Every case below also asserts that ConsensusScore, Confidence and
// NearUnanimous come out identical to what went in. That is the guard rail on
// the part deliberately NOT changed: the score is a reproducible heuristic and
// blanking it would lose the record of which way the thin evidence pointed.
//
// The archived corpora cannot exercise any of this — all 13 of them carry 7 or
// more works against a threshold of 5, so the guard is never active in a single
// one. The proof has to be synthetic.
func TestApplyLowEvidenceGuard(t *testing.T) {
	// Deliberately un-round values: a byte-identical check on a value like 0.5
	// would pass even if something re-derived it.
	const (
		inScore = -0.9966666666666667
		inConf  = 0.4123456789
	)

	tests := []struct {
		name          string
		method        string
		relevantCount int
		inVerdict     Verdict
		inStrength    EvidenceStrength
		wantVerdict   Verdict
		wantStrength  EvidenceStrength
	}{
		{
			name:          "guard neutralises a refuting verdict",
			method:        "heuristic",
			relevantCount: 3,
			inVerdict:     VerdictRefutes,
			inStrength:    StrengthModerate,
			wantVerdict:   VerdictInsufficient,
			wantStrength:  StrengthInsufficient,
		},
		{
			name:          "guard neutralises a supporting verdict",
			method:        "heuristic",
			relevantCount: 3,
			inVerdict:     VerdictSupports,
			inStrength:    StrengthHigh,
			wantVerdict:   VerdictInsufficient,
			wantStrength:  StrengthInsufficient,
		},
		{
			name:          "already insufficient is not double-applied",
			method:        "heuristic",
			relevantCount: 1,
			inVerdict:     VerdictInsufficient,
			inStrength:    StrengthInsufficient,
			wantVerdict:   VerdictInsufficient,
			wantStrength:  StrengthInsufficient,
		},
		{
			// The boundary. An off-by-one here would silently neutralise
			// verdicts backed by enough evidence to state one.
			name:          "exactly at the threshold is untouched",
			method:        "heuristic",
			relevantCount: LowEvidenceThreshold,
			inVerdict:     VerdictRefutes,
			inStrength:    StrengthModerate,
			wantVerdict:   VerdictRefutes,
			wantStrength:  StrengthModerate,
		},
		{
			name:          "one below the threshold is guarded",
			method:        "heuristic",
			relevantCount: LowEvidenceThreshold - 1,
			inVerdict:     VerdictRefutes,
			inStrength:    StrengthModerate,
			wantVerdict:   VerdictInsufficient,
			wantStrength:  StrengthInsufficient,
		},
		{
			// An LLM vetted relevance, so a thin corpus is a real answer
			// rather than a lexical accident.
			name:          "an llm run is untouched at a low count",
			method:        "llm:anthropic",
			relevantCount: 1,
			inVerdict:     VerdictRefutes,
			inStrength:    StrengthModerate,
			wantVerdict:   VerdictRefutes,
			wantStrength:  StrengthModerate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := ConsensusResult{
				Verdict:          tt.inVerdict,
				ConsensusScore:   inScore,
				Confidence:       inConf,
				EvidenceStrength: tt.inStrength,
				ApexDesign:       DesignRCT,
				StudyCount:       3,
				Refuting:         1,
				Inconclusive:     2,
				TotalCitations:   42,
				NearUnanimous:    true,
			}

			got := ApplyLowEvidenceGuard(in, tt.method, tt.relevantCount)

			if got.Verdict != tt.wantVerdict {
				t.Errorf("verdict = %q, want %q", got.Verdict, tt.wantVerdict)
			}
			if got.EvidenceStrength != tt.wantStrength {
				t.Errorf("evidence_strength = %q, want %q", got.EvidenceStrength, tt.wantStrength)
			}

			// Compared bit-for-bit rather than with a tolerance: the guard does
			// no arithmetic on these at all, so any difference is a bug, not
			// rounding.
			if math.Float64bits(got.ConsensusScore) != math.Float64bits(in.ConsensusScore) {
				t.Errorf("consensus_score = %v, want %v byte-identical", got.ConsensusScore, in.ConsensusScore)
			}
			if math.Float64bits(got.Confidence) != math.Float64bits(in.Confidence) {
				t.Errorf("confidence = %v, want %v byte-identical", got.Confidence, in.Confidence)
			}
			if got.NearUnanimous != in.NearUnanimous {
				t.Errorf("near_unanimous = %v, want %v unchanged", got.NearUnanimous, in.NearUnanimous)
			}
		})
	}
}
