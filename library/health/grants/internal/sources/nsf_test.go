package sources

import (
	"reflect"
	"testing"
)

func TestNSFStem(t *testing.T) {
	cases := []struct{ in, want string }{
		{"resilience", "resil"},
		{"resilient", "resil"},
		{"resiliency", "resil"},
		{"computing", "comput"},
		{"therapies", "therap"},
		{"therapy", "therap"},
		{"published", "publish"},
		{"genes", "gene"},
		{"gene", "gene"},   // four letters left is the floor, so nothing is trimmed
		{"aging", "aging"}, // trimming "-ing" would leave only two characters
		{"characterization", "character"},
		{"sustainability", "sustain"},
		{"observations", "observ"},
		{"measurements", "measur"},
		{"Climate", "climate"},
	}
	for _, c := range cases {
		if got := nsfStem(c.in); got != c.want {
			t.Errorf("nsfStem(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNSFTerms(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"climate resilience", []string{"climate", "resil"}},
		{"gene therapy", []string{"gene", "therap"}},
		{"the use of AI for imaging", []string{"imag"}},
		{"cancer", []string{"cancer"}},
		{"", nil},
	}
	for _, c := range cases {
		if got := nsfTerms(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("nsfTerms(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// The measured case: "climate resilience" must match an abstract that only ever
// says "resilient".
func TestNSFScoreMatchesByStem(t *testing.T) {
	award := nsfAwardRaw{
		NSFAward: NSFAward{Title: "Coastal Infrastructure Study"},
		Abstract: "This project studies how climate change makes coastal towns more resilient.",
	}
	score, titleHit, ok := nsfScore(award, nsfTerms("climate resilience"))
	if !ok {
		t.Fatal("expected an abstract saying \"resilient\" to match the term \"resilience\"")
	}
	if titleHit {
		t.Error("titleHit = true, want false: neither term is in the title")
	}
	if score != 2 {
		t.Errorf("score = %d, want 2 (two abstract hits)", score)
	}
}

func TestNSFScore(t *testing.T) {
	cases := []struct {
		name     string
		award    nsfAwardRaw
		query    string
		wantOK   bool
		wantHit  bool
		wantScrE int
	}{
		{
			name:     "both terms in title",
			award:    nsfAwardRaw{NSFAward: NSFAward{Title: "A Novel Gene Therapy Platform", ID: "1"}, Abstract: "delivery vectors"},
			query:    "gene therapy",
			wantOK:   true,
			wantHit:  true,
			wantScrE: 20,
		},
		{
			name:     "one term in title, one in abstract",
			award:    nsfAwardRaw{NSFAward: NSFAward{Title: "Gene Editing in Zebrafish", ID: "2"}, Abstract: "a therapeutic approach"},
			query:    "gene therapy",
			wantOK:   true,
			wantHit:  true,
			wantScrE: 11,
		},
		{
			name:    "missing a term is rejected",
			award:   nsfAwardRaw{NSFAward: NSFAward{Title: "Squid Hydrodynamics", ID: "3"}, Abstract: "artificial neural networks are used"},
			query:   "artificial intelligence",
			wantOK:  false,
			wantHit: false,
		},
		{
			name:     "empty query keeps everything",
			award:    nsfAwardRaw{NSFAward: NSFAward{Title: "Anything", ID: "4"}, Abstract: ""},
			query:    "",
			wantOK:   true,
			wantHit:  false,
			wantScrE: 0,
		},
	}
	for _, c := range cases {
		score, hit, ok := nsfScore(c.award, nsfTerms(c.query))
		if ok != c.wantOK || hit != c.wantHit || (ok && score != c.wantScrE) {
			t.Errorf("%s: nsfScore = (%d, %v, %v), want (%d, %v, %v)",
				c.name, score, hit, ok, c.wantScrE, c.wantHit, c.wantOK)
		}
	}
}

func TestNSFRank(t *testing.T) {
	pool := []nsfAwardRaw{
		{NSFAward: NSFAward{ID: "1", Title: "Squid Hydrodynamics", FundsObligated: "900000"},
			Abstract: "artificial neural sampling, no second term here"},
		{NSFAward: NSFAward{ID: "2", Title: "Graduate Fellowship", FundsObligated: "100000"},
			Abstract: "supports the artificial intelligence priority area"},
		{NSFAward: NSFAward{ID: "3", Title: "Artificial Intelligence for Robotics", FundsObligated: "500000"},
			Abstract: "an artificial intelligence system"},
		// Same collaborative award, registered per institution: different IDs,
		// identical titles. The larger fundsObligatedAmt must win.
		{NSFAward: NSFAward{ID: "4a", Title: "Collaborative Research: AI and Intelligence Testing", FundsObligated: "200000"},
			Abstract: "artificial methods"},
		{NSFAward: NSFAward{ID: "4b", Title: "collaborative research:  AI and Intelligence Testing ", FundsObligated: "800000"},
			Abstract: "artificial methods"},
	}

	awards, stats := nsfRank(pool, nsfTerms("artificial intelligence"), 5)

	if stats.Examined != 5 {
		t.Errorf("Examined = %d, want 5", stats.Examined)
	}
	if stats.Matched != 3 {
		t.Errorf("Matched = %d, want 3 (one rejected, one deduped)", stats.Matched)
	}
	if stats.TitleMatched != 2 {
		t.Errorf("TitleMatched = %d, want 2", stats.TitleMatched)
	}
	wantIDs := []string{"3", "4b", "2"}
	var gotIDs []string
	for _, a := range awards {
		gotIDs = append(gotIDs, a.ID)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Errorf("ranked IDs = %v, want %v", gotIDs, wantIDs)
	}
}

func TestNSFRankLimitsRows(t *testing.T) {
	pool := []nsfAwardRaw{
		{NSFAward: NSFAward{ID: "1", Title: "Cancer A"}, Abstract: "cancer"},
		{NSFAward: NSFAward{ID: "2", Title: "Cancer B"}, Abstract: "cancer"},
		{NSFAward: NSFAward{ID: "3", Title: "Cancer C"}, Abstract: "cancer"},
	}
	awards, stats := nsfRank(pool, nsfTerms("cancer"), 2)
	if len(awards) != 2 {
		t.Errorf("len(awards) = %d, want 2", len(awards))
	}
	if stats.Matched != 3 {
		t.Errorf("Matched = %d, want 3 (stats count matches, not shown rows)", stats.Matched)
	}
}

// The PI must reach the normalised output in the NIH path's
// "SURNAME, FIRSTNAME" shape, built from the split pair. The measured hazard is
// a multi-word surname: reading the combined pdPIName instead and taking its
// last word as the surname misfiles "Emma A Elliott Smith" (9 of 236 awards
// measured 2026-09-04).
func TestNSFRankEmitsPI(t *testing.T) {
	pool := []nsfAwardRaw{
		{NSFAward: NSFAward{ID: "1", Title: "Isotope Ecology"}, Abstract: "ecology",
			PIFirstName: "Emma", PILastName: "Elliott Smith"},
	}
	awards, _ := nsfRank(pool, nsfTerms("ecology"), 5)
	if len(awards) != 1 {
		t.Fatalf("len(awards) = %d, want 1", len(awards))
	}
	if got, want := awards[0].PI, "Elliott Smith, Emma"; got != want {
		t.Errorf("PI = %q, want %q", got, want)
	}
}

func TestNSFPIName(t *testing.T) {
	cases := []struct{ first, last, want string }{
		{"Jenny", "Ouyang", "Ouyang, Jenny"},
		{"Emma", "Elliott Smith", "Elliott Smith, Emma"},
		{" Emma ", " Elliott Smith ", "Elliott Smith, Emma"},
		{"", "Ouyang", "Ouyang"}, // no stray leading comma
		{"Jenny", "", "Jenny"},   // no stray trailing comma
		{"", "", ""},
	}
	for _, c := range cases {
		if got := nsfPIName(c.first, c.last); got != c.want {
			t.Errorf("nsfPIName(%q, %q) = %q, want %q", c.first, c.last, got, c.want)
		}
	}
}
