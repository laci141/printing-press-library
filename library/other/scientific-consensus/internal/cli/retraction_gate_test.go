// Hand-authored tests for the retraction gate on the live consensus path:
// normalization of OpenAlex is_retracted, the two detection signals, and an
// end-to-end proof that a retracted work reaches neither the score nor the
// apex design. No network: a fake apiGetter serves canned OpenAlex works.
// Not generated.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// fakeRetractionClient implements apiGetter and records the select list it was
// asked for, so a test can assert that is_retracted was requested AND that the
// flag survives normalization into scWork.
type fakeRetractionClient struct {
	works      []retractionFixture
	lastSelect string
}

type retractionFixture struct {
	title       string
	isRetracted bool
	workType    string
	citedBy     int
	abstract    string // single-space-separated words, served as abstract_inverted_index
}

func (f *fakeRetractionClient) Get(_ context.Context, _ string, params map[string]string) (json.RawMessage, error) {
	f.lastSelect = params["select"]
	results := make([]map[string]any, 0, len(f.works))
	for i, w := range f.works {
		wt := w.workType
		if wt == "" {
			wt = "article"
		}
		var inverted map[string][]int
		if w.abstract != "" {
			inverted = map[string][]int{}
			for pos, word := range strings.Fields(w.abstract) {
				inverted[word] = append(inverted[word], pos)
			}
		}
		results = append(results, map[string]any{
			"abstract_inverted_index": inverted,
			"id":                      "https://openalex.org/W" + string(rune('1'+i)),
			"display_name":            w.title,
			"publication_year":        2021,
			"cited_by_count":          w.citedBy,
			"type":                    wt,
			"is_retracted":            w.isRetracted,
		})
	}
	raw, err := json.Marshal(map[string]any{
		"meta":    map[string]any{"count": len(results)},
		"results": results,
	})
	return json.RawMessage(raw), err
}

// TestFetchWorksRequestsAndCarriesIsRetracted pins the drop-at-normalization
// bug this change exists to prevent: the field must be in the OpenAlex select
// list, and the parsed value must still be on scWork after normalization.
func TestFetchWorksRequestsAndCarriesIsRetracted(t *testing.T) {
	c := &fakeRetractionClient{works: []retractionFixture{
		{title: "A trial that was later withdrawn", isRetracted: true},
		{title: "An ordinary trial", isRetracted: false},
	}}
	works, _, err := fetchWorks(context.Background(), c, "claim", "", "", 10)
	if err != nil {
		t.Fatalf("fetchWorks error: %v", err)
	}
	if !strings.Contains(c.lastSelect, "is_retracted") {
		t.Fatalf("select list does not request is_retracted: %q", c.lastSelect)
	}
	if len(works) != 2 {
		t.Fatalf("fetchWorks returned %d works, want 2", len(works))
	}
	if !works[0].IsRetracted {
		t.Errorf("works[0].IsRetracted = false; the index flag was dropped during normalization")
	}
	if works[1].IsRetracted {
		t.Errorf("works[1].IsRetracted = true, want false")
	}
}

// TestFilterRetracted covers the three signal combinations the gate has to
// distinguish, and pins that the input slice is not mutated.
func TestFilterRetracted(t *testing.T) {
	tests := []struct {
		name         string
		work         scWork
		wantExcluded bool
		wantStatus   scengine.Retraction
	}{
		{
			name:         "title marker alone is declared",
			work:         scWork{Title: "RETRACTED: Vitamin C prevents the common cold", IsRetracted: false},
			wantExcluded: true,
			wantStatus:   scengine.RetractionDeclared,
		},
		{
			name:         "index flag alone is flagged",
			work:         scWork{Title: "Vitamin C and the common cold: a meta-analysis", IsRetracted: true},
			wantExcluded: true,
			wantStatus:   scengine.RetractionFlagged,
		},
		{
			name:         "neither signal is kept",
			work:         scWork{Title: "Vitamin C and the common cold: a meta-analysis", IsRetracted: false},
			wantExcluded: false,
			wantStatus:   scengine.NotRetracted,
		},
		{
			name:         "a paper about retraction is kept (no start-anchored marker)",
			work:         scWork{Title: "Retracted Science and the Retraction Index", IsRetracted: false},
			wantExcluded: false,
			wantStatus:   scengine.NotRetracted,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := []scWork{tt.work}
			kept, excluded := filterRetracted(in)
			gotExcluded := len(excluded) == 1
			if gotExcluded != tt.wantExcluded {
				t.Fatalf("excluded = %v (kept %d, excluded %d), want %v",
					gotExcluded, len(kept), len(excluded), tt.wantExcluded)
			}
			got := append(append([]scWork{}, kept...), excluded...)
			if got[0].Retraction != tt.wantStatus {
				t.Errorf("Retraction = %q, want %q", got[0].Retraction, tt.wantStatus)
			}
			if in[0].Retraction != scengine.NotRetracted {
				t.Errorf("input slice mutated: Retraction = %q", in[0].Retraction)
			}
		})
	}
}

// TestFilterRetractedPropagatesToSameTitleYearTwin pins the measured bug: the
// same paper indexed twice (a curly-apostrophe copy with no DOI and no flag, and
// a straight-apostrophe copy that OpenAlex flags) used to be split, the
// unflagged copy being scored. Works are addressed by ID because the helper
// returns them partitioned, not in input order.
func TestFilterRetractedPropagatesToSameTitleYearTwin(t *testing.T) {
	const (
		curly    = "Vitamin D reduces falls and hip fractures in vascular Parkinsonism but not in Parkinson’s disease"
		straight = "Vitamin D reduces falls and hip fractures in vascular Parkinsonism but not in Parkinson's disease"
	)
	a := func(year int) scWork { return scWork{ID: "A", Title: curly, Year: year} }
	b := func(year int) scWork { return scWork{ID: "B", Title: straight, Year: year, IsRetracted: true} }

	tests := []struct {
		name         string
		in           []scWork
		wantKept     int
		wantExcluded int
		want         map[string]scengine.Retraction
	}{
		{
			name:         "sato pair, unflagged copy first",
			in:           []scWork{a(2013), b(2013)},
			wantKept:     0,
			wantExcluded: 2,
			want:         map[string]scengine.Retraction{"A": scengine.RetractionTwin, "B": scengine.RetractionFlagged},
		},
		{
			name:         "sato pair, reversed input order",
			in:           []scWork{b(2013), a(2013)},
			wantKept:     0,
			wantExcluded: 2,
			want:         map[string]scengine.Retraction{"A": scengine.RetractionTwin, "B": scengine.RetractionFlagged},
		},
		{
			name:         "different year is not a twin",
			in:           []scWork{a(2013), b(2014)},
			wantKept:     1,
			wantExcluded: 1,
			want:         map[string]scengine.Retraction{"A": scengine.NotRetracted, "B": scengine.RetractionFlagged},
		},
		{
			name:         "year 0 on both never gives or receives a twin mark",
			in:           []scWork{a(0), b(0)},
			wantKept:     1,
			wantExcluded: 1,
			want:         map[string]scengine.Retraction{"A": scengine.NotRetracted, "B": scengine.RetractionFlagged},
		},
		{
			name: "unrelated title in the same year is kept",
			in: []scWork{
				{ID: "C", Title: "Calcium intake and bone density in postmenopausal women", Year: 2013},
				b(2013),
			},
			wantKept:     1,
			wantExcluded: 1,
			want:         map[string]scengine.Retraction{"C": scengine.NotRetracted, "B": scengine.RetractionFlagged},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]scWork(nil), tt.in...)
			kept, excluded := filterRetracted(in)
			if len(kept) != tt.wantKept || len(excluded) != tt.wantExcluded {
				t.Fatalf("kept %d, excluded %d; want kept %d, excluded %d",
					len(kept), len(excluded), tt.wantKept, tt.wantExcluded)
			}
			got := map[string]scengine.Retraction{}
			for _, w := range append(append([]scWork{}, kept...), excluded...) {
				got[w.ID] = w.Retraction
			}
			for id, want := range tt.want {
				if got[id] != want {
					t.Errorf("work %s: Retraction = %q, want %q", id, got[id], want)
				}
			}
			for i := range in {
				if in[i].Retraction != scengine.NotRetracted {
					t.Errorf("input slice mutated: in[%d].Retraction = %q", i, in[i].Retraction)
				}
			}
		})
	}
}

// TestPropagateRetractionToTwinsKeepsExistingStatus calls the helper directly:
// a twin mark is only ever given to a NotRetracted work, and the two source
// tiers are never overwritten.
func TestPropagateRetractionToTwinsKeepsExistingStatus(t *testing.T) {
	works := []scWork{
		{ID: "decl", Title: "Same Title", Year: 2020, Retraction: scengine.RetractionDeclared},
		{ID: "flag", Title: "same  title!", Year: 2020, Retraction: scengine.RetractionFlagged},
		{ID: "none", Title: "SAME title", Year: 2020},
		{ID: "other", Title: "Same Title", Year: 2021},
		{ID: "noyear", Title: "Same Title"},
		{ID: "notitle", Title: "?!", Year: 2020},
	}
	propagateRetractionToTwins(works)
	want := map[string]scengine.Retraction{
		"decl":    scengine.RetractionDeclared,
		"flag":    scengine.RetractionFlagged,
		"none":    scengine.RetractionTwin,
		"other":   scengine.NotRetracted,
		"noyear":  scengine.NotRetracted,
		"notitle": scengine.NotRetracted,
	}
	for _, w := range works {
		if w.Retraction != want[w.ID] {
			t.Errorf("work %s: Retraction = %q, want %q", w.ID, w.Retraction, want[w.ID])
		}
	}
}

// TestConsensusExcludesRetractedFromScoreAndApex is the end-to-end proof.
// The retracted work is a meta-analysis (the apex tier) with far more
// citations than anything else and a supporting finding, so if it reached the
// scoring loop it would raise apex_design to meta-analysis, add its citations
// to total_citations, and pull the score toward +1. The control run is the
// same corpus with that one work not retracted.
func TestConsensusExcludesRetractedFromScoreAndApex(t *testing.T) {
	claim := "vitamin C prevents the common cold"
	corpus := func(retracted bool) []retractionFixture {
		return []retractionFixture{
			{title: "Vitamin C showed no significant effect on cold incidence in a cohort study",
				citedBy: 10},
			{title: "Vitamin C did not reduce cold duration in a cohort study",
				citedBy: 12},
			{title: "Vitamin C had no effect on common cold risk in a cohort study",
				citedBy: 14},
			{title: "Meta-analysis: vitamin C significantly reduced common cold incidence and improved recovery",
				citedBy: 5000, isRetracted: retracted},
		}
	}

	ctrl := &fakeRetractionClient{works: corpus(false)}
	control, err := computeConsensus(context.Background(), ctrl, claim, 10, 0, false)
	if err != nil {
		t.Fatalf("computeConsensus (control) error: %v", err)
	}
	if control.ApexDesign != scengine.DesignMetaAnalysis {
		t.Fatalf("control apex_design = %q, want %q — fixture no longer exercises the case",
			control.ApexDesign, scengine.DesignMetaAnalysis)
	}
	if control.RetractedExcluded != 0 {
		t.Fatalf("control retracted_excluded = %d, want 0", control.RetractedExcluded)
	}

	rc := &fakeRetractionClient{works: corpus(true)}
	got, err := computeConsensus(context.Background(), rc, claim, 10, 0, false)
	if err != nil {
		t.Fatalf("computeConsensus (retracted) error: %v", err)
	}

	if got.RetractedExcluded != 1 {
		t.Errorf("retracted_excluded = %d, want 1", got.RetractedExcluded)
	}
	if got.StudyCount != control.StudyCount-1 {
		t.Errorf("study_count = %d, want %d (control %d minus the retracted work)",
			got.StudyCount, control.StudyCount-1, control.StudyCount)
	}
	if got.ApexDesign == scengine.DesignMetaAnalysis {
		t.Errorf("apex_design = %q: the retracted meta-analysis still set the evidence tier",
			got.ApexDesign)
	}
	if got.TotalCitations != control.TotalCitations-5000 {
		t.Errorf("total_citations = %d, want %d: the retracted work's citation mass still counted",
			got.TotalCitations, control.TotalCitations-5000)
	}
	if got.Supporting != control.Supporting-1 {
		t.Errorf("supporting = %d, want %d: the retracted work still counted as supporting evidence",
			got.Supporting, control.Supporting-1)
	}
	if got.ConsensusScore >= control.ConsensusScore {
		t.Errorf("consensus_score = %+.2f, control %+.2f: excluding a heavily cited supporting "+
			"retraction must not leave the score at or above the control",
			got.ConsensusScore, control.ConsensusScore)
	}

	// PRISMA visibility: the excluded work must still be listed, with its
	// reason attached, rather than vanishing from the output.
	var found *workBrief
	for i := range got.AllStudies {
		if strings.HasPrefix(got.AllStudies[i].Title, "Meta-analysis: vitamin C") {
			found = &got.AllStudies[i]
		}
	}
	if found == nil {
		t.Fatalf("the retracted work disappeared from all_studies")
	}
	if found.Retraction != scengine.RetractionFlagged {
		t.Errorf("all_studies retraction = %q, want %q", found.Retraction, scengine.RetractionFlagged)
	}
	if found.RetractionNote == "" {
		t.Errorf("all_studies retraction_note is empty; the reader cannot see why it was dropped")
	}
	if found.Stance != "" {
		t.Errorf("all_studies stance = %q for an unscored work, want empty", found.Stance)
	}
}

// TestConsensusTwinSurvivesRelevanceGate pins the order of detection and the
// relevance gate. The PICO gate reads abstract + title, so two copies of one
// paper (same title, same year) split at it on the abstract alone: the flagged
// copy has none and is dropped, the unflagged copy mentions the outcome and
// survives. Detection that runs after the gate then sees no flagged match and
// scores the retracted paper.
func TestConsensusTwinSurvivesRelevanceGate(t *testing.T) {
	const title = "Coffee and cognition"
	c := &fakeRetractionClient{works: []retractionFixture{
		{title: title, isRetracted: true, citedBy: 100},
		{title: title, citedBy: 50, abstract: "Coffee intake improved alertness in a cohort study"},
	}}
	got, err := computeConsensus(context.Background(), c, "coffee improves alertness", 10, 0, false)
	if err != nil {
		t.Fatalf("computeConsensus error: %v", err)
	}
	if got.StudyCount != 0 || got.Supporting+got.Refuting+got.Mixed+got.Inconclusive != 0 {
		t.Errorf("the unflagged twin was scored: study_count=%d supporting=%d refuting=%d mixed=%d inconclusive=%d",
			got.StudyCount, got.Supporting, got.Refuting, got.Mixed, got.Inconclusive)
	}
	if got.RetractedExcluded != 1 {
		t.Errorf("retracted_excluded = %d, want 1 (only the copy that passed the gate)", got.RetractedExcluded)
	}
	if len(got.AllStudies) != 1 {
		t.Fatalf("all_studies has %d entries, want 1 (the dropped flagged copy must not appear)", len(got.AllStudies))
	}
	if r := got.AllStudies[0].Retraction; r != scengine.RetractionTwin {
		t.Errorf("all_studies[0].retraction = %q, want %q", r, scengine.RetractionTwin)
	}
	if got.AllStudies[0].CitedBy != 50 {
		t.Errorf("all_studies[0].cited_by_count = %d, want 50 (the unflagged copy)", got.AllStudies[0].CitedBy)
	}
}

// TestConsensusCommandTwinSurvivesRelevanceGate drives the real consensus
// command (consensus.go's own markRetractions call, not computeConsensus)
// against an in-process OpenAlex stand-in. The command builds its client from
// config, so the base URL is redirected by env; nothing leaves the process and
// everything the client or config could write is pointed into t.TempDir().
func TestConsensusCommandTwinSurvivesRelevanceGate(t *testing.T) {
	const title = "Coffee and cognition"
	fake := &fakeRetractionClient{works: []retractionFixture{
		{title: title, isRetracted: true, citedBy: 100},
		{title: title, citedBy: 50, abstract: "Coffee intake improved alertness in a cohort study"},
	}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/works" {
			http.NotFound(w, r)
			return
		}
		raw, err := fake.Get(r.Context(), r.URL.Path, map[string]string{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw)
	}))
	t.Cleanup(srv.Close)

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("SCIENTIFIC_CONSENSUS_CONFIG", filepath.Join(home, "config.toml"))
	t.Setenv("SCIENTIFIC_CONSENSUS_BASE_URL", srv.URL)
	for _, k := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "DEEPSEEK_API_KEY", "GEMINI_API_KEY", "GROQ_API_KEY", "MISTRAL_API_KEY"} {
		t.Setenv(k, "")
	}

	var flags rootFlags
	root := newRootCmd(&flags)
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"consensus", "coffee improves alertness", "--json", "--no-cache", "--enrich=false"})
	if err := root.Execute(); err != nil {
		t.Fatalf("consensus command error: %v", err)
	}

	var got consensusOutput
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("decoding command output: %v\n%s", err, stdout.String())
	}
	if got.StudyCount != 0 || got.Supporting+got.Refuting+got.Mixed+got.Inconclusive != 0 {
		t.Errorf("the unflagged twin was scored: study_count=%d supporting=%d refuting=%d mixed=%d inconclusive=%d",
			got.StudyCount, got.Supporting, got.Refuting, got.Mixed, got.Inconclusive)
	}
	if got.RetractedExcluded != 1 {
		t.Errorf("retracted_excluded = %d, want 1", got.RetractedExcluded)
	}
	if len(got.AllStudies) != 1 {
		t.Fatalf("all_studies has %d entries, want 1 (the flagged copy must be absent)", len(got.AllStudies))
	}
	if r := got.AllStudies[0].Retraction; r != scengine.RetractionTwin {
		t.Errorf("all_studies[0].retraction = %q, want %q", r, scengine.RetractionTwin)
	}
	if got.AllStudies[0].CitedBy != 50 {
		t.Errorf("all_studies[0].cited_by_count = %d, want 50 (the unflagged copy)", got.AllStudies[0].CitedBy)
	}
}
