package scengine

import (
	"strings"
	"testing"
)

func TestClassifyDesign_PubTypeAuthoritative(t *testing.T) {
	tests := []struct {
		name     string
		pubTypes []string
		want     Design
		method   Method
	}{
		{"meta-analysis pubtype", []string{"Journal Article", "Meta-Analysis"}, DesignMetaAnalysis, MethodPubMedPubType},
		{"systematic review pubtype", []string{"Systematic Review"}, DesignSystematicReview, MethodPubMedPubType},
		{"rct pubtype", []string{"Randomized Controlled Trial"}, DesignRCT, MethodPubMedPubType},
		{"apex wins over review", []string{"Review", "Meta-Analysis"}, DesignMetaAnalysis, MethodPubMedPubType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyDesign("Some title", "Some abstract", "article", tt.pubTypes)
			if got.Design != tt.want {
				t.Errorf("design = %q, want %q", got.Design, tt.want)
			}
			if got.Method != tt.method {
				t.Errorf("method = %q, want %q", got.Method, tt.method)
			}
		})
	}
}

func TestClassifyDesign_Heuristic(t *testing.T) {
	tests := []struct {
		title string
		want  Design
	}{
		{"A systematic review and meta-analysis of vitamin D", DesignMetaAnalysis},
		{"A systematic review of statin therapy", DesignSystematicReview},
		{"A double-blind, placebo-controlled randomized trial of zinc", DesignRCT},
		{"A prospective cohort study of diet and mortality", DesignCohort},
		{"A nested case-control study of smoking", DesignCaseControl},
		{"A cross-sectional survey of sleep habits", DesignCrossSectional},
		{"Case report: an unusual presentation of lupus", DesignCaseReport},
		{"Umbrella review of exercise interventions", DesignUmbrellaReview},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := ClassifyDesign(tt.title, "", "article", nil)
			if got.Design != tt.want {
				t.Errorf("design = %q, want %q", got.Design, tt.want)
			}
			if got.Method != MethodHeuristic {
				t.Errorf("method = %q, want heuristic", got.Method)
			}
		})
	}
}

func TestClassifyDesign_Fallback(t *testing.T) {
	got := ClassifyDesign("An ordinary article", "no design cues here", "article", nil)
	if got.Design != DesignUnknown {
		t.Errorf("design = %q, want unknown", got.Design)
	}
	rev := ClassifyDesign("Thoughts on a topic", "", "review", nil)
	if rev.Design != DesignNarrativeReview || rev.Method != MethodOpenAlexType {
		t.Errorf("review fallback = %q/%q", rev.Design, rev.Method)
	}
}

func TestClassifyStance(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		abstract string
		want     Stance
	}{
		{"supporting", "Vitamin D and infection", "Supplementation significantly reduced the risk of respiratory infection and improved outcomes.", StanceSupporting},
		{"refuting", "Vitamin D trial", "There was no significant association and supplementation did not reduce infection rates.", StanceRefuting},
		{"harm", "Beta-carotene trial", "Supplementation was associated with a higher risk of lung cancer and adverse outcomes.", StanceRefuting},
		{"inconclusive", "A descriptive paper", "We describe the prevalence of a condition across regions.", StanceInconclusive},
		// Regression: harm-context "increas*" phrasing must not count as a
		// positive/support cue (RE2 has no lookahead; excluded per-match).
		{"increased risk is harm", "Processed meat and cancer", "Consumption was associated with increased risk of colorectal cancer.", StanceRefuting},
		{"increased the risk is harm", "Smoking study", "Smoking increased the risk of stroke in all age groups.", StanceRefuting},
		{"increased mortality is harm", "Drug X trial", "Treatment with drug X increased mortality compared with placebo.", StanceRefuting},
		{"increased incidence is harm", "Screening cohort", "Exposure increased incidence of adverse events.", StanceRefuting},
		// True positive-increase claims must still count as support.
		{"increased survival supports", "Exercise program", "The program significantly increased survival rates and improved quality of life.", StanceSupporting},
		{"increase in remission supports", "Therapy trial", "Therapy led to an increase in remission and improved adherence.", StanceSupporting},
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

// TestClassifyStance_ClaimAware pins the claim-aware polarity behavior (FIX A).
// Today ClassifyStance ignores the claim and derives stance only from whether
// the paper reports a beneficial finding, so a HARM-asserting claim inverts:
// a paper reporting the harm should SUPPORT the claim, and a paper reporting
// benefit / no-effect should REFUTE it. BENEFIT-asserting and ambiguous claims
// must keep today's behavior exactly (no regression).
//
// These cases FAIL against current code (the two harm cases classify as
// inconclusive today because the base cue vocabulary is risk/mortality/incidence
// and never recognizes "weight gain"; see repro in the plan).
func TestClassifyStance_ClaimAware(t *testing.T) {
	tests := []struct {
		name     string
		claim    string
		title    string
		abstract string
		want     Stance
	}{
		{
			name:     "harm claim, paper shows less of the harm -> refuting",
			claim:    "artificial sweeteners cause weight gain",
			title:    "Non-nutritive sweeteners and body weight: a randomized trial",
			abstract: "The sweetener group showed less weight gain than the sugar group over 12 weeks.",
			want:     StanceRefuting,
		},
		{
			name:     "harm claim, paper shows more of the harm -> supporting",
			claim:    "artificial sweeteners cause weight gain",
			title:    "Sweetener consumption and adiposity: a prospective cohort",
			abstract: "Sweetener consumption was associated with greater weight gain over 10 years of follow-up.",
			want:     StanceSupporting,
		},
		{
			name:     "benefit claim, paper reports the benefit -> supporting (no regression)",
			claim:    "coffee improves alertness",
			title:    "Caffeine and cognitive performance",
			abstract: "Caffeine significantly improved alertness and vigilance in a double-blind trial.",
			want:     StanceSupporting,
		},
		{
			name:     "ambiguous claim -> falls back to today's claim-agnostic behavior",
			claim:    "the relationship between coffee and alertness",
			title:    "Caffeine and cognitive performance",
			abstract: "Caffeine significantly improved alertness and vigilance in a double-blind trial.",
			// Expectation is defined relative to today's behavior below.
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conf := ClassifyStance(tt.title, tt.abstract, tt.claim)
			want := tt.want
			if tt.name == "ambiguous claim -> falls back to today's claim-agnostic behavior" {
				// Ambiguous polarity must be indistinguishable from the
				// claim-agnostic baseline (empty claim) — no crash, no drift.
				want, _ = ClassifyStance(tt.title, tt.abstract, "")
			}
			if got != want {
				t.Errorf("stance = %q, want %q (conf %.2f)", got, want, conf)
			}
			if conf < 0 || conf > 1 {
				t.Errorf("confidence out of range: %v", conf)
			}
		})
	}
}

// TestClassifyStance_HarmClaimGates pins the four gates that a positive cue
// must survive on the harm-claim branch (FIX A + B). Each case isolates ONE
// gate: without that gate the case classifies as supporting, which is the
// exact failure the vaccines/autism corpus exhibited.
func TestClassifyStance_HarmClaimGates(t *testing.T) {
	const vaxClaim = "vaccines cause autism"
	tests := []struct {
		name     string
		claim    string
		title    string
		abstract string
		want     Stance
		gate     string
	}{
		{
			// Without the negation gate: harmCues matches "increase the risk"
			// inside a clause that says the opposite. Real case: Hviid 2019.
			name:     "negated harm cue is not a report of harm",
			claim:    vaxClaim,
			title:    "MMR vaccination and autism",
			abstract: "The study strongly supports that MMR vaccination does not increase the risk for autism in children.",
			want:     StanceInconclusive, // Option C will earn this a refuting cue
			gate:     "negation",
		},
		{
			// Without the framing gate: an Objective sentence poses the
			// question and is scored as if it answered it. Real case: Hviid 2019.
			name:     "question framing is not a finding",
			claim:    vaxClaim,
			title:    "MMR vaccination and autism",
			abstract: "Objective: To evaluate whether the MMR vaccine increases the risk for autism in children.",
			want:     StanceInconclusive,
			gate:     "framing",
		},
		{
			// KNOWN LIMITATION, deliberately pinned. The negation gate is
			// clause-local (negationLookback), so a null finding stated far
			// earlier in the same sentence does not scope over a later cue:
			// "no correlation exists between X and the increase in the risk of
			// Y" still yields one positive cue, landing on mixed instead of
			// refuting. Real case: Kaye 2001.
			//
			// A sentence-wide null gate was measured and rejected: on the
			// vaccines/autism corpus it changed exactly one work (this one,
			// mixed instead of supporting) while making "no benefit was seen;
			// the drug increased mortality" a false negative. Not worth the
			// blast radius. Revisit only with real clause parsing.
			name:     "sentence-wide null finding does NOT scope over a later cue",
			claim:    vaxClaim,
			title:    "MMR and the incidence of autism",
			abstract: "The data provide evidence that no correlation exists between MMR vaccination and the increase in the risk of autism over time.",
			want:     StanceMixed,
			gate:     "clause-local negation (limitation)",
		},
		{
			// The case the rejected sentence-wide gate would have broken: an
			// unrelated null finding early in the sentence must NOT erase a
			// genuine harm report later in it. The harm cue still counts, so
			// the null and the harm balance out to mixed. Under a
			// sentence-wide null gate the positive cue would be dropped and
			// this would read as refuting — the harm report silently deleted.
			name:     "early null finding does not erase a real harm report",
			claim:    vaxClaim,
			title:    "Vaccination and autism: a cohort study",
			abstract: "No difference in adherence was seen, but vaccination was linked to an increased risk of autism.",
			want:     StanceMixed,
			gate:     "negation must stay clause-local",
		},
		{
			// Without the pair gate: the outcome token alone satisfies the old
			// trailing-window check even though the sentence never mentions the
			// intervention. Real case: Kaye 2001's incidence trend.
			name:     "outcome token alone does not tie a cue to the claim",
			claim:    vaxClaim,
			title:    "MMR vaccine and autism: a time trend analysis",
			abstract: "The incidence of newly diagnosed autism increased sevenfold over the decade.",
			want:     StanceInconclusive,
			gate:     "pairing (outcome only)",
		},
		{
			// The mirror image: the intervention token alone is not enough
			// either. Real case: "Social media and vaccine hesitancy".
			name:     "intervention token alone does not tie a cue to the claim",
			claim:    vaxClaim,
			title:    "Social media and vaccine hesitancy",
			abstract: "Exposure to the campaign produced an increase in the number of negative vaccine tweets.",
			want:     StanceInconclusive,
			gate:     "pairing (intervention only)",
		},
		{
			// Both sides in one sentence, no negation, no framing: this is what
			// a genuine report of the claimed harm looks like, and it must
			// still count. Guards against the gates degenerating into "always
			// refute".
			name:     "both claim sides in scope still count as supporting",
			claim:    vaxClaim,
			title:    "Vaccination and autism: a cohort study",
			abstract: "Vaccination was linked to an increased risk of autism in this cohort.",
			want:     StanceSupporting,
			gate:     "pairing (positive control)",
		},
		{
			// Dedup: harmCues ("increased risk") and directionUpCues
			// ("increased") match the same words. Counted twice the positive
			// side is 2 vs 1 negative (ratio 0.67 -> supporting); counted once
			// it is 1 vs 1 -> mixed.
			name:     "overlapping harm and direction cues count once",
			claim:    vaxClaim,
			title:    "Vaccination and autism: a cohort study",
			abstract: "Vaccination was linked to an increased risk of autism, although no significant difference was seen in girls.",
			want:     StanceMixed,
			gate:     "dedup",
		},
		{
			// A claim with nothing on the intervention side cannot be paired;
			// the branch must fall back to the pre-pairing behavior instead of
			// silently counting nothing.
			name:     "unsplittable claim falls back to the unpaired behavior",
			claim:    "causes autism",
			title:    "Exposure and autism",
			abstract: "Exposure was associated with a higher risk of autism.",
			want:     StanceSupporting,
			gate:     "pairing fallback",
		},

		// --- Option C: strongRefutCues (full text, no pairing, B2 applies) ---
		{
			name:  "optC strong: did not cause",
			claim: vaxClaim,
			title: "Vaccines Did Not Cause Rachel's Autism",
			want:  StanceRefuting,
			gate:  "optC strong",
		},
		{
			name:  "optC strong: does not support a causal association",
			claim: vaxClaim,
			title: "Evidence Does Not Support a Causal Association between MMR Vaccine and Autism",
			want:  StanceRefuting,
			gate:  "optC strong",
		},
		{
			name:     "optC strong: no causal link",
			claim:    vaxClaim,
			title:    "Vaccination schedule and autism",
			abstract: "Extensive review found no causal link between vaccination schedule and autism spectrum disorder.",
			want:     StanceRefuting,
			gate:     "optC strong",
		},

		// --- Option C: metaRefutCues (pairing within metaRefutRadius) ---
		{
			// Adapted from the spec text "MMR and autism: further evidence
			// against": the claim's intervention stem is "vacci" (the token
			// "MMR" is three characters and ClaimContentTokens drops it), so
			// the bare MMR title cannot pair. The case below is the same title
			// with the intervention word present. The unpaired variant is
			// pinned separately as a known limitation.
			name:  "optC meta: further evidence against, both sides in radius",
			claim: vaxClaim,
			title: "MMR vaccine and autism: further evidence against a causal association",
			want:  StanceRefuting,
			gate:  "optC meta",
		},
		{
			name:  "optC meta: fraud with pairing",
			claim: vaxClaim,
			title: "Fraudulent data linking MMR vaccine to autism: a retraction analysis",
			want:  StanceRefuting,
			gate:  "optC meta",
		},
		{
			name:  "optC meta: no pairing stays inconclusive",
			claim: vaxClaim,
			title: "Fraudulent Science",
			want:  StanceInconclusive,
			gate:  "optC meta pairing",
		},
		{
			// KNOWN LIMITATION: a title that names only the trade name and the
			// outcome ("MMR and autism: further evidence against a causal
			// association", corpus work #11) has no intervention token, so the
			// meta cue cannot pair and the work stays inconclusive.
			name:  "optC meta: intervention word absent stays inconclusive",
			claim: vaxClaim,
			title: "MMR and autism: further evidence against a causal association",
			want:  StanceInconclusive,
			gate:  "optC meta (limitation)",
		},

		// --- Option C must not defeat the B2 framing gate ---
		{
			name:     "optC: framing blocks a strong refutation cue",
			claim:    vaxClaim,
			title:    "Vaccines and autism",
			abstract: "Objective: to examine whether vaccines do not cause autism in predisposed children.",
			want:     StanceInconclusive,
			gate:     "optC + B2 framing",
		},

		// --- Phase 2 behavior preserved under Option C ---
		{
			// Adapted from the spec case: the claim drives the intervention
			// tokens here, so the claim names corn syrup rather than "sugar",
			// which does not appear in the text.
			name:     "optC regression: genuine harm report still supports",
			claim:    "corn syrup causes obesity",
			title:    "Corn syrup and adolescent obesity",
			abstract: "High fructose corn syrup consumption significantly increased obesity risk in adolescents.",
			want:     StanceSupporting,
			gate:     "optC regression (harm)",
		},
		{
			name:     "optC regression: null finding still refutes",
			claim:    "artificial sweeteners cause weight gain",
			title:    "Artificially sweetened beverages and body weight",
			abstract: "Artificially sweetened beverage intake showed no association with weight gain.",
			want:     StanceRefuting,
			gate:     "optC regression (null)",
		},
		{
			// KNOWN LIMITATION, accepted: a co-occurrence sentence with no
			// negation and no framing still counts, so a time-trend
			// association reads as supporting. Option C is not meant to fix
			// this — it carries no refutation cue to fire on. Real case: the
			// Kaye 2001 time-trend paper (corpus work #34).
			//
			// Adapted from the spec text, which used "a rise in": "rise" is
			// not in directionUpCues, so that wording produces no cue at all
			// and would have pinned inconclusive for the wrong reason.
			name:     "optC known limit: time-trend co-occurrence still supports",
			claim:    vaxClaim,
			title:    "Thimerosal and autism diagnoses",
			abstract: "Thimerosal exposure in vaccines was associated with an increase in autism diagnoses from 1988 to 1999.",
			want:     StanceSupporting,
			gate:     "optC known limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, conf := ClassifyStance(tt.title, tt.abstract, tt.claim)
			if got != tt.want {
				t.Errorf("gate %s: stance = %q, want %q (conf %.2f)", tt.gate, got, tt.want, conf)
			}
			if conf < 0 || conf > 1 {
				t.Errorf("confidence out of range: %v", conf)
			}
		})
	}
}

// TestClaimSides pins the claim split that the pair gate depends on: content
// tokens before the polarity verb are the intervention, the ones after are the
// outcome. An empty side is the documented "cannot pair" signal.
func TestClaimSides(t *testing.T) {
	tests := []struct {
		claim  string
		wantIn []string
		wantOu []string
	}{
		{"vaccines cause autism", []string{"vacci"}, []string{"autis"}},
		{"artificial sweeteners cause weight gain", []string{"artif", "sweet"}, []string{"weigh", "gain"}},
		{"smoking causes lung cancer", []string{"smoki"}, []string{"lung", "cance"}},
		{"causes autism", nil, []string{"autis"}},
		{"coffee improves alertness", nil, nil}, // no harm cue: not a harm claim
	}
	for _, tt := range tests {
		t.Run(tt.claim, func(t *testing.T) {
			in, out := claimSides(tt.claim)
			if !equalTokens(in, tt.wantIn) {
				t.Errorf("intervention = %v, want %v", in, tt.wantIn)
			}
			if !equalTokens(out, tt.wantOu) {
				t.Errorf("outcome = %v, want %v", out, tt.wantOu)
			}
		})
	}
}

func equalTokens(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestSentenceBounds pins the sentence splitter the framing and pairing gates
// scope to, including the decimal guard that keeps "0.3 per 10 000" and a
// confidence interval inside one sentence.
func TestSentenceBounds(t *testing.T) {
	hay := "first sentence here. the rate was 0.3 per 10 000; ci 1.18; second part. third sentence."
	tests := []struct {
		name string
		at   int
		want string
	}{
		{"first sentence", 3, "first sentence here."},
		{"decimals do not split", strings.Index(hay, "per"), "the rate was 0.3 per 10 000; ci 1.18; second part."},
		{"last sentence", strings.Index(hay, "third"), "third sentence."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, e := sentenceBounds(hay, tt.at)
			if got := hay[s:e]; got != tt.want {
				t.Errorf("sentence = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConsensus(t *testing.T) {
	works := []ScoredWork{
		{Stance: StanceSupporting, StanceConf: 0.8, Design: DesignMetaAnalysis, CitedBy: 500},
		{Stance: StanceSupporting, StanceConf: 0.7, Design: DesignRCT, CitedBy: 200},
		{Stance: StanceSupporting, StanceConf: 0.7, Design: DesignSystematicReview, CitedBy: 150},
		{Stance: StanceRefuting, StanceConf: 0.6, Design: DesignCaseReport, CitedBy: 5},
	}
	r := Consensus(works)
	if r.Verdict != VerdictSupports {
		t.Errorf("verdict = %q, want supports", r.Verdict)
	}
	if r.ConsensusScore <= 0 {
		t.Errorf("score = %v, want > 0", r.ConsensusScore)
	}
	if r.StudyCount != 4 || r.Supporting != 3 || r.Refuting != 1 {
		t.Errorf("counts wrong: %+v", r)
	}
	if r.ApexDesign != DesignMetaAnalysis {
		t.Errorf("apex = %q, want meta-analysis", r.ApexDesign)
	}
	if r.TotalCitations != 855 {
		t.Errorf("total citations = %d, want 855", r.TotalCitations)
	}
}

func TestConsensus_Insufficient(t *testing.T) {
	r := Consensus([]ScoredWork{{Stance: StanceSupporting, StanceConf: 0.8, Design: DesignRCT, CitedBy: 10}})
	if r.Verdict != VerdictInsufficient {
		t.Errorf("verdict = %q, want insufficient for n<3", r.Verdict)
	}
	empty := Consensus(nil)
	if empty.Verdict != VerdictInsufficient || empty.StudyCount != 0 {
		t.Errorf("empty consensus wrong: %+v", empty)
	}
}

// strength labels the evidence base from apex design + volume only.
func TestStrength(t *testing.T) {
	tests := []struct {
		name    string
		apex    Design
		studies int
		want    EvidenceStrength
	}{
		{"meta-analysis with volume", DesignMetaAnalysis, 5, StrengthHigh},
		{"meta-analysis thin volume", DesignMetaAnalysis, 4, StrengthModerate},
		{"rct with volume", DesignRCT, 3, StrengthModerate},
		{"rct thin volume", DesignRCT, 2, StrengthLow},
		{"cohort apex", DesignCohort, 10, StrengthLow},
		{"case report apex", DesignCaseReport, 10, StrengthVeryLow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strength(tt.apex, tt.studies); got != tt.want {
				t.Errorf("strength(%q, %d) = %q, want %q", tt.apex, tt.studies, got, tt.want)
			}
		})
	}
}

func TestPyramidAndApex(t *testing.T) {
	cs := []Classification{
		{Design: DesignRCT}, {Design: DesignRCT}, {Design: DesignCaseReport}, {Design: DesignMetaAnalysis},
	}
	levels := Pyramid(cs)
	if len(levels) != len(PyramidOrder) {
		t.Fatalf("pyramid levels = %d, want %d", len(levels), len(PyramidOrder))
	}
	byDesign := map[Design]int{}
	for _, l := range levels {
		byDesign[l.Design] = l.Count
	}
	if byDesign[DesignRCT] != 2 || byDesign[DesignMetaAnalysis] != 1 || byDesign[DesignCaseReport] != 1 {
		t.Errorf("pyramid counts wrong: %v", byDesign)
	}
	if ApexDesign(cs) != DesignMetaAnalysis {
		t.Errorf("apex = %q, want meta-analysis", ApexDesign(cs))
	}
}

func TestReconstructAbstract(t *testing.T) {
	inv := map[string][]int{"Vitamin": {0}, "D": {1}, "reduces": {2}, "risk": {3}}
	got := ReconstructAbstract(inv)
	want := "Vitamin D reduces risk"
	if got != want {
		t.Errorf("abstract = %q, want %q", got, want)
	}
	if ReconstructAbstract(nil) != "" {
		t.Errorf("nil index should yield empty string")
	}
}

func TestIsPICORelevant(t *testing.T) {
	tests := []struct {
		name              string
		abstract          string
		title             string
		ivTokens, outToks []string
		want              bool
	}{
		{
			name:     "pico_both_tokens_present",
			abstract: "A cohort of vaccinated and unvaccinated children showed increased autism rates.",
			title:    "Vaccines and autism: a population study",
			ivTokens: []string{"vaccine"}, outToks: []string{"autism"},
			want: true,
		},
		{
			name:     "pico_only_iv_present",
			abstract: "Vaccination schedules in pediatrics.",
			title:    "Study of vaccine timing",
			ivTokens: []string{"vaccine"}, outToks: []string{"autism"},
			want: false,
		},
		{
			name:     "pico_only_outcome_present",
			abstract: "Autism prevalence rose over the study period.",
			title:    "Autism trends 1990-2020",
			ivTokens: []string{"vaccine"}, outToks: []string{"autism"},
			want: false,
		},
		{
			name:     "pico_empty_tokens_bypass_gate",
			abstract: "Any text",
			title:    "Any title",
			ivTokens: nil, outToks: nil,
			want: true,
		},
		{
			name:     "pico_case_insensitive_and_stemmed",
			abstract: "VACCINATED infants were followed for AUTISM spectrum disorder.",
			title:    "",
			ivTokens: []string{"vacci"}, outToks: []string{"autis"},
			want: true,
		},
		{
			name:     "pico_token_may_come_from_title_only",
			abstract: "Children were followed for ten years.",
			title:    "Vaccine exposure and autism risk",
			ivTokens: []string{"vaccine"}, outToks: []string{"autism"},
			want: true,
		},
		{
			// Within a side the match is OR: the second token carries it.
			name:     "pico_any_token_on_a_side_suffices",
			abstract: "Low-calorie sweeteners and body weight.",
			title:    "",
			ivTokens: []string{"artif", "sweet"}, outToks: []string{"weigh", "gain"},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsPICORelevant(tt.abstract, tt.title, tt.ivTokens, tt.outToks)
			if got != tt.want {
				t.Errorf("IsPICORelevant(iv=%v, out=%v) = %v, want %v", tt.ivTokens, tt.outToks, got, tt.want)
			}
		})
	}
}

// TestIsPICORelevantFromClaim exercises the gate the way production does:
// tokens derived from the claim by PICOTokens, not hand-written.
func TestIsPICORelevantFromClaim(t *testing.T) {
	tests := []struct {
		name     string
		abstract string
		title    string
		claim    string
		want     bool
	}{
		{
			name:     "pico_head_noun_matches_without_modifier",
			abstract: "Low-calorie sweeteners and body weight: a meta-analysis",
			title:    "",
			claim:    "artificial sweeteners cause weight gain",
			want:     true,
		},
		{
			name:     "pico_modifier_only_still_needs_outcome",
			abstract: "The artificial sweetener erythritol and cardiovascular events",
			title:    "",
			claim:    "artificial sweeteners cause weight gain",
			want:     false,
		},
		{
			name:     "pico_stopwords_do_not_open_gate",
			abstract: "A study of the effects in patients and their outcomes",
			title:    "",
			claim:    "artificial sweeteners cause weight gain",
			want:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv, out := PICOTokens(tt.claim)
			got := IsPICORelevant(tt.abstract, tt.title, iv, out)
			if got != tt.want {
				t.Errorf("IsPICORelevant(%q | iv=%v out=%v) = %v, want %v",
					tt.abstract, iv, out, got, tt.want)
			}
		})
	}
}

func TestPICOTokens(t *testing.T) {
	tests := []struct {
		name            string
		claim           string
		wantIV, wantOut []string
	}{
		{
			name:   "harm claim returns every content token per side",
			claim:  "vaccines cause autism",
			wantIV: []string{"vacci"}, wantOut: []string{"autis"},
		},
		{
			name:   "modifier and head noun both kept",
			claim:  "artificial sweeteners cause weight gain",
			wantIV: []string{"artif", "sweet"}, wantOut: []string{"weigh", "gain"},
		},
		{
			name:   "unsplittable claim bypasses gate",
			claim:  "autism",
			wantIV: nil, wantOut: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			iv, out := PICOTokens(tt.claim)
			if !equalTokens(iv, tt.wantIV) || !equalTokens(out, tt.wantOut) {
				t.Errorf("PICOTokens(%q) = (%v, %v), want (%v, %v)", tt.claim, iv, out, tt.wantIV, tt.wantOut)
			}
		})
	}
}

func TestDropPICOStopwords(t *testing.T) {
	if got := dropPICOStopwords([]string{"the", "sweet", "of"}); !equalTokens(got, []string{"sweet"}) {
		t.Errorf("stopwords not dropped: %v", got)
	}
	// A side made only of stopwords falls back to the unfiltered list rather
	// than emptying out, which would bypass the gate on BOTH sides.
	if got := dropPICOStopwords([]string{"the", "of"}); !equalTokens(got, []string{"the", "of"}) {
		t.Errorf("all-stopword side should fall back, got %v", got)
	}
}

// --- confidence baseline ---------------------------------------------------
//
// confidence() had no test coverage before Phase 4. These pin the pre-Phase-4
// formula term by term so the dispersion penalty cannot silently move anything
// it is not meant to move.

const confEpsilon = 1e-6

func confNear(got, want float64) bool {
	d := got - want
	if d < 0 {
		d = -d
	}
	return d < confEpsilon
}

// TestConfidenceBaseline pins the PRE-Phase-4 formula, with the dispersion
// penalty switched off. These values were recorded against the code as it
// stood before the penalty existed, so the toggle is a genuine escape hatch:
// flipping it off must reproduce the old numbers exactly, not approximately.
func TestConfidenceBaseline(t *testing.T) {
	defer func(prev bool) { phase4ConfidenceEnabled = prev }(phase4ConfidenceEnabled)
	phase4ConfidenceEnabled = false

	tests := []struct {
		name string
		r    ConsensusResult
		want float64
	}{
		{
			// volume 10/25=0.4, apex meta-analysis rank1 -> 1-1/11, agreement 1
			name: "unanimous small meta-analysis corpus",
			r:    ConsensusResult{StudyCount: 10, ApexDesign: DesignMetaAnalysis, Supporting: 10},
			want: 0.45*0.4 + 0.30*(1-1.0/11.0) + 0.25*1,
		},
		{
			// volume capped at 1, apex RCT rank3, agreement 0 (perfect split)
			name: "evenly split large rct corpus",
			r:    ConsensusResult{StudyCount: 30, ApexDesign: DesignRCT, Supporting: 15, Refuting: 15},
			want: 0.45*1 + 0.30*(1-3.0/11.0) + 0.25*0,
		},
		{
			// volume 3/25, apex unknown rank10, agreement |2-1|/3
			name: "thin corpus unknown design",
			r:    ConsensusResult{StudyCount: 3, ApexDesign: DesignUnknown, Supporting: 2, Refuting: 1},
			want: 0.45*(3.0/25.0) + 0.30*(1-10.0/11.0) + 0.25*(1.0/3.0),
		},
		{
			// every term maxed -> hits the 0.97 ceiling
			name: "ceiling",
			r:    ConsensusResult{StudyCount: 40, ApexDesign: DesignUmbrellaReview, Supporting: 40},
			want: 0.97,
		},
		{
			name: "empty corpus",
			r:    ConsensusResult{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			directional := tt.r.Supporting + tt.r.Refuting
			got := confidence(tt.r, directional)
			if !confNear(got, tt.want) {
				t.Errorf("confidence = %.9f, want %.9f", got, tt.want)
			}
			t.Logf("BASELINE %s = %.6f", tt.name, got)
		})
	}
}

func TestStanceDispersion(t *testing.T) {
	tests := []struct {
		name                                      string
		supporting, refuting, mixed, inconclusive int
		want                                      float64
	}{
		{"unanimous supporting", 10, 0, 0, 0, 0.0},
		{"unanimous refuting", 0, 10, 0, 0, 0.0},
		{"even split", 5, 5, 0, 0, 1.0},
		{"lopsided but contested", 8, 2, 0, 0, 0.4},
		// The sharpest failure of the agreement term: it sees only directional
		// works, so this corpus scores a perfect 1.0 on agreement while knowing
		// essentially nothing. Dispersion must see it for what it is.
		{"one directional work among thirty unknowns", 1, 0, 0, 30, 1 - 1.0/31.0},
		{"mixed works count as dissent", 6, 0, 4, 0, 0.4},
		{"empty corpus is not dispersed", 0, 0, 0, 0, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stanceDispersion(tt.supporting, tt.refuting, tt.mixed, tt.inconclusive)
			if !confNear(got, tt.want) {
				t.Errorf("stanceDispersion(%d,%d,%d,%d) = %.6f, want %.6f",
					tt.supporting, tt.refuting, tt.mixed, tt.inconclusive, got, tt.want)
			}
			if got < 0 || got > 1 {
				t.Errorf("dispersion out of range: %v", got)
			}
		})
	}
}

// TestStanceDispersionMonotonic checks the property the value is meant to have:
// holding the corpus size fixed, moving works away from the dominant stance
// never lowers dispersion.
func TestStanceDispersionMonotonic(t *testing.T) {
	prev := -1.0
	for refuting := 0; refuting <= 10; refuting++ {
		got := stanceDispersion(20-refuting, refuting, 0, 0)
		if got < prev {
			t.Fatalf("dispersion dropped as dissent grew: refuting=%d gave %.4f after %.4f",
				refuting, got, prev)
		}
		prev = got
	}
}

// TestConfidencePhase4 pins the post-penalty values for the same inputs the
// baseline test uses, so the delta the penalty introduces is explicit.
func TestConfidencePhase4(t *testing.T) {
	if !phase4ConfidenceEnabled {
		t.Fatal("phase4ConfidenceEnabled must default to true in production")
	}
	tests := []struct {
		name string
		r    ConsensusResult
		want float64
	}{
		{
			// Unanimous: dispersion 0, so the penalty is a no-op and the
			// baseline value survives untouched.
			name: "unanimous corpus is not penalized",
			r:    ConsensusResult{StudyCount: 10, ApexDesign: DesignMetaAnalysis, Supporting: 10},
			want: 0.45*0.4 + 0.30*(1-1.0/11.0) + 0.25*1,
		},
		{
			// Perfectly split: dispersion 1, so the full weight applies.
			name: "evenly split corpus loses the full weight",
			r:    ConsensusResult{StudyCount: 30, ApexDesign: DesignRCT, Supporting: 15, Refuting: 15},
			want: (0.45*1 + 0.30*(1-3.0/11.0)) * (1 - dispersionWeight),
		},
		{
			name: "thin contested corpus",
			r:    ConsensusResult{StudyCount: 3, ApexDesign: DesignUnknown, Supporting: 2, Refuting: 1},
			want: (0.45*(3.0/25.0) + 0.30*(1-10.0/11.0) + 0.25*(1.0/3.0)) *
				(1 - dispersionWeight*(1-1.0/3.0)),
		},
		{
			name: "ceiling still applies after the penalty",
			r:    ConsensusResult{StudyCount: 40, ApexDesign: DesignUmbrellaReview, Supporting: 40},
			want: 0.97,
		},
		{
			name: "empty corpus",
			r:    ConsensusResult{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := confidence(tt.r, tt.r.Supporting+tt.r.Refuting)
			if !confNear(got, tt.want) {
				t.Errorf("confidence = %.9f, want %.9f", got, tt.want)
			}
		})
	}
}

// TestConfidenceSmallUnanimousBeatsLargeDivided is the behavior Phase 4 exists
// for: study count is not evidence quality. Ten works that agree must outrank
// a hundred that cancel out.
func TestConfidenceSmallUnanimousBeatsLargeDivided(t *testing.T) {
	small := ConsensusResult{StudyCount: 10, ApexDesign: DesignRCT, Supporting: 10}
	large := ConsensusResult{StudyCount: 100, ApexDesign: DesignRCT, Supporting: 50, Refuting: 50}

	smallConf := confidence(small, small.Supporting+small.Refuting)
	largeConf := confidence(large, large.Supporting+large.Refuting)
	if smallConf <= largeConf {
		t.Errorf("small unanimous corpus (%.4f) should outrank large divided corpus (%.4f)",
			smallConf, largeConf)
	}

	// Without the penalty the ordering is the wrong way round — that inversion
	// is the defect Phase 4 corrects, and pinning it here documents why the
	// penalty cannot simply be deleted.
	defer func(prev bool) { phase4ConfidenceEnabled = prev }(phase4ConfidenceEnabled)
	phase4ConfidenceEnabled = false
	if confidence(small, 10) >= confidence(large, 100) {
		t.Errorf("pre-Phase-4 formula was expected to rank the large divided corpus higher; "+
			"small=%.4f large=%.4f", confidence(small, 10), confidence(large, 100))
	}
}

// TestConfidenceInconclusiveMassIsPenalized covers the case the agreement term
// is blind to: one directional work surrounded by works that concluded nothing.
func TestConfidenceInconclusiveMassIsPenalized(t *testing.T) {
	r := ConsensusResult{StudyCount: 31, ApexDesign: DesignCohort, Supporting: 1, Inconclusive: 30}

	withPenalty := confidence(r, 1)

	defer func(prev bool) { phase4ConfidenceEnabled = prev }(phase4ConfidenceEnabled)
	phase4ConfidenceEnabled = false
	withoutPenalty := confidence(r, 1)

	if withPenalty >= withoutPenalty {
		t.Errorf("a corpus of 1 supporting + 30 inconclusive must lose confidence: "+
			"with=%.4f without=%.4f", withPenalty, withoutPenalty)
	}
}

// TestConsensusEmptyApexIsUnclassified pins the zero-work path. The early
// return skips ApexDesign(), so the field has to be seeded — otherwise the
// result marshals "apex_design": "", which is not a design and cannot be
// mapped to a tier by any consumer.
func TestConsensusEmptyApexIsUnclassified(t *testing.T) {
	res := Consensus(nil)
	if res.ApexDesign != DesignUnknown {
		t.Errorf("ApexDesign on empty corpus = %q, want %q", res.ApexDesign, DesignUnknown)
	}
	if res.ApexDesign == "" {
		t.Error("ApexDesign is the empty string — the seed is missing")
	}
	// Everything else on this path must be untouched.
	if res.StudyCount != 0 || res.Verdict != VerdictInsufficient || res.EvidenceStrength != StrengthVeryLow {
		t.Errorf("empty-corpus result drifted: %+v", res)
	}
	if res.ConsensusScore != 0 || res.Confidence != 0 {
		t.Errorf("empty corpus must score 0/0, got %.4f/%.4f", res.ConsensusScore, res.Confidence)
	}
}

// TestConsensusApexUnchangedForNonEmpty guards the seed against changing the
// apex a real corpus reports.
func TestConsensusApexUnchangedForNonEmpty(t *testing.T) {
	works := []ScoredWork{
		{Stance: StanceSupporting, Design: DesignCohort, StanceConf: 0.9, CitedBy: 10},
		{Stance: StanceSupporting, Design: DesignMetaAnalysis, StanceConf: 0.9, CitedBy: 5},
		{Stance: StanceRefuting, Design: DesignCaseReport, StanceConf: 0.9, CitedBy: 1},
	}
	if got := Consensus(works).ApexDesign; got != DesignMetaAnalysis {
		t.Errorf("apex = %q, want %q", got, DesignMetaAnalysis)
	}
}
