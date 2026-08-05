package cli

import "testing"

func TestBuildNarrowedSearch(t *testing.T) {
	cases := []struct {
		name  string
		claim string
		want  string
	}{
		{
			name:  "measured live query 2026-08-05",
			claim: "vitamin D reduces respiratory infections",
			want:  "vitamin d respiratory infections",
		},
		{
			name:  "hyphenated intervention keeps its number",
			claim: "omega-3 improves cardiovascular health",
			want:  "omega-3 cardiovascular health",
		},
		{
			name:  "harm claim drops its verb too",
			claim: "HPV vaccine causes infertility",
			want:  "hpv vaccine infertility",
		},
		{
			name:  "comma cannot open a second filter clause",
			claim: "coffee reduces heart disease, stroke",
			want:  "coffee heart disease stroke",
		},
		{
			name:  "pipe cannot open an OR clause",
			claim: "exercise improves mood|sleep",
			want:  "exercise mood sleep",
		},
		{
			name:  "colon cannot start a filter value",
			claim: "melatonin improves sleep: a review",
			want:  "melatonin sleep review",
		},
		{
			name:  "articles and prepositions dropped",
			claim: "the effect of probiotics on the gut",
			want:  "effect probiotics gut",
		},
		{
			name:  "verb-only claim yields nothing",
			claim: "reduces",
			want:  "",
		},
		{
			name:  "empty claim yields nothing",
			claim: "",
			want:  "",
		},
		{
			name:  "stopwords only yields nothing",
			claim: "the of and",
			want:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildNarrowedSearch(tc.claim); got != tc.want {
				t.Errorf("buildNarrowedSearch(%q)\n got: %q\nwant: %q", tc.claim, got, tc.want)
			}
		})
	}
}

func TestNarrowSearchDefaultsOff(t *testing.T) {
	t.Setenv("SC_NARROW_SEARCH", "")
	if narrowSearchEnabled() {
		t.Fatal("narrowed search must be off unless SC_NARROW_SEARCH=1")
	}
	t.Setenv("SC_NARROW_SEARCH", "0")
	if narrowSearchEnabled() {
		t.Fatal("SC_NARROW_SEARCH=0 must not enable narrowing")
	}
	t.Setenv("SC_NARROW_SEARCH", "true")
	if narrowSearchEnabled() {
		t.Fatal("only the exact value 1 enables narrowing")
	}
	t.Setenv("SC_NARROW_SEARCH", "1")
	if !narrowSearchEnabled() {
		t.Fatal("SC_NARROW_SEARCH=1 must enable narrowing")
	}
}