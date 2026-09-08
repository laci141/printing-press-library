// Hand-authored edge-case tests for the controversies command. Not generated.
package cli

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// stanceFixture builds a corpus with the requested number of works per stance.
// Stances are set literally rather than classified from text, so these tests
// exercise the tally and not the classifier's heuristics.
func stanceFixture(supporting, refuting, mixed, inconclusive int) []workStance {
	stances := []workStance{}
	add := func(st scengine.Stance, n int) {
		for i := 0; i < n; i++ {
			stances = append(stances, workStance{
				Work:   scWork{ID: string(st) + string(rune('1'+i)), Title: string(st) + " study", Year: 2021, CitedBy: 10 + i},
				Stance: st, Confidence: 0.6,
			})
		}
	}
	add(scengine.StanceSupporting, supporting)
	add(scengine.StanceRefuting, refuting)
	add(scengine.StanceMixed, mixed)
	add(scengine.StanceInconclusive, inconclusive)
	return stances
}

// Contested requires BOTH a score at or above 0.5 AND at least four
// directional works. A corpus split 1/1 is perfectly balanced and scores 1.0,
// but two directional works are too thin to call a topic contested.
// Mutation-verified: dropping the directional>=4 half of the condition fails
// this test.
func TestControversiesContestedNeedsFourDirectional(t *testing.T) {
	out := tallyControversy("thin corpus", 2, stanceFixture(1, 1, 0, 0))
	if out.ControversyScore != 1.0 {
		t.Fatalf("controversy_score = %v, want 1.0 (a 1/1 split is even)", out.ControversyScore)
	}
	if out.Contested {
		t.Error("contested = true on 2 directional works, want false")
	}
}

// The two Note branches are ordered, and an empty corpus must report that
// nothing was found rather than that the evidence is thin: with studyCount 0
// both conditions hold, so only the order decides which message ships.
// Mutation-verified: swapping the two branches fails the empty_corpus subtest.
func TestControversiesNoteBranches(t *testing.T) {
	cases := []struct {
		name       string
		studyCount int
		stances    []workStance
		wantNote   string
	}{
		{"empty corpus", 0, nil, "no works found; try a broader query"},
		{"thin directional", 3, stanceFixture(2, 1, 0, 0), "too few directional studies to assess controversy reliably"},
		{"only inconclusive", 4, stanceFixture(0, 0, 0, 4), "too few directional studies to assess controversy reliably"},
		{"enough directional", 8, stanceFixture(5, 3, 0, 0), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := tallyControversy("q", tc.studyCount, tc.stances)
			if out.Note != tc.wantNote {
				t.Errorf("note = %q, want %q", out.Note, tc.wantNote)
			}
		})
	}
}

// A corpus with no directional works must not reach the division. Removing the
// directional>0 guard does NOT surface as a NaN: round2f converts through int,
// and converting NaN to int is undefined in Go, so the measured result was
// -9.223372036854776e+16 — a finite number that marshals to valid JSON. That
// is worse than a NaN, which encoding/json would reject outright, so this test
// asserts the exact value rather than only checking IsNaN.
// Mutation-verified: removing the guard fails on the value assertion.
func TestControversiesNoDirectionalWorksScoresZero(t *testing.T) {
	out := tallyControversy("q", 4, stanceFixture(0, 0, 0, 4))
	if out.ControversyScore != 0 {
		t.Errorf("controversy_score = %v, want 0 with no directional works", out.ControversyScore)
	}
	if math.IsNaN(out.ControversyScore) || math.IsInf(out.ControversyScore, 0) {
		t.Errorf("controversy_score = %v, want a finite number", out.ControversyScore)
	}
	if _, err := json.Marshal(out); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

// round2f rounds to the nearest hundredth rather than truncating. A 5/4 split
// gives 2*4/9 = 0.888..., which rounds to 0.89 and truncates to 0.88.
// Mutation-verified: dropping the +0.5 fails this test at 0.88.
func TestControversiesScoreRoundsNotTruncates(t *testing.T) {
	out := tallyControversy("q", 9, stanceFixture(5, 4, 0, 0))
	if out.ControversyScore != 0.89 {
		t.Errorf("controversy_score = %v, want 0.89 (2*4/9 = 0.888... rounded)", out.ControversyScore)
	}
}

// With no supporting or refuting works the side lists must be absent from the
// rendered text rather than printed as empty headings.
// Mutation-verified: removing either len()>0 guard fails this test.
func TestControversiesRenderOmitsEmptySides(t *testing.T) {
	var sb strings.Builder
	renderControversy(&sb, tallyControversy("q", 4, stanceFixture(0, 0, 0, 4)))
	got := sb.String()
	if strings.Contains(got, "Supporting side:") {
		t.Error("rendered a Supporting side heading with no supporting works")
	}
	if strings.Contains(got, "Refuting side:") {
		t.Error("rendered a Refuting side heading with no refuting works")
	}
}
