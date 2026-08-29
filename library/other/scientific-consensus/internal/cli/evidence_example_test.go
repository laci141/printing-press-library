// Hand-authored test for the evidence pyramid's exemplar. The pyramid names one
// work per design level and reports its DOI and year alongside; this pins that
// the three come from the same work. A client renders a citation from all three
// at once, so a title paired with another work's DOI resolves to the wrong
// paper while looking complete. Calls classifyForPyramid directly rather than
// reproducing its loop, so a change to the real code is what the assertions
// see. Not generated.
package cli

import (
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// TestClassifyForPyramidExemplarIsOneWork is the assertion the change exists
// for. The two fixtures classify to different design levels and carry DOIs and
// years that are individually plausible against either title, so a swap is
// caught by the pairing rather than by one value looking obviously wrong.
func TestClassifyForPyramidExemplarIsOneWork(t *testing.T) {
	works := []scWork{
		{
			Title:    "A meta-analysis of melatonin for shift work sleep disorder",
			DOI:      "10.1000/meta.2019",
			Year:     2019,
			Type:     "review",
			PubTypes: []string{"Meta-Analysis"},
		},
		{
			Title:    "Randomised controlled trial of melatonin timing in rotating shift workers",
			DOI:      "10.1000/rct.2011",
			Year:     2011,
			Type:     "article",
			PubTypes: []string{"Randomized Controlled Trial"},
		},
	}

	_, _, exemplar := classifyForPyramid(works, nil)

	// Index the fixtures by title so each assertion asks the question a citation
	// asks: given this name, is the rest of the row this work's?
	want := map[string]scWork{}
	for _, w := range works {
		want[w.Title] = w
	}

	if len(exemplar) == 0 {
		t.Fatal("no exemplar recorded for any design")
	}
	for design, ex := range exemplar {
		w, ok := want[ex.title]
		if !ok {
			t.Errorf("%s: exemplar title %q is not one of the fixtures", design, ex.title)
			continue
		}
		if ex.doi != w.DOI {
			t.Errorf("%s: exemplar %q has doi %q, want %q — the DOI came from a different work",
				design, ex.title, ex.doi, w.DOI)
		}
		if ex.year != w.Year {
			t.Errorf("%s: exemplar %q has year %d, want %d — the year came from a different work",
				design, ex.title, ex.year, w.Year)
		}
	}
}

// TestClassifyForPyramidKeepsFirstAtEachDesign pins the selection rule itself.
// Two works of the same design must not overwrite each other, or the reported
// example would depend on result ordering the caller cannot see.
func TestClassifyForPyramidKeepsFirstAtEachDesign(t *testing.T) {
	works := []scWork{
		{Title: "First RCT", DOI: "10.1000/first", Year: 2001, Type: "article", PubTypes: []string{"Randomized Controlled Trial"}},
		{Title: "Second RCT", DOI: "10.1000/second", Year: 2002, Type: "article", PubTypes: []string{"Randomized Controlled Trial"}},
	}

	_, _, exemplar := classifyForPyramid(works, nil)

	ex, ok := exemplar[scengine.DesignRCT]
	if !ok {
		t.Fatalf("no exemplar for the RCT level; got %v", exemplar)
	}
	if ex.title != "First RCT" {
		t.Errorf("exemplar title = %q, want the first work seen at this design", ex.title)
	}
	if ex.doi != "10.1000/first" {
		t.Errorf("exemplar doi = %q, want the first work's", ex.doi)
	}
}

// TestClassifyForPyramidCountsPubTypeClassifications pins the second return
// value, which the UI shows as how much of the pyramid rests on publisher
// metadata rather than a title guess.
func TestClassifyForPyramidCountsPubTypeClassifications(t *testing.T) {
	works := []scWork{
		{Title: "Typed by the publisher", Type: "article", PubTypes: []string{"Randomized Controlled Trial"}},
		{Title: "Nothing here identifies a design", Type: "article"},
	}

	_, byPub, _ := classifyForPyramid(works, nil)

	if byPub != 1 {
		t.Errorf("byPub = %d, want 1 — only the first fixture carries a publication type", byPub)
	}
}
