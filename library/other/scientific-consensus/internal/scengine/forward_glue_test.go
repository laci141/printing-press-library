package scengine

import (
	"strings"
	"testing"
)

// TestForwardGlueIntervention pins the asymmetry between the two sides: a
// leading short qualifier is kept on the intervention side and dropped on the
// outcome side. Both halves matter. Losing the first re-opens the gate on
// abbreviation-named interventions; losing the second reverses the measured
// 2026-08-02 decision on "gut health".
func TestForwardGlueIntervention(t *testing.T) {
	cases := []struct {
		claim  string
		wantIV string
		reason string
	}{
		{
			claim:  "HPV vaccine causes infertility",
			wantIV: "hpv vaccine",
			reason: "measured live 2026-08-05: iv=[vacci] matched vaccine-hesitancy studies",
		},
		{
			claim:  "MMR vaccine causes autism",
			wantIV: "mmr vaccine",
			reason: "same shape, different abbreviation",
		},
		{
			claim:  "IUD causes infertility",
			wantIV: "iud",
			reason: "the whole intervention is the abbreviation; empty tokens bypass the gate",
		},
		{
			claim:  "CBT reduces PTSD symptoms",
			wantIV: "cbt",
			reason: "gate was fully off for this claim before",
		},
		{
			claim:  "HRT increases breast cancer risk",
			wantIV: "hrt",
			reason: "gate was fully off for this claim before",
		},
		{
			claim:  "red meat causes colon cancer",
			wantIV: "red meat",
			reason: "the only archived corpus this tightens; excludes 1 of 11, correctly",
		},
		{
			claim:  "vitamin C prevents the common cold",
			wantIV: "vitamin c",
			reason: "trailing qualifier still glues backwards, and the phrase is not stemmed",
		},
		{
			claim:  "vitamin D reduces respiratory infections",
			wantIV: "vitamin d",
			reason: "unchanged",
		},
		{
			claim:  "cell phones cause brain cancer",
			wantIV: "cell|phone",
			reason: "4 characters clears the short-token floor; nothing is held",
		},
		{
			claim:  "coffee causes heart disease",
			wantIV: "coffe",
			reason: "no qualifier at all; plain stem",
		},
		{
			claim:  "omega-3 improves cardiovascular health",
			wantIV: "omega",
			reason: "numeric qualifiers are still dropped, never held",
		},
	}
	for _, tc := range cases {
		t.Run(tc.claim, func(t *testing.T) {
			iv, _ := PICOTokens(tc.claim)
			if got := strings.Join(iv, "|"); got != tc.wantIV {
				t.Errorf("intervention tokens = %q, want %q\n  %s", got, tc.wantIV, tc.reason)
			}
		})
	}
}

// TestOutcomeSideStillDropsLeadingQualifier is the other half. If this ever
// starts returning "gut health", the 2026-08-02 measurement has been silently
// reversed: that token excluded 7 of 9 probiotics works against 2 of 9 for the
// loose stem.
func TestOutcomeSideStillDropsLeadingQualifier(t *testing.T) {
	cases := []struct {
		claim   string
		wantOut string
	}{
		{"probiotics improve gut health", "healt"},
		{"vitamin C prevents the common cold", "commo|cold"},
		{"HPV vaccine causes infertility", "infer"},
		{"HRT increases breast cancer risk", "breas|cance"},
	}
	for _, tc := range cases {
		t.Run(tc.claim, func(t *testing.T) {
			_, out := PICOTokens(tc.claim)
			if got := strings.Join(out, "|"); got != tc.wantOut {
				t.Errorf("outcome tokens = %q, want %q", got, tc.wantOut)
			}
		})
	}
}
