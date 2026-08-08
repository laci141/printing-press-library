package cli

import (
	"context"
	"errors"
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
//
// It also silences the Europe PMC fallback, because a test that leaves a DOI
// unresolved on purpose would otherwise reach the live network to find that out.
// Tests that care about the fallback install their own stub over this one.
func stubBackfill(t *testing.T, resp map[string]string) (*[]string, *int) {
	t.Helper()
	stubEuropePMCBackfill(t, nil, nil)
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

// stubEuropePMCBackfill is stubBackfill's twin for the fallback source, with one
// addition: the error return is settable, because "Europe PMC answered nothing"
// and "Europe PMC failed" must leave the caller in the same state and only a
// stub that can do both proves it.
func stubEuropePMCBackfill(t *testing.T, resp map[string]string, err error) (*[]string, *int) {
	t.Helper()
	var seen []string
	calls := 0
	orig := europePMCBackfiller
	europePMCBackfiller = func(_ context.Context, dois []string) (map[string]string, error) {
		calls++
		seen = append(seen, dois...)
		return resp, err
	}
	t.Cleanup(func() { europePMCBackfiller = orig })
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
	_, epmcCalls := stubEuropePMCBackfill(t, map[string]string{"10.1/a": "so would this"}, nil)
	works := []scWork{{Title: "Meditation and anxiety", DOI: "10.1/a"}}

	rep := backfillAbstracts(t.Context(), backfillClaim, works)
	if *calls != 0 {
		t.Errorf("%s must prevent every lookup, got %d", noAbstractBackfillEnv, *calls)
	}
	if *epmcCalls != 0 {
		t.Errorf("%s is one switch for both sources; Europe PMC was asked %d time(s)", noAbstractBackfillEnv, *epmcCalls)
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

// The fallback is a fallback. When PubMed answers every DOI there is nothing
// left to ask about, and asking anyway would spend a request and a second source's
// goodwill on a question already answered.
func TestBackfillAbstractsEuropePMCUnusedWhenPubMedResolvesEverything(t *testing.T) {
	stubBackfill(t, map[string]string{
		"10.5/a": "meditation and anxiety, resolved by pubmed",
		"10.5/b": "more meditation and anxiety text",
	})
	_, epmcCalls := stubEuropePMCBackfill(t, map[string]string{"10.5/a": "must never be used"}, nil)
	works := []scWork{
		{Title: "Ceramic glaze A", DOI: "10.5/a"},
		{Title: "Ceramic glaze B", DOI: "10.5/b"},
	}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if *epmcCalls != 0 {
		t.Errorf("Europe PMC asked %d time(s) with nothing unresolved", *epmcCalls)
	}
	if rep.EuropePMCRequested != 0 || rep.EuropePMCFilled != 0 {
		t.Errorf("report %+v; the fallback must be absent from the ledger when it did not run", rep)
	}
	if rep.PubMedResolved != 2 || rep.PubMedFilled != 2 || rep.Resolved != 2 || rep.Filled != 2 {
		t.Errorf("report %+v; want PubMedResolved=2 PubMedFilled=2 Resolved=2 Filled=2", rep)
	}
	if note := backfillNote(rep); strings.Contains(note, "Europe PMC") {
		t.Errorf("note names a source that never ran: %q", note)
	}
}

// The case the second source exists for: PubMed does not index the work, Europe
// PMC does. Only the DOIs PubMed left blank may be forwarded, and they must
// arrive in the gate-excluded-first order the cap was already spent on.
func TestBackfillAbstractsFallsBackToEuropePMC(t *testing.T) {
	const survivorDOI = "10.6/survivor"
	works := []scWork{
		{Title: "Ceramic glaze firing schedule", DOI: "10.6/excluded"},
		{Title: "Meditation reduces anxiety in adults", DOI: survivorDOI},
		{Title: "Ceramic kiln temperatures", DOI: "10.6/pubmed"},
	}
	if !relevantToClaim(backfillClaim, works[1]) {
		t.Fatalf("test setup broken: %q must pass the relevance gate", works[1].Title)
	}
	stubBackfill(t, map[string]string{"10.6/pubmed": "pubmed text about meditation"})
	seen, epmcCalls := stubEuropePMCBackfill(t, map[string]string{
		"10.6/excluded": "europe pmc text about meditation and anxiety",
	}, nil)

	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if *epmcCalls != 1 {
		t.Fatalf("want exactly one batched fallback lookup, got %d", *epmcCalls)
	}
	if got := *seen; len(got) != 2 || got[0] != "10.6/excluded" || got[1] != survivorDOI {
		t.Errorf("fallback saw %v; want only the DOIs PubMed left blank, gate-excluded first", got)
	}
	if works[0].Abstract == "" {
		t.Error("Europe PMC's answer was not written back to the work")
	}
	if works[2].Abstract != "pubmed text about meditation" {
		t.Errorf("PubMed's answer was disturbed by the fallback: %q", works[2].Abstract)
	}
	if works[1].Abstract != "" {
		t.Errorf("a DOI neither source answered must stay empty, got %q", works[1].Abstract)
	}
	if rep.PubMedResolved != 1 || rep.EuropePMCRequested != 2 || rep.EuropePMCResolved != 1 ||
		rep.EuropePMCFilled != 1 || rep.Resolved != 2 || rep.Filled != 2 {
		t.Errorf("report %+v; want PubMedResolved=1 EuropePMCRequested=2 EuropePMCResolved=1 "+
			"EuropePMCFilled=1 Resolved=2 Filled=2", rep)
	}
	note := backfillNote(rep)
	for _, want := range []string{
		"2 missing abstract(s) backfilled from PubMed and Europe PMC",
		"1 of 3 looked-up DOI(s) unresolved",
		"PubMed resolved 1 of 3 DOI(s) and filled 1 work(s)",
		"Europe PMC was asked for the remaining 2, resolved 1 and filled 1",
		"leaving 1 DOI(s) neither source could reach",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("note missing %q\n got: %s", want, note)
		}
	}
}

// Both sources down is the degradation case. The command must still return, the
// works it already had must be untouched, and the ledger must say plainly that
// nothing was recovered rather than staying silent about the attempt.
func TestBackfillAbstractsSurvivesBothSourcesFailing(t *testing.T) {
	orig := abstractBackfiller
	abstractBackfiller = func(_ context.Context, _ []string) (map[string]string, error) {
		return nil, errors.New("pubmed unreachable")
	}
	t.Cleanup(func() { abstractBackfiller = orig })
	_, epmcCalls := stubEuropePMCBackfill(t, nil, errors.New("europe pmc unreachable"))

	works := []scWork{
		{Title: "Ceramic glaze", DOI: "10.7/down"},
		{Title: "Meditation reduces anxiety", DOI: "10.7/kept", Abstract: "an abstract we already had"},
	}
	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if *epmcCalls != 1 {
		t.Errorf("a PubMed failure must still hand its unresolved DOIs on, got %d call(s)", *epmcCalls)
	}
	if works[0].Abstract != "" || works[1].Abstract != "an abstract we already had" {
		t.Errorf("a total outage must leave the corpus exactly as it was: %+v", works)
	}
	if rep.Resolved != 0 || rep.Filled != 0 {
		t.Errorf("report %+v; nothing was recovered", rep)
	}
	if rep.Requested != 1 || rep.EuropePMCRequested != 1 {
		t.Errorf("report %+v; both attempts must stay on the ledger", rep)
	}
	if rep.Disabled {
		t.Error("a failed step is not a disabled one; Disabled must keep meaning 'switched off'")
	}
}

// The cap is spent once, before either source runs. The fallback inherits the
// capped set rather than reopening the budget — otherwise the second source
// would quietly undo the time bound the first one is capped for.
func TestBackfillAbstractsCapBindsAcrossBothSources(t *testing.T) {
	var works []scWork
	for i := 0; i < abstractBackfillCap+5; i++ {
		works = append(works, scWork{
			Title: "Ceramic glaze firing schedule " + strconv.Itoa(i),
			DOI:   fmt.Sprintf("10.8/x-%d", i),
		})
	}
	stubBackfill(t, map[string]string{"10.8/x-0": "meditation and anxiety text"})
	seen, epmcCalls := stubEuropePMCBackfill(t, nil, nil)

	rep := backfillAbstracts(t.Context(), backfillClaim, works)

	if rep.Skipped != 5 {
		t.Errorf("Skipped = %d, want 5 over the cap of %d", rep.Skipped, abstractBackfillCap)
	}
	if *epmcCalls != 1 {
		t.Fatalf("want one fallback lookup, got %d", *epmcCalls)
	}
	if len(*seen) != abstractBackfillCap-1 {
		t.Errorf("fallback saw %d DOI(s); want the capped %d minus the 1 PubMed resolved",
			len(*seen), abstractBackfillCap)
	}
	for _, d := range *seen {
		if d == "10.8/x-0" {
			t.Error("a DOI PubMed already resolved was re-asked of the fallback")
		}
	}
	if rep.EuropePMCRequested != abstractBackfillCap-1 {
		t.Errorf("EuropePMCRequested = %d, want %d", rep.EuropePMCRequested, abstractBackfillCap-1)
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
