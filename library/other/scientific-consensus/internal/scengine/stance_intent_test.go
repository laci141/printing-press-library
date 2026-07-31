package scengine

import "testing"

// TestClassifyStance_PurposeClause covers support cues that state a PURPOSE
// rather than report a FINDING. "Vitamin C for preventing the common cold" is
// the title of a Cochrane review whose conclusion is that vitamin C does not
// prevent colds in the general population; the verb names what was studied,
// not what was found, and counting it scored the review as agreeing with the
// claim it refutes.
//
// The rule is deliberately narrow: only "for" + gerund. Measured over the 13
// archived corpora, the three prepositions behave differently —
//
//	for   23 cues, 7 works change stance   purpose clauses throughout
//	in    11 cues, 1 work changes stance   mostly findings
//	to     1 cue,  0 works change stance   noise
//
// — and the "in" examples are exactly the ones that must survive: "benefit of
// fish oil supplementation IN PREVENTING cardiovascular events" and "efficacy
// of meditation programs IN IMPROVING stress-related outcomes" are results.
// Widening the rule to "in" would silence them, and one of them sits in the
// meditation benefit control.
func TestClassifyStance_PurposeClause(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		abstract string
		want     Stance
	}{
		{
			name:     "purpose clause in title, null finding in abstract",
			title:    "Vitamin C for preventing the common cold",
			abstract: "There was no significant difference in incidence between the groups.",
			want:     StanceRefuting,
		},
		{
			name:     "purpose clause in abstract prose",
			title:    "Dietary policy review",
			abstract: "The authors highlight the need for improving diet across nations. No significant difference was observed between the cohorts.",
			want:     StanceRefuting,
		},

		{
			name:     "GUARD finding form still counts",
			title:    "Vitamin C trial",
			abstract: "Supplementation prevented illness and improved recovery times.",
			want:     StanceSupporting,
		},
		{
			name:     "GUARD in-gerund is a finding, not a purpose",
			title:    "Meditation programme",
			abstract: "The programme was effective in improving stress-related outcomes.",
			want:     StanceSupporting,
		},
		{
			name:     "GUARD to-gerund is not a purpose clause either",
			title:    "Mechanism review",
			abstract: "Understanding these effects is essential to improving therapeutic potential, and the intervention improved outcomes.",
			want:     StanceSupporting,
		},
		{
			name:     "GUARD bare gerund with no preposition still counts",
			title:    "Exercise trial",
			abstract: "The programme was successful, significantly improving quality of life.",
			want:     StanceSupporting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conf := ClassifyStance(tt.title, tt.abstract, "")
			if got != tt.want {
				t.Errorf("stance = %q, want %q (conf %.2f)", got, tt.want, conf)
			}
			if conf < 0 || conf > 1 {
				t.Errorf("confidence out of range: %v", conf)
			}
		})
	}
}
