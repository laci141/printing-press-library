package source

import (
	"encoding/xml"
	"strings"
	"testing"
)

// efetch payloads below are trimmed real-shape PubMed XML. They exist to pin the
// PARSING CONTRACT — which elements count as "this article's abstract" — because
// that contract is where this feature can fail silently: a mis-scoped path does
// not error, it attaches the wrong text to a real work and every downstream gate
// then believes it.

// structuredFixture is the common case: a labeled, sectioned abstract.
const structuredFixture = `<?xml version="1.0" ?>
<PubmedArticleSet>
 <PubmedArticle>
  <MedlineCitation>
   <PMID Version="1">22873530</PMID>
   <Article>
    <ArticleTitle>Tofacitinib in rheumatoid arthritis</ArticleTitle>
    <Abstract>
     <AbstractText Label="BACKGROUND" NlmCategory="BACKGROUND">Tofacitinib is an oral Janus kinase inhibitor.</AbstractText>
     <AbstractText Label="METHODS" NlmCategory="METHODS">611 patients were randomly assigned.</AbstractText>
    </Abstract>
   </Article>
  </MedlineCitation>
  <PubmedData>
   <ArticleIdList>
    <ArticleId IdType="pubmed">22873530</ArticleId>
    <ArticleId IdType="doi">10.1056/NEJMoa1109071</ArticleId>
   </ArticleIdList>
  </PubmedData>
 </PubmedArticle>
</PubmedArticleSet>`

func TestAbstractsFromFetchSetStructured(t *testing.T) {
	got := parseFixture(t, structuredFixture)
	want := "BACKGROUND: Tofacitinib is an oral Janus kinase inhibitor. METHODS: 611 patients were randomly assigned."
	if got["22873530"] != want {
		t.Errorf("structured abstract\n got %q\nwant %q", got["22873530"], want)
	}
}

// flatFixture has no Label attribute, which is how unstructured abstracts and
// many older records arrive. The renderer must not invent a ": " prefix.
const flatFixture = `<PubmedArticleSet>
 <PubmedArticle>
  <MedlineCitation>
   <PMID Version="2">100</PMID>
   <Article><Abstract><AbstractText>A single unlabelled paragraph.</AbstractText></Abstract></Article>
  </MedlineCitation>
 </PubmedArticle>
</PubmedArticleSet>`

func TestAbstractsFromFetchSetFlat(t *testing.T) {
	got := parseFixture(t, flatFixture)
	if got["100"] != "A single unlabelled paragraph." {
		t.Errorf("flat abstract: got %q", got["100"])
	}
	if strings.Contains(got["100"], ":") {
		t.Errorf("unlabelled section must not gain a label prefix: %q", got["100"])
	}
}

// TestAbstractsFromFetchSetIgnoresForeignElements is the regression guard that
// justifies the path-scoped struct tags in pubmedFetchArticle.
//
// A PubmedArticle carries AbstractText and ArticleId elements that are NOT the
// article's own: <OtherAbstract> holds publisher or translated abstracts, and
// every <ReferenceList> entry carries its own <ArticleIdList>. Measured against
// the live API on 41 records, a parser that collected ArticleId from anywhere in
// the subtree bound 8 of them to a CITED paper's DOI. If these tags are ever
// loosened to `xml:"...>AbstractText"` shortcuts, this test fails instead of the
// CLI quietly reporting someone else's research.
const foreignElementsFixture = `<PubmedArticleSet>
 <PubmedArticle>
  <MedlineCitation>
   <PMID Version="1">200</PMID>
   <Article><Abstract><AbstractText Label="RESULTS">The article's own finding.</AbstractText></Abstract></Article>
   <OtherAbstract Type="Publisher" Language="fre">
    <AbstractText>Resume traduit qui ne doit jamais apparaitre.</AbstractText>
   </OtherAbstract>
  </MedlineCitation>
  <PubmedData>
   <ArticleIdList><ArticleId IdType="doi">10.1000/own</ArticleId></ArticleIdList>
   <ReferenceList>
    <Reference>
     <Citation>Some cited paper.</Citation>
     <ArticleIdList><ArticleId IdType="doi">10.1000/cited-not-ours</ArticleId></ArticleIdList>
    </Reference>
   </ReferenceList>
  </PubmedData>
 </PubmedArticle>
</PubmedArticleSet>`

func TestAbstractsFromFetchSetIgnoresForeignElements(t *testing.T) {
	got := parseFixture(t, foreignElementsFixture)
	abs := got["200"]
	if abs != "RESULTS: The article's own finding." {
		t.Errorf("abstract\n got %q\nwant %q", abs, "RESULTS: The article's own finding.")
	}
	if strings.Contains(abs, "traduit") {
		t.Errorf("OtherAbstract leaked into the article's abstract: %q", abs)
	}
	if strings.Contains(abs, "cited-not-ours") {
		t.Errorf("ReferenceList content leaked into the article's abstract: %q", abs)
	}
}

// markupFixture carries the inline markup that makes `xml:",chardata"` wrong:
// measured 2026-08-02, 5 of 59 live AbstractText elements contained <sup>/<sub>.
// chardata collects only an element's DIRECT text, so the nested digits and the
// entity would vanish.
const markupFixture = `<PubmedArticleSet>
 <PubmedArticle>
  <MedlineCitation>
   <PMID>300</PMID>
   <Article><Abstract>
    <AbstractText>Serum 25(OH)D<sub>3</sub> rose while CO<sub>2</sub> fell; affron<sup>&#xae;</sup> was well tolerated &amp; safe.</AbstractText>
   </Abstract></Article>
  </MedlineCitation>
 </PubmedArticle>
</PubmedArticleSet>`

func TestAbstractsFromFetchSetKeepsNestedMarkupText(t *testing.T) {
	abs := parseFixture(t, markupFixture)["300"]
	for _, want := range []string{"25(OH)D3", "CO2", "affron®", "& safe"} {
		if !strings.Contains(abs, want) {
			t.Errorf("nested markup/entity lost: %q missing from %q", want, abs)
		}
	}
}

// A record with no abstract must be ABSENT from the map, never present with an
// empty value: the caller distinguishes "PubMed has no abstract for this work"
// from "this work was never looked up", and an empty string would erase that.
const noAbstractFixture = `<PubmedArticleSet>
 <PubmedArticle>
  <MedlineCitation><PMID>400</PMID><Article><ArticleTitle>Title only</ArticleTitle></Article></MedlineCitation>
 </PubmedArticle>
 <PubmedArticle>
  <MedlineCitation><PMID>401</PMID><Article><Abstract><AbstractText>   </AbstractText></Abstract></Article></MedlineCitation>
 </PubmedArticle>
 <PubmedArticle>
  <MedlineCitation><PMID></PMID><Article><Abstract><AbstractText>Orphaned.</AbstractText></Abstract></Article></MedlineCitation>
 </PubmedArticle>
</PubmedArticleSet>`

func TestAbstractsFromFetchSetOmitsEmptyAndUnkeyedRecords(t *testing.T) {
	got := parseFixture(t, noAbstractFixture)
	if len(got) != 0 {
		t.Errorf("expected no entries, got %#v", got)
	}
	if _, ok := got["400"]; ok {
		t.Error("record without an Abstract element must be absent, not empty")
	}
	if _, ok := got["401"]; ok {
		t.Error("record with whitespace-only AbstractText must be absent, not empty")
	}
}

func TestFlattenAbstractXML(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{"plain", "just text", "just text"},
		{"collapses whitespace", "line one\n   line   two", "line one line two"},
		{"keeps nested text", "CO<sub>2</sub> levels", "CO2 levels"},
		{"resolves entities", "a &amp; b &lt;c&gt;", "a & b <c>"},
		{"malformed yields prefix", "good text <unclosed", "good text"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := flattenAbstractXML(c.in); got != c.want {
				t.Errorf("flattenAbstractXML(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestPubMedAbstractsByDOIEmptyInputMakesNoRequest(t *testing.T) {
	// No httptest server is registered, so any outbound call would fail the
	// test by erroring; the contract is that an empty input short-circuits.
	got, err := PubMedAbstractsByDOI(t.Context(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty map, got %#v", got)
	}
}

func parseFixture(t *testing.T, doc string) map[string]string {
	t.Helper()
	var set pubmedFetchSet
	if err := xml.Unmarshal([]byte(doc), &set); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	return abstractsFromFetchSet(set)
}
