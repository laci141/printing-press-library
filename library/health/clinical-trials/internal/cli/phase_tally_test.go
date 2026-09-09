// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.

// Guards the phase tally against dropping trials that carry no phase.
package cli

import "testing"

// phaseTallyCorpus is a ten-trial sample shaped like a real registry page:
// six interventional trials with a phase, two interventional with phase NA,
// and two observational studies whose Phases array is absent entirely.
func phaseTallyCorpus() []Trial {
	return []Trial{
		{NCTID: "NCT00000001", Phases: []string{"PHASE1"}},
		{NCTID: "NCT00000002", Phases: []string{"PHASE2"}},
		{NCTID: "NCT00000003", Phases: []string{"PHASE3"}},
		{NCTID: "NCT00000004", Phases: []string{"PHASE3"}},
		{NCTID: "NCT00000005", Phases: []string{"PHASE4"}},
		{NCTID: "NCT00000006", Phases: []string{"EARLY_PHASE1"}},
		{NCTID: "NCT00000007", Phases: []string{"NA"}},
		{NCTID: "NCT00000008", Phases: []string{"NA"}},
		{NCTID: "NCT00000009"}, // observational: no Phases key
		{NCTID: "NCT00000010"}, // observational: no Phases key
	}
}

func tallyTotal(entries []rankedEntry) int {
	total := 0
	for _, e := range entries {
		total += e.Count
	}
	return total
}

// The tally must account for every trial in the corpus. Before the fix the
// range loop skipped phaseless trials entirely, so a distribution printed
// beside "sample size 10" summed to 8 and the two missing trials appeared
// nowhere — a reader could not tell they existed.
func TestPhaseTallyCoversEveryTrial(t *testing.T) {
	corpus := phaseTallyCorpus()
	view := buildTrialListView("q", "RECRUITING", len(corpus), corpus)
	if got := tallyTotal(view.PhaseDistribution); got != len(corpus) {
		t.Errorf("phase distribution sums to %d, want %d (the whole corpus)", got, len(corpus))
	}
	if view.Returned != len(corpus) {
		t.Errorf("returned = %d, want %d", view.Returned, len(corpus))
	}
}

// Phaseless trials land in N/A alongside the explicit "NA" ones: two of each
// in this corpus, so the bucket holds four. That is the same answer
// phaseDisplay gives a phaseless trial for the Phase column, which is why the
// summary and the printed rows agree.
func TestPhaselessTrialsCountAsNA(t *testing.T) {
	corpus := phaseTallyCorpus()
	view := buildTrialListView("q", "RECRUITING", len(corpus), corpus)
	found := false
	for _, e := range view.PhaseDistribution {
		if e.Label != "N/A" {
			continue
		}
		found = true
		if e.Count != 4 {
			t.Errorf("N/A bucket = %d, want 4 (2 explicit NA + 2 phaseless)", e.Count)
		}
	}
	if !found {
		t.Error("no N/A bucket in the distribution")
	}
	if got := phaseDisplay(nil); got != "N/A" {
		t.Errorf("phaseDisplay(nil) = %q, want %q — the tally must agree with the column", got, "N/A")
	}
}

// A corpus of nothing but observational studies must still produce a
// distribution. Before the fix this returned zero buckets, so the caller's
// `len(PhaseDistribution) == 0` branch reported "no distribution computed"
// for a sample that was in fact fully described by one.
func TestAllPhaselessCorpusStillTallies(t *testing.T) {
	corpus := []Trial{{NCTID: "NCT1"}, {NCTID: "NCT2"}, {NCTID: "NCT3"}}
	view := buildTrialListView("q", "RECRUITING", len(corpus), corpus)
	if len(view.PhaseDistribution) == 0 {
		t.Fatal("no phase distribution for an all-observational corpus")
	}
	if got := tallyTotal(view.PhaseDistribution); got != len(corpus) {
		t.Errorf("phase distribution sums to %d, want %d", got, len(corpus))
	}
}
