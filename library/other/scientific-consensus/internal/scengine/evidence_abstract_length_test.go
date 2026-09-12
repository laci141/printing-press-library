// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.

package scengine

import (
	"strings"
	"testing"
)

// The heuristic tier reads title + ". " + abstract, and OpenAlex's
// abstract_inverted_index does not reliably stop where the abstract stops. On
// 10.7326/0003-4819-55-1-33 it runs 23,238 characters into the paper's own
// reference list, where a CITED paper's title supplies "a systematic review
// and meta-analysis of randomized controlled trials" at character 8,727. That
// one phrase classified the Framingham six-year follow-up cohort as a
// meta-analysis, tier 9 instead of 5, under an omega-3 claim published decades
// after it.
//
// These tests pin the cap that stops it, and the two boundaries it must not
// cross: a work whose OWN abstract names its design early still classifies,
// and an authoritative publication type is never affected at any length.

// realAbstractPrefix stands in for the readable part of an abstract: prose with
// no design term in it, so only the tail can decide the classification.
const realAbstractPrefix = "Increasingly reliable estimates of the prevalence and " +
	"incidence of coronary heart disease emphasize the importance of this disease as a " +
	"contemporary health hazard. Cardiovascular disease is now the leading cause of death, " +
	"with coronary heart disease accounting for two-thirds of all heart disease deaths. "

// citedPaperTitle is the phrase measured in the Kannel reference list. It names
// another paper's design, not this work's.
const citedPaperTitle = "Efficacy and safety of non-vitamin K antagonist oral " +
	"anticoagulants combined with antiplatelet drugs for patients with peripheral artery " +
	"disease: a systematic review and meta-analysis of randomized controlled trials"

// kannelLikeAbstract reproduces the measured shape: real abstract, then filler,
// then a cited paper's title well beyond the cap.
func kannelLikeAbstract() string {
	filler := strings.Repeat("further discussion of risk factors and their measurement. ", 200)
	return realAbstractPrefix + filler + citedPaperTitle
}

func TestHeuristicsIgnoreDesignTermsPastTheCap(t *testing.T) {
	const title = "Factors of Risk in the Development of Coronary Heart Disease" +
		"\u2014Six-Year Follow-up Experience"

	abstract := kannelLikeAbstract()
	if len(abstract) <= maxAbstractForHeuristics {
		t.Fatalf("fixture is not long enough to exercise the cap: %d bytes, cap %d",
			len(abstract), maxAbstractForHeuristics)
	}
	if idx := strings.Index(abstract, "meta-analysis"); idx <= maxAbstractForHeuristics {
		t.Fatalf("fixture puts the cited design term at %d, inside the cap — it would "+
			"be read whether or not the cap works, so this test would prove nothing", idx)
	}

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design == DesignMetaAnalysis {
		t.Errorf("a design term in a cited paper's title, %d bytes into the abstract, "+
			"classified the work as %s", strings.Index(abstract, "meta-analysis"), got.Design)
	}
	if got.Design != DesignUnknown {
		t.Errorf("Design = %q, want %q: nothing in the title or the real abstract names "+
			"a design", got.Design, DesignUnknown)
	}
}

func TestHeuristicsStillReadTheWorksOwnAbstract(t *testing.T) {
	// The counterpart the cap must not break. A real meta-analysis says so in
	// its own abstract, near the front, and a long tail must not hide that.
	const title = "Dietary sugars and body weight"
	abstract := "OBJECTIVE: To summarise evidence on the association between intake of " +
		"dietary sugars and body weight in adults. DESIGN: Systematic review and " +
		"meta-analysis of randomised controlled trials. " +
		strings.Repeat("results and discussion follow at length. ", 300)

	if len(abstract) <= maxAbstractForHeuristics {
		t.Fatalf("fixture must exceed the cap to prove the cap is not the reason it "+
			"passes: %d bytes, cap %d", len(abstract), maxAbstractForHeuristics)
	}

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignMetaAnalysis {
		t.Errorf("Design = %q, want %q: the work names its own design at character %d, "+
			"well inside the cap", got.Design, DesignMetaAnalysis,
			strings.Index(abstract, "meta-analysis"))
	}
	if got.Method != MethodHeuristic {
		t.Errorf("Method = %q, want %q", got.Method, MethodHeuristic)
	}
}

func TestAuthoritativePubTypeIsUnaffectedByTheCap(t *testing.T) {
	// The cap sits in the heuristic tier only. A source that names the design
	// returns before the abstract is read at all, so abstract length cannot
	// change the answer.
	const title = "Some work with an enormous abstract"
	abstract := kannelLikeAbstract()

	got := ClassifyDesign(title, abstract, "article", []string{"Randomized Controlled Trial"})
	if got.Design != DesignRCT {
		t.Errorf("Design = %q, want %q", got.Design, DesignRCT)
	}
	if got.Method != MethodPubMedPubType {
		t.Errorf("Method = %q, want %q: an authoritative type must win before the "+
			"abstract is consulted", got.Method, MethodPubMedPubType)
	}
}

func TestCapLeavesShortAbstractsUntouched(t *testing.T) {
	// Median corpus abstract is 1,547 bytes and the 90th percentile 2,547, so
	// the overwhelming majority never reach the cap. A term anywhere in a
	// normal-length abstract must still be read.
	const title = "Coffee consumption and coronary heart disease in women"
	abstract := strings.Repeat("background and methods. ", 40) +
		"DESIGN: Prospective cohort study with consumption measured over ten years."

	if len(abstract) >= maxAbstractForHeuristics {
		t.Fatalf("fixture is meant to sit under the cap: %d bytes, cap %d",
			len(abstract), maxAbstractForHeuristics)
	}

	got := ClassifyDesign(title, abstract, "article", nil)
	if got.Design != DesignCohort {
		t.Errorf("Design = %q, want %q", got.Design, DesignCohort)
	}
}
