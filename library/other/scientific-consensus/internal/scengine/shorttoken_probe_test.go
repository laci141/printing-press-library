package scengine

import (
	"testing"
)

// TestShortTokenGuard is the regression guard on picoSideTokens' short-qualifier
// gluing. It asserts against the REAL PICOTokens output, not against a
// hardcoded string, so reverting the glue (or reintroducing the bare "vitam"
// stem) makes this test fail rather than silently print a different number.
//
// Background: ClaimContentTokens drops anything under four characters, so
// "vitamin D" once reduced to "vitam" — which matches vitamin C, vitamin A,
// vitamin K and riboflavin just as happily. Measured on the vitamind corpus:
// 4 of 26 works matched "vitam" while never naming vitamin D, two of them
// vitamin C papers that then voted on the vitamin D verdict.
//
// picoSideTokens now glues a 1-3 character alphabetic token to the preceding
// content token, so the claim yields the phrase token "vitamin d". These cases
// pin that behavior on real titles from the corpus.
func TestShortTokenGuard(t *testing.T) {
	const claim = "vitamin D reduces respiratory infections"

	iv, out := PICOTokens(claim)
	if len(iv) == 0 || len(out) == 0 {
		t.Fatalf("PICOTokens(%q) returned empty sides iv=%v out=%v — the gate would bypass itself", claim, iv, out)
	}

	// The intervention side must carry the qualifier, not the bare stem.
	// If this fails, the glue was reverted and every vitamin paper leaks.
	if !containsAnyToken("vitamin d", iv) {
		t.Errorf("intervention tokens %v do not match the phrase \"vitamin d\" — "+
			"short-qualifier gluing is not in effect", iv)
	}
	t.Logf("claim=%q -> iv=%v out=%v", claim, iv, out)

	cases := []struct {
		name     string
		title    string
		abstract string
		want     bool // should this work pass the PICO gate for the vitamin D claim?
	}{
		// Measured leaks from the vitamind corpus — these must be EXCLUDED.
		{"vitamin C immune", "Vitamin C and Immune Function", "ascorbic acid supports respiratory infection defence", false},
		{"vitamin C ultramarathon", "Vitamin C supplementation reduces the incidence of postrace symptoms of upper-respiratory-tract infection in ultramarathon runners", "", false},
		{"micronutrients review", "A Review of Micronutrients and the Immune System", "reduce the risk of respiratory infection", false},
		{"optimal nutrition", "Optimal Nutritional Status for a Well-Functioning Immune System", "protects against viral respiratory infections", false},

		// Controls — these must KEEP passing. A fix judged only on the leaks
		// is not a fix; losing these would be a regression.
		{"vitamin D plain", "Vitamin D supplementation and respiratory infection", "", true},
		{"vitamin D3", "Vitamin D3 supplementation in adults", "reduced respiratory infection rates", true},
		{"vitamin D lowercase", "vitamin d deficiency and immune response", "respiratory infection outcomes", true},
	}

	for _, c := range cases {
		got := IsPICORelevant(c.abstract, c.title, iv, out)
		if got != c.want {
			t.Errorf("%s: IsPICORelevant = %v, want %v (title=%q)", c.name, got, c.want, c.title)
			continue
		}
		t.Logf("%-26s gate=%-5v OK", c.name, got)
	}
}

// TestShortTokenGuardOmega3 pins the deliberate NON-gluing of numeric
// qualifiers. omega-3 and omega-6 are different fatty acids, so gluing looks
// attractive here too — but it does not work: in abstract text "omega-3" is
// written with a hyphen, and a glued token "omega 3" cannot bridge it via
// strings.Contains. The "omega" stem is therefore kept deliberately.
//
// This test documents that decision as a measurement, so a future change that
// starts gluing digits has to confront the omega-6 leak it does NOT fix.
func TestShortTokenGuardOmega3(t *testing.T) {
	const claim = "omega-3 improves cardiovascular health"

	iv, out := PICOTokens(claim)
	if len(iv) == 0 || len(out) == 0 {
		t.Fatalf("PICOTokens(%q) returned empty sides iv=%v out=%v", claim, iv, out)
	}
	t.Logf("claim=%q -> iv=%v out=%v", claim, iv, out)

	// The digit is NOT glued: the intervention side stays the bare stem.
	if containsAnyToken("omega 3", iv) && !containsAnyToken("omega-3", iv) {
		t.Errorf("intervention tokens %v glued the digit as \"omega 3\" — "+
			"that form cannot match the hyphenated \"omega-3\" in abstract text", iv)
	}

	cases := []struct {
		name     string
		title    string
		abstract string
		want     bool
	}{
		{"omega-3 hyphen", "omega-3 fatty acids and cardiovascular health", "", true},
		{"omega 3 space", "omega 3 supplementation trial", "cardiovascular health outcomes", true},

		// KNOWN LIMIT, measured and accepted: "omega" cannot separate omega-6
		// from omega-3. This passes the gate today. The test asserts the known
		// behavior rather than the desired one, so the gap stays visible.
		{"omega-6 known leak", "omega-6 fatty acids and inflammation", "cardiovascular health markers", true},
	}

	for _, c := range cases {
		got := IsPICORelevant(c.abstract, c.title, iv, out)
		if got != c.want {
			t.Errorf("%s: IsPICORelevant = %v, want %v (title=%q)", c.name, got, c.want, c.title)
			continue
		}
		t.Logf("%-20s gate=%-5v OK", c.name, got)
	}
}
