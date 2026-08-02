package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// backfillClaim is used throughout: its stems ("medit", "reduc", "anxie") make
// it easy to build a work that clearly passes both gates and one that clearly
// fails the first, which is what the priority test needs.
const backfillClaim = "meditation reduces anxiety"

// stubBackfill replaces the PubMed lookup for one test and records what it was
// asked for. Returns a pointer to the recorded DOIs and the call count, so a
// test can assert that no request happened at all — the difference between
// "found nothing" and "never asked" is the whole point of the skip paths.
func stubBackfill(t *testing.T, resp map[string]string) (*[]string, *int) {
	t.Helper()
	var seen []string
	calls := 0
	orig := abstractBackfiller
	abstractBackfiller = func(_ context.Context, dois []string) (map[string]string, error) {
		calls++
		seen = append(seen, dois...)
		return resp, nil
	}
	t.Cleanup(func() { abstractBackfiller = orig })
	return &seen, &calls
}

func TestBackfillAbstractsNoCandidatesMakesNoRequest(t *testing.T) {
	_, calls := stubBackfill(t, nil)
	works := []scWork{
		{Title: "Meditation and anxiety", DOI: "10.1/a", Abstract: "already present"},
		{Title: "No DOI and no abstract here"},
	}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)
	if *calls != 0 {
		t.Errorf("no work carries both a missing abstract and a DOI; want 0 lookups, got %d", *calls)
	}
	if rep.Candidates != 0 || rep.Filled != 0 {
		t.Errorf("unexpected report %+v", rep)
	}
	if rep.NoDOI != 1 {
		t.Errorf("abstract-less work without a DOI must be counted as unreachable, got NoDOI=%d", rep.NoDOI)
	}
	if works[0].Abstract != "already present" {
		t.Errorf("a work that already has an abstract must never be overwritten, got %q", works[0].Abstract)
	}
}

func TestBackfillAbstractsDisabledByEnv(t *testing.T) {
	t.Setenv(noAbstractBackfillEnv, "1")
	_, calls := stubBackfill(t, map[string]string{"10.1/a": "would have been filled"})
	works := []scWork{{Title: "Meditation and anxiety", DOI: "10.1/a"}}

	rep := backfillAbstracts(t.Context(), backfillClaim, works)
	if *calls != 0 {
		t.Errorf("%s must prevent every lookup, got %d", noAbstractBackfillEnv, *calls)
	}
	if !rep.Disabled {
		t.Error("report must record that the step was switched off, not that it found nothing")
	}
	if works[0].Abstract != "" {
		t.Errorf("abstract written despite the kill switch: %q", works[0].Abstract)
	}
	if backfillNote(rep) != "" {
		t.Errorf("a disabled step must not emit a PRISMA fragment, got %q", backfillNote(rep))
	}
}

func TestBackfillAbstractsFillsAndNormalizesDOIs(t *testing.T) {
	// The lookup is keyed on the NORMALIZED DOI, so a work carrying a URL-form
	// or upper-case DOI must still be matched to the answer.
	seen, calls := stubBackfill(t, map[string]string{
		"10.1/upper": "RESULTS: recovered text about meditation and anxiety.",
	})
	works := []scWork{
		{Title: "Meditation trial", DOI: "https://doi.org/10.1/UPPER"},
		{Title: "Meditation and anxiety", DOI: "10.1/missing"},
	}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if *calls != 1 {
		t.Fatalf("want exactly one batched lookup, got %d", *calls)
	}
	for _, d := range *seen {
		if d != strings.ToLower(d) || strings.Contains(d, "doi.org") {
			t.Errorf("DOI sent unnormalized: %q", d)
		}
	}
	if works[0].Abstract == "" {
		t.Error("resolved abstract was not written back to the work")
	}
	if works[1].Abstract != "" {
		t.Errorf("unresolved DOI must leave the work untouched, got %q", works[1].Abstract)
	}
	if rep.Candidates != 2 || rep.Requested != 2 || rep.Resolved != 1 || rep.Filled != 1 {
		t.Errorf("report %+v; want Candidates=2 Requested=2 Resolved=1 Filled=1", rep)
	}
	if note := backfillNote(rep); !strings.Contains(note, "1 missing abstract(s) backfilled") {
		t.Errorf("note does not report the fill: %q", note)
	}
}

// TestBackfillAbstractsPrioritizesGateExcludedWorks pins the rule that makes the
// cap worth having: when there are more candidates than budget, the budget goes
// to works the gates currently EXCLUDE, because those are the only ones a
// backfill can recover. A work the gates already keep can only be pushed out by
// one (an abstract-less work clears 1 stem, an abstract-bearing work 2).
//
// The survivor is placed FIRST in fetch order, so a implementation that simply
// took the first abstractBackfillCap candidates would spend budget on it and
// fail here.
func TestBackfillAbstractsPrioritizesGateExcludedWorks(t *testing.T) {
	const survivorDOI = "10.1/survivor"
	works := []scWork{{Title: "Meditation reduces anxiety in adults", DOI: survivorDOI}}
	if !relevantToClaim(backfillClaim, works[0]) {
		t.Fatalf("test setup broken: %q must pass the relevance gate", works[0].Title)
	}
	for i := 0; i < abstractBackfillCap; i++ {
		works = append(works, scWork{
			Title: "Ceramic glaze firing schedule " + strconv.Itoa(i),
			DOI:   fmt.Sprintf("10.2/excluded-%d", i),
		})
	}
	if relevantToClaim(backfillClaim, works[1]) {
		t.Fatalf("test setup broken: %q must fail the relevance gate", works[1].Title)
	}

	seen, _ := stubBackfill(t, nil)
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if rep.Candidates != abstractBackfillCap+1 {
		t.Fatalf("Candidates = %d, want %d", rep.Candidates, abstractBackfillCap+1)
	}
	if rep.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (one candidate over the cap of %d)", rep.Skipped, abstractBackfillCap)
	}
	if len(*seen) != abstractBackfillCap {
		t.Fatalf("looked up %d DOI(s), want exactly the cap of %d", len(*seen), abstractBackfillCap)
	}
	for _, d := range *seen {
		if d == survivorDOI {
			t.Fatal("budget spent on a work the gates already keep; the gate-excluded works are the recoverable ones")
		}
	}
}

func TestBackfillAbstractsUnderCapLooksUpEverything(t *testing.T) {
	var works []scWork
	for i := 0; i < abstractBackfillCap; i++ {
		works = append(works, scWork{
			Title: "Ceramic glaze firing schedule " + strconv.Itoa(i),
			DOI:   fmt.Sprintf("10.2/x-%d", i),
		})
	}
	seen, _ := stubBackfill(t, nil)
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if rep.Skipped != 0 {
		t.Errorf("Skipped = %d at exactly the cap; nothing should be dropped", rep.Skipped)
	}
	if len(*seen) != abstractBackfillCap {
		t.Errorf("looked up %d, want all %d", len(*seen), abstractBackfillCap)
	}
	if note := backfillNote(rep); note != "" {
		t.Errorf("nothing filled and nothing skipped must produce no fragment, got %q", note)
	}
}

func TestBackfillNoteReportsCapTruncation(t *testing.T) {
	note := backfillNote(backfillReport{Candidates: 60, Requested: 50, Resolved: 40, Filled: 40, Skipped: 10})
	for _, want := range []string{
		"40 missing abstract(s) backfilled",
		"10 of 50 looked-up DOI(s) unresolved",
		"10 abstract-less work(s) were NOT looked up",
		"cap of " + strconv.Itoa(abstractBackfillCap),
		"gated on their title alone",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("note missing %q\n got: %s", want, note)
		}
	}
}

// One DOI can back several records in an OpenAlex result set. Filled counts
// WORKS and Resolved counts DOIS, so Filled may exceed Resolved — the note must
// not then report a negative number of unresolved lookups.
func TestBackfillAbstractsDuplicateDOIFillsEveryWork(t *testing.T) {
	stubBackfill(t, map[string]string{"10.3/dup": "meditation and anxiety abstract"})
	works := []scWork{
		{Title: "Ceramic glaze A", DOI: "10.3/dup"},
		{Title: "Ceramic glaze B", DOI: "https://doi.org/10.3/DUP"},
	}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if rep.Requested != 1 || rep.Resolved != 1 || rep.Filled != 2 {
		t.Errorf("report %+v; want Requested=1 Resolved=1 Filled=2", rep)
	}
	if works[0].Abstract == "" || works[1].Abstract == "" {
		t.Error("both works sharing the DOI must receive the abstract")
	}
	if note := backfillNote(rep); !strings.Contains(note, "0 of 1 looked-up DOI(s) unresolved") {
		t.Errorf("unresolved count must not go negative when Filled > Resolved: %q", note)
	}
}

func TestBackfillAbstractsIgnoresBlankAnswers(t *testing.T) {
	stubBackfill(t, map[string]string{"10.4/blank": "   \n  "})
	works := []scWork{{Title: "Ceramic glaze", DOI: "10.4/blank"}}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if works[0].Abstract != "" {
		t.Errorf("a whitespace-only answer must not be written: %q", works[0].Abstract)
	}
	if rep.Resolved != 0 || rep.Filled != 0 {
		t.Errorf("report %+v; a blank answer counts as unresolved", rep)
	}
}

// The ledger must carry the backfill into the PRISMA note that the consensus
// command actually prints, and must not disturb the exclusion arithmetic —
// backfilling changes what the gates READ, never how many works they saw.
func TestGateLedgerCarriesBackfillWithoutBreakingArithmetic(t *testing.T) {
	g := gateLedger{
		backfill:      backfillReport{Candidates: 5, Requested: 5, Resolved: 4, Filled: 4, Skipped: 2},
		relevance:     relevanceReport{Fetched: 10, Kept: 8, Excluded: 2, Stems: []string{"medit"}, MinTokens: 2, MinTokensNoAbstract: 1},
		picoExcluded:  1,
		relevantCount: 7,
		studyCount:    7,
	}
	if !g.consistent() {
		t.Fatal("backfill must not participate in the exclusion arithmetic")
	}
	notes := gateNotes(g)
	if len(notes) == 0 || !strings.Contains(notes[0], "backfilled from PubMed") {
		t.Fatalf("backfill fragment must lead the ledger (it ran first), got %v", notes)
	}
	joined := appendGateNotes("", g)
	if !strings.Contains(joined, "were NOT looked up") {
		t.Errorf("capped candidates must reach the printed note: %s", joined)
	}
}
