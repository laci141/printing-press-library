package source

import "testing"

func TestNormDOI(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://doi.org/10.1/ABC", "10.1/abc"},
		{"doi:10.2/XyZ", "10.2/xyz"},
		{"  10.3/Foo  ", "10.3/foo"},
		{"http://doi.org/10.4/bar", "10.4/bar"},
	}
	for _, tt := range tests {
		if got := NormDOI(tt.in); got != tt.want {
			t.Errorf("NormDOI(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestPubmedSummaryYearDOI(t *testing.T) {
	s := pubmedSummary{
		PubDate: "2026 Jun 19",
		ArticleIDs: []struct {
			IDType string `json:"idtype"`
			Value  string `json:"value"`
		}{
			{IDType: "pubmed", Value: "123"},
			{IDType: "doi", Value: "10.5/test"},
		},
	}
	if s.Year() != 2026 {
		t.Errorf("Year() = %d, want 2026", s.Year())
	}
	if s.DOI() != "10.5/test" {
		t.Errorf("DOI() = %q, want 10.5/test", s.DOI())
	}
	empty := pubmedSummary{PubDate: ""}
	if empty.Year() != 0 {
		t.Errorf("empty Year() = %d, want 0", empty.Year())
	}
}

func TestCrossrefItemAccessors(t *testing.T) {
	it := crossrefItem{Title: []string{"A Study"}}
	it.Published.DateParts = [][]int{{2024, 3, 1}}
	if it.year() != 2024 {
		t.Errorf("year() = %d, want 2024", it.year())
	}
	if it.title() != "A Study" {
		t.Errorf("title() = %q, want 'A Study'", it.title())
	}
	if (crossrefItem{}).title() != "" {
		t.Errorf("empty title() should be empty")
	}
}

func TestRegistryHasAllSources(t *testing.T) {
	want := map[string]bool{"pubmed": true, "crossref": true, "europepmc": true, "semanticscholar": true}
	got := map[string]bool{}
	for _, s := range All() {
		got[s.Name()] = true
		if s.AuthRequired() {
			t.Errorf("source %q should be keyless (AuthRequired=false)", s.Name())
		}
	}
	for name := range want {
		if !got[name] {
			t.Errorf("source %q not registered", name)
		}
	}
	if _, ok := Lookup("pubmed"); !ok {
		t.Errorf("Lookup(pubmed) failed")
	}
}
