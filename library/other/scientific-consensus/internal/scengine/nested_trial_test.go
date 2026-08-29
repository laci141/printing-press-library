// Hand-authored tests for the host-trial guard in ClassifyDesign: an abstract
// that credits a DIFFERENT study for the randomisation must not make this work
// an RCT. Not generated.
package scengine

import "testing"

// The abstract of 10.1371/journal.pone.0045231, trimmed to the sentence that
// caused the misclassification. Its title ends "A Case Control Study"; the
// abstract names the trial it sat inside. Before the guard, the RCT heuristic
// matched that phrase and promoted the work eight tiers, from case-control to
// the apex of the pyramid, where it then carried a 0.90-confidence stance.
const nestedCaseControlTitle = "Reasons for Receiving or Not Receiving HPV Vaccination in Primary Schoolgirls in Tanzania: A Case Control Study"
const nestedCaseControlAbstract = "Background: There is little evidence on the acceptability of HPV vaccination in sub-Saharan Africa. Methods: We conducted a case-control study among girls within a phase IV cluster-randomised trial of HPV vaccination in Tanzania, comparing those who received and did not receive the vaccine."

func TestClassifyDesignIgnoresHostTrialRandomisation(t *testing.T) {
	got := ClassifyDesign(nestedCaseControlTitle, nestedCaseControlAbstract, "article", nil)
	if got.Design != DesignCaseControl {
		t.Errorf("Design = %s, want %s — the randomisation belongs to the host trial, not this work",
			got.Design, DesignCaseControl)
	}
}

// A work that calls itself randomised in its own TITLE is stating its own
// design. A host-trial phrase elsewhere in the abstract must not overrule that,
// or a genuine RCT reporting a sub-analysis would be demoted.
func TestClassifyDesignKeepsRCTWhenTitleSaysSo(t *testing.T) {
	title := "Effect of vitamin D on fracture risk: a randomised controlled trial"
	abstract := "Participants were recruited within a larger placebo-controlled trial of calcium supplementation."

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — the title states this work's own design",
			got.Design, DesignRCT)
	}
}

// The guard must not fire on an ordinary RCT abstract. "Randomised" without a
// phrase crediting another study is this work's own randomisation.
func TestClassifyDesignUnaffectedByPlainRCTAbstract(t *testing.T) {
	title := "Melatonin for shift work sleep disorder"
	abstract := "We randomised 120 night-shift nurses to melatonin or placebo in a double-blind trial."

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — nothing here credits another study",
			got.Design, DesignRCT)
	}
}

// The guard sits on the heuristic path only. An authoritative publication type
// still wins, which is the documented cascade and is why the live HPV query
// still reports this work as an RCT: OpenAlex indexes it under the host trial's
// type. Pinned so the limit is visible rather than discovered later.
func TestClassifyDesignHostTrialGuardDoesNotOverridePubType(t *testing.T) {
	got := ClassifyDesign(nestedCaseControlTitle, nestedCaseControlAbstract, "article",
		[]string{"Randomized Controlled Trial"})
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — a publication type outranks the heuristics",
			got.Design, DesignRCT)
	}
	if got.Method != MethodPubMedPubType {
		t.Errorf("Method = %s, want %s", got.Method, MethodPubMedPubType)
	}
}

// An abstract can name the trial its participants came from AND describe this
// work's own randomisation. Suppressing the RCT tier work-wide would discard
// the second along with the first, demoting a genuine RCT for having said
// where its participants came from.
func TestClassifyDesignKeepsRCTWhenOwnRandomisationIsSeparate(t *testing.T) {
	title := "Vitamin D supplementation and fracture risk in older adults"
	abstract := "Participants were drawn from within a larger placebo-controlled trial of calcium. " +
		"We then randomised 200 of them to vitamin D or placebo in a double-blind design."

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — the second sentence is this work's own randomisation",
			got.Design, DesignRCT)
	}
}

// The determiner separates a host trial from the work describing itself.
// "enrolled in THIS randomised trial" is an RCT stating its own design; an
// earlier version of the pattern demoted one for saying so.
func TestClassifyDesignKeepsRCTWhenPhraseSaysThis(t *testing.T) {
	title := "The ocular hypotensive effect of saffron extract in primary open angle glaucoma"
	abstract := "Thirty-four patients were enrolled in this prospective, comparative, randomized clinical trial."

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — \"this\" marks the work's own trial, not a host",
			got.Design, DesignRCT)
	}
}

// The host mention and this work's own randomisation can share one sentence.
// An earlier version dropped the whole sentence it found a host phrase in, so
// the second signal went with the first and a genuine RCT was demoted.
func TestClassifyDesignKeepsRCTWhenBothSignalsShareASentence(t *testing.T) {
	title := "Vitamin D supplementation and fracture risk in older adults"
	abstract := "Participants were drawn from within a larger placebo-controlled trial and we then randomised 200 of them in a double-blind design."

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignRCT {
		t.Errorf("Design = %s, want %s — the same sentence reports this work's own allocation",
			got.Design, DesignRCT)
	}
}
