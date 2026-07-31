package scengine

import "testing"

// TestClassifyStance_RiskFactorPhrasing covers the "risk factor" family, which
// harmCues did not recognise: it matched only adjacent "increas* (the) risk"
// and "associated with (a) higher/greater risk". Measured before the fix, all
// four harm sentences below classified as inconclusive with confidence 0.20 —
// the engine saw no direction at all, so the claim-side inversion never ran.
//
// The guard cases matter as much as the harm cases. A bare "risk factors? for"
// pattern would also match "reduced risk factors for cardiovascular disease",
// turning a benefit finding into harm. The cue therefore requires a copula
// (is/are/was/were/remains/as) in front, so a verb of reduction cannot be
// mistaken for an assertion of risk.
func TestClassifyStance_RiskFactorPhrasing(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		abstract string
		claim    string
		want     Stance
	}{
		// --- Harm on the paper side. No claim, so harm reads as refuting an
		// implicit benefit claim, matching the existing "increased risk" rows.
		{
			name:     "is a risk factor",
			title:    "Smoking and stroke",
			abstract: "Smoking is a risk factor for stroke in all age groups.",
			want:     StanceRefuting,
		},
		{
			name:     "is a major risk factor",
			title:    "Obesity cohort",
			abstract: "Obesity is a major risk factor for type 2 diabetes.",
			want:     StanceRefuting,
		},
		{
			name:     "are risk factors, plural and article-less",
			title:    "Diet study",
			abstract: "Processed meat and alcohol are risk factors for colorectal cancer.",
			want:     StanceRefuting,
		},
		{
			name:     "identified as a risk factor",
			title:    "Hypertension cohort",
			abstract: "Hypertension was identified as a risk factor for haemorrhagic stroke.",
			want:     StanceRefuting,
		},

		// --- Harm on the claim side. A harm-asserting claim inverts the mapping,
		// so a paper that finds the harm supports the claim.
		{
			name:     "harm claim met by risk-factor finding",
			title:    "Smoking and stroke",
			abstract: "Smoking is a risk factor for stroke in all age groups.",
			claim:    "smoking is a risk factor for stroke",
			want:     StanceSupporting,
		},

		// --- Guards. These must not be dragged into harm by the new cue.
		{
			name:     "GUARD reduced risk factors is benefit",
			title:    "Lifestyle trial",
			abstract: "The intervention significantly reduced risk factors for cardiovascular disease and improved outcomes.",
			want:     StanceSupporting,
		},
		{
			name:     "GUARD lowering risk factors is benefit",
			title:    "Exercise programme",
			abstract: "Exercise lowered the risk factors for heart disease and improved quality of life.",
			want:     StanceSupporting,
		},

		// --- Controls. Phrasings the engine already handled; they must not move.
		{
			name:     "CONTROL increased risk stays harm",
			title:    "Processed meat and cancer",
			abstract: "Consumption was associated with increased risk of colorectal cancer.",
			want:     StanceRefuting,
		},
		{
			name:     "CONTROL harm claim with increase phrasing",
			title:    "Smoking and stroke",
			abstract: "Smoking increased the risk of stroke in all age groups.",
			claim:    "smoking increases the risk of stroke",
			want:     StanceSupporting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conf := ClassifyStance(tt.title, tt.abstract, tt.claim)
			if got != tt.want {
				t.Errorf("stance = %q, want %q (conf %.2f)", got, tt.want, conf)
			}
			if conf < 0 || conf > 1 {
				t.Errorf("confidence out of range: %v", conf)
			}
		})
	}
}

// TestClassifyStance_RiskFactorNegation locks in the negation gate on the
// claim-agnostic path. That path counted harm cues ungated until this change,
// which the risk-factor cue exposed: "was not identified as a risk factor for
// autism" scored refuting at 0.90 confidence — stating the opposite of the
// sentence, with high confidence, on exactly the claim family the engine is
// most often asked about.
//
// Two shapes are covered because they fail differently. In "is not a risk
// factor" the negation sits inside the span the cue would have to match, so
// the closed adjective list blocks it structurally. In "was not identified as
// a risk factor" it sits before the match, so only the backward-window gate
// can catch it. Both must stay inconclusive, and the affirmative control must
// stay supporting — a gate that silences the real finding too is not a fix.
func TestClassifyStance_RiskFactorNegation(t *testing.T) {
	const harmClaim = "mmr vaccine is a risk factor for autism"

	tests := []struct {
		name     string
		abstract string
		claim    string
		want     Stance
	}{
		{
			name:     "negated copula, no claim",
			abstract: "MMR vaccination is not a risk factor for autism in children.",
			want:     StanceInconclusive,
		},
		{
			name:     "negated as-phrase, no claim",
			abstract: "MMR vaccination was not identified as a risk factor for autism.",
			want:     StanceInconclusive,
		},
		{
			name:     "negated copula, harm claim",
			abstract: "MMR vaccination is not a risk factor for autism in children.",
			claim:    harmClaim,
			want:     StanceInconclusive,
		},
		{
			name:     "negated as-phrase, harm claim",
			abstract: "MMR vaccination was not identified as a risk factor for autism.",
			claim:    harmClaim,
			want:     StanceInconclusive,
		},
		{
			name:     "CONTROL affirmative still supports the harm claim",
			abstract: "MMR vaccination is a risk factor for autism in children.",
			claim:    harmClaim,
			want:     StanceSupporting,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conf := ClassifyStance("Vaccine safety", tt.abstract, tt.claim)
			if got != tt.want {
				t.Errorf("stance = %q, want %q (conf %.2f)", got, tt.want, conf)
			}
		})
	}
}
