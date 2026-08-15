package scengine

import (
	"strings"
	"testing"
)

// TestTitleSupportCueIsNotAFinding pins the title-cue gate on the harm branch:
// a benefit word in the TITLE names the paper's subject, not its result, so it
// must not count as evidence against a harm claim. nullCues titles stay
// unaffected — a title CAN state a negative finding, and those count on a
// different branch.
//
// The smoking case is the live measurement that motivated the gate: before it,
// the single title cue "prevention" scored the paper refuting at 0.90 while
// nullCues and harmCues fired zero times.
func TestTitleSupportCueIsNotAFinding(t *testing.T) {
	cases := []struct {
		name     string
		claim    string
		title    string
		abstract string
		want     Stance
		why      string
	}{
		{
			name:  "benefit cue in title is the subject, not the finding",
			claim: "smoking causes lung cancer",
			title: "Cigarette smoking and lung cancer: chemical mechanisms and approaches to prevention",
			abstract: "Cigarette smoke contains a mixture of carcinogens. This review describes " +
				"the chemical mechanisms by which these compounds form DNA adducts in the " +
				"lung and how those adducts drive tumour development.",
			want: StanceInconclusive,
			why:  "\"prevention\" names the paper's topic; the abstract reports the carcinogenic mechanism",
		},
		{
			name:  "same benefit cue in the abstract still counts against the claim",
			claim: "smoking causes lung cancer",
			title: "Cigarette smoking and lung cancer",
			abstract: "In this cohort, smoking cessation was effective and lowered the risk " +
				"of lung cancer within five years.",
			want: StanceRefuting,
			why:  "the gate is scoped to the title sentence only; abstract cues are untouched",
		},
		{
			name:  "null cue in the title is a real finding and still counts",
			claim: "HPV vaccine causes infertility",
			title: "No association between HPV vaccination and infertility",
			abstract: "We compared time to pregnancy among vaccinated and unvaccinated women " +
				"in a nationwide registry cohort.",
			want: StanceRefuting,
			why:  "nullCues are deliberately outside this gate",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hay := strings.ToLower(normalizeText(tc.title + ". " + tc.abstract))
			got, conf := classifyAgainstHarmClaim(hay, tc.claim)
			if got != tc.want {
				t.Errorf("stance = %q (conf %.2f), want %q\n  %s", got, conf, tc.want, tc.why)
			}
		})
	}
}

// TestTitleCueGateCorpusDelta pins the measured blast radius of the gate across
// the archived harm corpora: exactly one work changes stance, "Coffee and tea
// on cardiovascular disease (CVD) prevention", refuting -> inconclusive. If a
// later cue change moves more works than this, that is a signal to re-measure,
// not to widen the expectation silently.
func TestTitleCueGateCorpusDelta(t *testing.T) {
	want := map[string][4]int{
		// corpus -> {supporting, refuting, mixed, inconclusive}
		"cellphones":     {6, 7, 0, 18},
		"coffee":         {0, 3, 1, 7},
		"hpvinfertility": {2, 9, 0, 18},
		"redmeat_run1":   {5, 2, 0, 4},
		"redmeat_run2":   {5, 2, 0, 4},
		"sweeteners":     {0, 4, 0, 3},
		"vaccines":       {1, 14, 1, 21},
	}

	for name, exp := range want {
		t.Run(name, func(t *testing.T) {
			full := mustLoadFullCorpus(t, name)
			counts := map[Stance]int{}
			for _, s := range full.AllStudies {
				hay := strings.ToLower(normalizeText(s.Title + ". " + s.Abstract))
				st, _ := classifyAgainstHarmClaim(hay, full.Claim)
				counts[st]++
			}
			got := [4]int{
				counts[StanceSupporting], counts[StanceRefuting],
				counts[StanceMixed], counts[StanceInconclusive],
			}
			if got != exp {
				t.Errorf("%s: sup/ref/mix/inc = %v, want %v (of %d works)",
					name, got, exp, len(full.AllStudies))
			}
		})
	}
}
