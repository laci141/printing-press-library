// Hand-authored tests for the version dedupe. Not generated.
package cli

import (
	"strings"
	"testing"
)

// TestDedupeCollapsesCochraneFamily pins the corpus that motivated this gate.
//
// Measured live 2026-08-03, keyless heuristic run, claim "vitamin C prevents
// the common cold", --limit 10: nine works survived the relevance gates and
// FOUR of them were the same Cochrane review CD000980 — .pub4 (2013), .pub3
// (2007), .pub2 (2004) and the suffix-less 1998 original. One review cast 44%
// of the votes, and because Consensus() weights by citations it carried its
// authority in four times over as well.
//
// The fixture reproduces that family plus the five unrelated works that shared
// the corpus with it, so a regression that collapses too much fails here rather
// than in production.
func TestDedupeCollapsesCochraneFamily(t *testing.T) {
	works := []scWork{
		{DOI: "10.1002/14651858.cd000980.pub4", Year: 2013, CitedBy: 816, Title: "Vitamin C for preventing and treating the common cold"},
		{DOI: "10.1002/14651858.cd000980.pub3", Year: 2007, CitedBy: 421, Title: "Vitamin C for preventing and treating the common cold"},
		{DOI: "10.5867/medwave.2018.04.7236", Year: 2018, CitedBy: 12, Title: "Unrelated review"},
		{DOI: "10.1002/14651858.cd000980.pub2", Year: 2004, CitedBy: 203, Title: "Vitamin C for preventing and treating the common cold"},
		{DOI: "10.1155/2018/5813095", Year: 2018, CitedBy: 31, Title: "Another unrelated work"},
		{DOI: "10.1371/journal.pmed.0020168", Year: 2005, CitedBy: 275, Title: "Yet another work"},
		{DOI: "10.1002/14651858.cd000980", Year: 1998, CitedBy: 51, Title: "Vitamin C for preventing and treating the common cold"},
		{DOI: "10.3390/nu9040339", Year: 2017, CitedBy: 88, Title: "Nutrients review"},
		{DOI: "10.1007/bf02850271", Year: 2002, CitedBy: 44, Title: "An RCT"},
	}

	got, rep := dedupeVersions(works)

	if len(got) != 6 {
		t.Errorf("survivor count = %d, want 6 (9 works, one 4-edition family)", len(got))
	}
	if rep.Excluded != 3 {
		t.Errorf("Excluded = %d, want 3", rep.Excluded)
	}

	// The newest edition, and ONLY the newest, survives the family.
	var kept []string
	for _, w := range got {
		if strings.Contains(w.DOI, "cd000980") {
			kept = append(kept, w.DOI)
		}
	}
	if len(kept) != 1 {
		t.Fatalf("family survivors = %v, want exactly one", kept)
	}
	if kept[0] != "10.1002/14651858.cd000980.pub4" {
		t.Errorf("kept %q, want the 2013 .pub4 edition", kept[0])
	}

	// Every unrelated work is untouched. A dedupe that quietly takes a
	// non-duplicate is worse than one that misses a duplicate: the first
	// removes evidence, the second only fails to consolidate it.
	for _, doi := range []string{
		"10.5867/medwave.2018.04.7236",
		"10.1155/2018/5813095",
		"10.1371/journal.pmed.0020168",
		"10.3390/nu9040339",
		"10.1007/bf02850271",
	} {
		found := false
		for _, w := range got {
			if w.DOI == doi {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unrelated work %s was dropped", doi)
		}
	}

	// The note must name what was merged, not just how many. A bare count is a
	// number the reader has to take on faith.
	if len(rep.Families) != 1 {
		t.Fatalf("Families = %v, want one entry", rep.Families)
	}
	fam := rep.Families[0]
	for _, want := range []string{"10.1002/14651858.cd000980", "kept 2013 (.pub4)", "1998 (original)", "2004 (.pub2)", "2007 (.pub3)"} {
		if !strings.Contains(fam, want) {
			t.Errorf("family note %q missing %q", fam, want)
		}
	}
}

// TestDedupePreservesOrder pins that survivors keep their fetch (relevance)
// order. all_studies is documented as being in relevance order and downstream
// consumers re-filter on it, so a dedupe that reshuffles would corrupt a
// contract it never touched.
func TestDedupePreservesOrder(t *testing.T) {
	works := []scWork{
		{DOI: "10.1/a", Year: 2020},
		{DOI: "10.2/b.pub2", Year: 2015},
		{DOI: "10.3/c", Year: 2019},
		{DOI: "10.2/b.pub3", Year: 2021},
		{DOI: "10.4/d", Year: 2018},
	}

	got, rep := dedupeVersions(works)

	if rep.Excluded != 1 {
		t.Fatalf("Excluded = %d, want 1", rep.Excluded)
	}
	want := []string{"10.1/a", "10.3/c", "10.2/b.pub3", "10.4/d"}
	if len(got) != len(want) {
		t.Fatalf("survivors = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].DOI != w {
			t.Errorf("position %d = %q, want %q", i, got[i].DOI, w)
		}
	}
}

// TestDedupeNeverCollapsesMissingDOIs pins the rule that makes this gate safe
// on incomplete metadata.
//
// doiFamily returns "" for a work with no DOI. If the caller treated that as a
// family key, every DOI-less work in a corpus would collapse into one — five
// unrelated studies reduced to a single vote, silently. OpenAlex ships works
// without DOIs routinely, so this is the ordinary case, not an edge case.
func TestDedupeNeverCollapsesMissingDOIs(t *testing.T) {
	works := []scWork{
		{DOI: "", Year: 2020, Title: "First work with no DOI"},
		{DOI: "", Year: 2019, Title: "Second, unrelated, also no DOI"},
		{DOI: "   ", Year: 2018, Title: "Whitespace-only DOI"},
		{DOI: "10.1/real", Year: 2021, Title: "Has a DOI"},
	}

	got, rep := dedupeVersions(works)

	if rep.Excluded != 0 {
		t.Errorf("Excluded = %d, want 0 — works without a DOI are never deduplicable", rep.Excluded)
	}
	if len(got) != 4 {
		t.Errorf("survivors = %d, want all 4", len(got))
	}
	if len(rep.Families) != 0 {
		t.Errorf("Families = %v, want none", rep.Families)
	}
}

// TestDedupeYearTieBreaksOnEdition pins the tie-break, which is not decoration:
// OpenAlex records some editions under their reissue year rather than their
// original one, so same-year siblings within a family do occur. Without a
// deterministic rule the survivor would depend on fetch order, and two
// identical runs could disagree about which edition the corpus contains.
func TestDedupeYearTieBreaksOnEdition(t *testing.T) {
	// Same year, different editions: the higher .pubN wins.
	works := []scWork{
		{DOI: "10.1002/14651858.cd000980.pub2", Year: 2013},
		{DOI: "10.1002/14651858.cd000980.pub4", Year: 2013},
		{DOI: "10.1002/14651858.cd000980.pub3", Year: 2013},
	}
	got, rep := dedupeVersions(works)
	if rep.Excluded != 2 {
		t.Fatalf("Excluded = %d, want 2", rep.Excluded)
	}
	if len(got) != 1 || got[0].DOI != "10.1002/14651858.cd000980.pub4" {
		t.Errorf("kept %v, want only the .pub4 edition", got)
	}

	// Reversed input order must give the same answer. A tie-break that depends
	// on which sibling arrived first is not a rule.
	reversed := []scWork{
		{DOI: "10.1002/14651858.cd000980.pub4", Year: 2013},
		{DOI: "10.1002/14651858.cd000980.pub3", Year: 2013},
		{DOI: "10.1002/14651858.cd000980.pub2", Year: 2013},
	}
	got2, _ := dedupeVersions(reversed)
	if len(got2) != 1 || got2[0].DOI != "10.1002/14651858.cd000980.pub4" {
		t.Errorf("reversed input kept %v, want only the .pub4 edition", got2)
	}

	// The suffix-less original is edition 1, so it loses a same-year tie
	// against any .pubN sibling.
	orig := []scWork{
		{DOI: "10.1002/14651858.cd000980", Year: 2004},
		{DOI: "10.1002/14651858.cd000980.pub2", Year: 2004},
	}
	got3, _ := dedupeVersions(orig)
	if len(got3) != 1 || got3[0].DOI != "10.1002/14651858.cd000980.pub2" {
		t.Errorf("kept %v, want the .pub2 edition over the original", got3)
	}
}

// TestDOIFamilyNormalizes pins the input shapes doiFamily has to survive.
// OpenAlex hands DOIs back as bare strings about half the time and as
// https://doi.org/ URLs the rest, in mixed case, and a family key that treats
// those as different works would never fire.
func TestDOIFamilyNormalizes(t *testing.T) {
	const want = "10.1002/14651858.cd000980"
	for _, in := range []string{
		"10.1002/14651858.cd000980",
		"10.1002/14651858.CD000980.pub4",
		"https://doi.org/10.1002/14651858.cd000980.pub2",
		"http://doi.org/10.1002/14651858.cd000980.pub3",
		"doi:10.1002/14651858.cd000980.pub4",
		"  10.1002/14651858.cd000980.pub4  ",
	} {
		if got := doiFamily(in); got != want {
			t.Errorf("doiFamily(%q) = %q, want %q", in, got, want)
		}
	}

	if got := doiFamily(""); got != "" {
		t.Errorf("doiFamily(\"\") = %q, want empty", got)
	}
	if got := doiFamily("   "); got != "" {
		t.Errorf("doiFamily(whitespace) = %q, want empty", got)
	}

	// ".pub" must be anchored to the END. A DOI whose body happens to contain
	// "pub" — publisher prefixes do — must come through untouched.
	if got := doiFamily("10.1234/pubmed.2020.pub2"); got != "10.1234/pubmed.2020" {
		t.Errorf("doiFamily = %q, want the body preserved with only the tail stripped", got)
	}
	if got := doiFamily("10.1234/journal.pub.study"); got != "10.1234/journal.pub.study" {
		t.Errorf("doiFamily stripped a non-suffix 'pub': %q", got)
	}
}

// TestDedupeEmptyAndSingleton pins the degenerate inputs. An empty corpus
// reaches this function whenever both relevance gates emptied it, which the
// consensus command handles as a normal result rather than an error.
func TestDedupeEmptyAndSingleton(t *testing.T) {
	got, rep := dedupeVersions(nil)
	if len(got) != 0 || rep.Excluded != 0 || len(rep.Families) != 0 {
		t.Errorf("nil input: got %v, %+v; want empty everything", got, rep)
	}

	one := []scWork{{DOI: "10.1/only", Year: 2020}}
	got2, rep2 := dedupeVersions(one)
	if len(got2) != 1 || rep2.Excluded != 0 || len(rep2.Families) != 0 {
		t.Errorf("singleton: got %v, %+v; want the work kept and no families", got2, rep2)
	}
}
