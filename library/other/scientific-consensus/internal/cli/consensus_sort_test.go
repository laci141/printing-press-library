// Hand-authored tests for the Phase 5 evidence-tier-first card ordering.
package cli

import (
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// ws builds a supporting workStance with a given design and citation count.
func ws(title string, design scengine.Design, cites int) workStance {
	return workStance{
		Work:   scWork{Title: title, CitedBy: cites, ID: title},
		Stance: scengine.StanceSupporting,
		Design: design,
	}
}

func titlesOf(briefs []workBrief) []string {
	out := make([]string, 0, len(briefs))
	for _, b := range briefs {
		out = append(out, b.Title)
	}
	return out
}

func sameTitles(got []workBrief, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Title != want[i] {
			return false
		}
	}
	return true
}

func TestTopByStanceTierFirst(t *testing.T) {
	// The cohort study has ten times the citations of the meta-analysis, which
	// is the age effect the tier key exists to override.
	stances := []workStance{
		ws("cohort-old", scengine.DesignCohort, 500),
		ws("meta-new", scengine.DesignMetaAnalysis, 50),
		ws("rct-mid", scengine.DesignRCT, 200),
	}
	got := topByStance(stances, scengine.StanceSupporting, 3)
	want := []string{"meta-new", "rct-mid", "cohort-old"}
	if !sameTitles(got, want) {
		t.Errorf("order = %v, want %v", titlesOf(got), want)
	}
}

func TestTopByStanceCitationsBreakTies(t *testing.T) {
	stances := []workStance{
		ws("rct-few-cites", scengine.DesignRCT, 10),
		ws("rct-many-cites", scengine.DesignRCT, 900),
		ws("rct-mid-cites", scengine.DesignRCT, 100),
	}
	got := topByStance(stances, scengine.StanceSupporting, 3)
	want := []string{"rct-many-cites", "rct-mid-cites", "rct-few-cites"}
	if !sameTitles(got, want) {
		t.Errorf("order = %v, want %v", titlesOf(got), want)
	}
}

func TestTopByStanceUnclassifiedSinksToBottom(t *testing.T) {
	stances := []workStance{
		ws("unclassified-huge", scengine.DesignUnknown, 5000),
		ws("case-report", scengine.DesignCaseReport, 1),
		ws("meta", scengine.DesignMetaAnalysis, 2),
	}
	got := topByStance(stances, scengine.StanceSupporting, 3)
	want := []string{"meta", "case-report", "unclassified-huge"}
	if !sameTitles(got, want) {
		t.Errorf("order = %v, want %v", titlesOf(got), want)
	}
}

// TestTopByStanceToggleOffIsCitationOnly is the regression guard: with the
// switch off the list must be exactly what the pre-Phase-5 code produced.
func TestTopByStanceToggleOffIsCitationOnly(t *testing.T) {
	stances := []workStance{
		ws("cohort-old", scengine.DesignCohort, 500),
		ws("meta-new", scengine.DesignMetaAnalysis, 50),
		ws("rct-mid", scengine.DesignRCT, 200),
	}
	defer func(prev bool) { phase5SortEnabled = prev }(phase5SortEnabled)
	phase5SortEnabled = false

	got := topByStance(stances, scengine.StanceSupporting, 3)
	want := []string{"cohort-old", "rct-mid", "meta-new"}
	if !sameTitles(got, want) {
		t.Errorf("toggle-off order = %v, want pre-Phase-5 citation order %v", titlesOf(got), want)
	}
}

// TestTopByStanceCutChangesMembership documents the accepted consequence: the
// cut to N happens after the sort, so tier-first ordering changes WHICH works
// reach the cards, not merely their sequence.
func TestTopByStanceCutChangesMembership(t *testing.T) {
	stances := []workStance{
		ws("cohort-a", scengine.DesignCohort, 900),
		ws("cohort-b", scengine.DesignCohort, 800),
		ws("cohort-c", scengine.DesignCohort, 700),
		ws("meta-new", scengine.DesignMetaAnalysis, 5),
	}
	withTier := topByStance(stances, scengine.StanceSupporting, 3)
	if !sameTitles(withTier, []string{"meta-new", "cohort-a", "cohort-b"}) {
		t.Errorf("tier-first top-3 = %v", titlesOf(withTier))
	}

	defer func(prev bool) { phase5SortEnabled = prev }(phase5SortEnabled)
	phase5SortEnabled = false
	citationOnly := topByStance(stances, scengine.StanceSupporting, 3)
	if !sameTitles(citationOnly, []string{"cohort-a", "cohort-b", "cohort-c"}) {
		t.Errorf("citation-only top-3 = %v", titlesOf(citationOnly))
	}
}

// TestTopByStanceDoesNotReorderInput protects all_studies: it is built from the
// same stances slice and must keep fetch (relevance) order.
func TestTopByStanceDoesNotReorderInput(t *testing.T) {
	stances := []workStance{
		ws("first", scengine.DesignCohort, 1),
		ws("second", scengine.DesignMetaAnalysis, 2),
		ws("third", scengine.DesignRCT, 3),
	}
	_ = topByStance(stances, scengine.StanceSupporting, 3)

	want := []string{"first", "second", "third"}
	for i, w := range want {
		if stances[i].Work.Title != w {
			t.Fatalf("input reordered at %d: got %q, want %q", i, stances[i].Work.Title, w)
		}
	}
	if !sameTitles(allStudyBriefs(stances), want) {
		t.Errorf("all_studies order changed: %v", titlesOf(allStudyBriefs(stances)))
	}
}

// TestTopByStanceFiltersByStance keeps the sort change from widening the list.
func TestTopByStanceFiltersByStance(t *testing.T) {
	refuting := ws("refuting-meta", scengine.DesignMetaAnalysis, 100)
	refuting.Stance = scengine.StanceRefuting
	stances := []workStance{refuting, ws("supporting-rct", scengine.DesignRCT, 5)}

	got := topByStance(stances, scengine.StanceSupporting, 3)
	if !sameTitles(got, []string{"supporting-rct"}) {
		t.Errorf("stance filter broke: %v", titlesOf(got))
	}
}
