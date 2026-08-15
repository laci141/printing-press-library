package scengine

import (
	"strings"
	"testing"
)

// TestSupportCueNeedsClaimPairing pins the 2026-08-05 change: against a harm
// claim, a benefit word only counts as evidence against the claim when it sits
// in the same sentence-clipped window as both claim sides.
//
// The abstracts here are shortened from works measured live that day.
//
// Each case carries an explicit title because ClassifyStance always builds its
// haystack as title + ". " + abstract, and the title-cue gate keys off the
// first sentence. Feeding a bare abstract would make sentence one look like a
// title and measure a code path production never takes.
func TestSupportCueNeedsClaimPairing(t *testing.T) {
	const claim = "HPV vaccine causes infertility"

	cases := []struct {
		name  string
		title string
		text  string
		want  Stance
		why   string
	}{
		{
			name:  "background benefit about a different outcome",
			title: "Religious beliefs and HPV vaccine acceptance in Islamic countries: a scoping review",
			text: "Despite the availability of effective HPV vaccines, their acceptance " +
				"in Islamic countries is often influenced by religious beliefs and " +
				"misconceptions. This review identified the current literature on " +
				"religious beliefs toward HPV vaccine acceptance.",
			want: StanceInconclusive,
			why:  "effective refers to cervical cancer prevention; the review measures acceptance, not fertility",
		},
		{
			name:  "uptake study with no fertility finding",
			title: "Factors influencing HPV vaccination uptake in sub-Saharan Africa: a case control study",
			text: "There are few data on factors influencing human papillomavirus HPV " +
				"vaccination uptake in sub-Saharan Africa. We conducted a case control " +
				"study of HPV vaccine receivers and non-receivers within a phase IV " +
				"cluster-randomised trial of effective HPV vaccination in 134 schools.",
			want: StanceInconclusive,
			why:  "scored refuting at 0.90 before this change, on background vocabulary alone",
		},
		{
			name:  "benefit reported about the claim's own outcome still counts",
			title: "HPV vaccination and male fertility: a prospective cohort",
			text: "HPV vaccination is effective in preventing infertility in men, and " +
				"vaccinated men showed no impairment of sperm parameters compared " +
				"with unvaccinated controls.",
			want: StanceRefuting,
			why:  "here the benefit word and both claim sides share a sentence, which is the whole point of the gate",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hay := strings.ToLower(normalizeText(tc.title + ". " + tc.text))
			got, _ := classifyAgainstHarmClaim(hay, claim)
			if got != tc.want {
				t.Errorf("stance = %q, want %q\n  %s\n  text: %s", got, tc.want, tc.why, tc.text[:80])
			}
		})
	}
}

// TestHarmCorporaKeepTheirDirection is the safety net. The support-pairing gate
// removes a lot of negative counts, and the risk is that a corpus which
// correctly refutes its claim loses the majority that made it do so. Measured
// 2026-08-05: vaccines 17 refuting to 14 against 1 supporting, coffee 6 to 4,
// redmeat 4 to 2, sweeteners 5 to 4. Each still refutes.
func TestHarmCorporaKeepTheirDirection(t *testing.T) {
	for _, tc := range []struct {
		corpus     string
		minRefute  int
		maxSupport int
	}{
		{"vaccines", 10, 3},
		{"coffee", 3, 2},
		{"redmeat_run1", 2, 6},
		{"sweeteners", 3, 2},
	} {
		t.Run(tc.corpus, func(t *testing.T) {
			full := mustLoadFullCorpus(t, tc.corpus)
			var sup, ref int
			for _, s := range full.AllStudies {
				hay := strings.ToLower(s.Title + ". " + s.Abstract)
				st, _ := classifyAgainstHarmClaim(hay, full.Claim)
				switch st {
				case StanceSupporting:
					sup++
				case StanceRefuting:
					ref++
				}
			}
			if ref < tc.minRefute {
				t.Errorf("%s: refuting=%d, want at least %d — the gate cut too deep", tc.corpus, ref, tc.minRefute)
			}
			if sup > tc.maxSupport {
				t.Errorf("%s: supporting=%d, want at most %d", tc.corpus, sup, tc.maxSupport)
			}
			t.Logf("%s: supporting=%d refuting=%d of %d works", tc.corpus, sup, ref, len(full.AllStudies))
		})
	}
}
