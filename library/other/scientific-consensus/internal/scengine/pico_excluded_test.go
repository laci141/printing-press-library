package scengine

// pico_excluded_test.go names the works the widened PICO gate would exclude.
//
// TestPICOImpactFullBenefitCorpora established that the exclusions on the
// benefit-shaped corpora are real, not truncation artifacts: saffron 5/12,
// omega3 30/62, melatonin 8/19, vitamind 5/26, meditation 2/10, probiotics 2/9.
// A percentage cannot say whether that is the gate working or the gate
// misfiring — only the titles can. This file prints them, with the side of the
// claim that failed to match, so each exclusion can be judged individually.
//
// Ordered by citation count descending: a heavily cited excluded work is the
// loudest signal that the tokenizer, not the paper, is at fault.
//
// Measurement only. Nothing here is asserted, nothing in the engine is touched.

import (
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// excludedWork is one work the gate would drop, with the reason.
type excludedWork struct {
	study    corpusStudy
	missesIV bool
	missesO  bool
}

// reason renders which side of the claim the work failed to name.
func (e excludedWork) reason() string {
	switch {
	case e.missesIV && e.missesO:
		return "BOTH"
	case e.missesIV:
		return "intervention"
	default:
		return "outcome"
	}
}

// excludedFrom returns every work the widened gate drops from a corpus,
// sorted by citations descending.
func excludedFrom(res corpusResult, iv, out []string) []excludedWork {
	var dropped []excludedWork
	for _, s := range res.AllStudies {
		if IsPICORelevant(s.Abstract, s.Title, iv, out) {
			continue
		}
		text := strings.ToLower(s.Abstract + " " + s.Title)
		dropped = append(dropped, excludedWork{
			study:    s,
			missesIV: !containsAnyToken(text, iv),
			missesO:  !containsAnyToken(text, out),
		})
	}
	sort.SliceStable(dropped, func(i, j int) bool {
		return dropped[i].study.CitedByCount > dropped[j].study.CitedByCount
	})
	return dropped
}

// TestPICOExcludedWorks prints the full roster of excluded works for each
// benefit-shaped corpus. This is the input to the decision on whether widening
// the verb recognizer is a net improvement.
func TestPICOExcludedWorks(t *testing.T) {
	for _, name := range benefitCorpora {
		full := mustLoadFullCorpus(t, name)
		iv, out := fixedPICOTokens(full.Claim)
		dropped := excludedFrom(full, iv, out)

		t.Logf("\n========== %s | claim=%q | iv=%v out=%v | %d of %d excluded",
			name, full.Claim, iv, out, len(dropped), len(full.AllStudies))
		for i, e := range dropped {
			t.Logf("  %2d. [cites=%-5d year=%-4d stance=%-12s missing=%s]\n      %s",
				i+1, e.study.CitedByCount, e.study.Year, e.study.Stance, e.reason(), e.study.Title)
		}
	}
}

// omega3Synonyms are the ways the literature names omega-3 fatty acids without
// using the word "omega". The claim's intervention side stems to a single token
// ("omega"), so every one of these is invisible to the gate. Counting how many
// excluded works use them measures what a synonym table would recover.
//
// EPA, DHA and PUFA are matched with word boundaries: "epa" is a substring of
// "hepatic" and "separate", which would otherwise inflate the count.
var omega3Synonyms = []struct {
	label string
	re    *regexp.Regexp
}{
	{"fish oil", regexp.MustCompile(`(?i)fish[- ]oil`)},
	{"EPA", regexp.MustCompile(`(?i)\bepa\b|eicosapentaenoic`)},
	{"DHA", regexp.MustCompile(`(?i)\bdha\b|docosahexaenoic`)},
	{"n-3", regexp.MustCompile(`(?i)\bn[-‐–]3\b`)},
	{"PUFA", regexp.MustCompile(`(?i)\bpufas?\b`)},
	{"polyunsaturated", regexp.MustCompile(`(?i)polyunsaturated`)},
}

// TestPICOExcludedOmega3Synonyms answers the specific question: of the works
// the gate drops from the omega3 corpus, how many name the intervention by a
// synonym the single "omega" token cannot see? A high count means a synonym
// table recovers more than widening the verb list does.
func TestPICOExcludedOmega3Synonyms(t *testing.T) {
	full := mustLoadFullCorpus(t, "omega3")
	iv, out := fixedPICOTokens(full.Claim)
	dropped := excludedFrom(full, iv, out)

	titleHits, anyHits := 0, 0
	perLabel := map[string]int{}

	t.Logf("omega3: %d excluded works, iv=%v out=%v", len(dropped), iv, out)
	for _, e := range dropped {
		var inTitle, inText []string
		for _, syn := range omega3Synonyms {
			if syn.re.MatchString(e.study.Title) {
				inTitle = append(inTitle, syn.label)
			}
			if syn.re.MatchString(e.study.Title + " " + e.study.Abstract) {
				inText = append(inText, syn.label)
				perLabel[syn.label]++
			}
		}
		if len(inTitle) > 0 {
			titleHits++
		}
		if len(inText) > 0 {
			anyHits++
		}
		if len(inTitle) > 0 || len(inText) > 0 {
			t.Logf("  [cites=%-5d missing=%-12s] title_syn=%v text_syn=%v\n      %s",
				e.study.CitedByCount, e.reason(), inTitle, inText, e.study.Title)
		}
	}

	t.Logf("\n  SYNONYM RECOVERY POTENTIAL")
	t.Logf("  excluded works whose TITLE names an omega-3 synonym : %d of %d", titleHits, len(dropped))
	t.Logf("  excluded works whose TITLE-OR-ABSTRACT names one    : %d of %d", anyHits, len(dropped))
	for _, syn := range omega3Synonyms {
		t.Logf("    %-16s %d", syn.label, perLabel[syn.label])
	}
}

// TestPICOExcludedMissingSideBreakdown summarizes, across all six benefit
// corpora, which side of the claim fails. The split matters for the fix: mostly
// intervention misses point at a synonym table for the intervention term;
// mostly outcome misses point at the outcome vocabulary instead.
func TestPICOExcludedMissingSideBreakdown(t *testing.T) {
	totIV, totOut, totBoth, tot := 0, 0, 0, 0
	for _, name := range benefitCorpora {
		full := mustLoadFullCorpus(t, name)
		iv, out := fixedPICOTokens(full.Claim)
		dropped := excludedFrom(full, iv, out)

		var a, b, c int
		for _, e := range dropped {
			switch {
			case e.missesIV && e.missesO:
				c++
			case e.missesIV:
				a++
			default:
				b++
			}
		}
		t.Logf("%-12s excluded=%2d | missing intervention only=%d, outcome only=%d, both=%d",
			name, len(dropped), a, b, c)
		totIV += a
		totOut += b
		totBoth += c
		tot += len(dropped)
	}
	t.Logf("TOTAL excluded=%d | intervention only=%d, outcome only=%d, both=%d",
		tot, totIV, totOut, totBoth)
}

// ---------------------------------------------------------------------------
// Missing-abstract analysis.
//
// 24% of works in these corpora arrive with no abstract at all, and they make
// up 35% of the gate's exclusions. Two candidate fixes point at that: narrowing
// the OpenAlex search so fewer off-topic works are fetched, and backfilling
// abstracts from another source. The three tests below produce the raw facts
// needed to order those two, and NOTHING ELSE — no classification, no verdict,
// no opinion is computed here. Judgement belongs in
// testdata/MISSING_ABSTRACT_ANALYSIS.md, written by a human (or by a model that
// signs its name to it), never in a test that prints numbers.
//
// The gate logic is the existing fixedPICOTokens + containsAnyToken. No second
// tokenizer is defined anywhere in this file, so a difference between these
// numbers and the earlier ones can only come from the data, not from drift
// between two implementations of the same rule.
//
// Simulation premise for "would the narrowed search still return this work":
// OpenAlex's title_and_abstract search sees title + abstract. For a work whose
// abstract is absent, that reduces to the title alone — which is checkable
// offline, with no live API call.

// noAbstractFact is one work with no abstract, reduced to the facts that decide
// whether narrowing the search or backfilling the abstract would help it.
type noAbstractFact struct {
	corpus     string
	study      corpusStudy
	ivInTitle  bool
	outInTitle bool
	passesGate bool
}

// collectNoAbstractFacts gathers every abstract-less work across the six
// benefit-shaped corpora.
func collectNoAbstractFacts(t *testing.T) []noAbstractFact {
	t.Helper()
	var facts []noAbstractFact
	for _, name := range benefitCorpora {
		full := mustLoadFullCorpus(t, name)
		iv, out := fixedPICOTokens(full.Claim)
		for _, s := range full.AllStudies {
			if strings.TrimSpace(s.Abstract) != "" {
				continue
			}
			title := strings.ToLower(s.Title)
			facts = append(facts, noAbstractFact{
				corpus:     name,
				study:      s,
				ivInTitle:  containsAnyToken(title, iv),
				outInTitle: containsAnyToken(title, out),
				passesGate: IsPICORelevant(s.Abstract, s.Title, iv, out),
			})
		}
	}
	sort.SliceStable(facts, func(i, j int) bool {
		return facts[i].study.CitedByCount > facts[j].study.CitedByCount
	})
	return facts
}

// TestNoAbstractQ1TitleCarriesIntervention answers: for works with no abstract,
// does the title alone carry the intervention token?
//
// A "yes" means a title_and_abstract-narrowed search would still return the
// work, so backfilling its abstract is worth doing. A "no" means the narrowed
// search would never fetch it, so backfilling would be wasted on it.
func TestNoAbstractQ1TitleCarriesIntervention(t *testing.T) {
	facts := collectNoAbstractFacts(t)

	type row struct{ total, ivYes int }
	per := map[string]*row{}
	for _, name := range benefitCorpora {
		per[name] = &row{}
	}
	for _, f := range facts {
		per[f.corpus].total++
		if f.ivInTitle {
			per[f.corpus].ivYes++
		}
	}

	t.Logf("%-12s %14s %26s %26s", "corpus", "no abstract", "IV token IS in title", "IV token NOT in title")
	var sumT, sumY int
	for _, name := range benefitCorpora {
		r := per[name]
		t.Logf("%-12s %14d %26d %26d", name, r.total, r.ivYes, r.total-r.ivYes)
		sumT += r.total
		sumY += r.ivYes
	}
	t.Logf("%-12s %14d %26d %26d", "TOTAL", sumT, sumY, sumT-sumY)

	t.Logf("\nevery abstract-less work, by citations:")
	for _, f := range facts {
		t.Logf("  [iv_in_title=%-5v out_in_title=%-5v passes_gate=%-5v] cites=%-6d %-11s %s",
			f.ivInTitle, f.outInTitle, f.passesGate, f.study.CitedByCount, f.corpus, f.study.Title)
	}
}

// TestNoAbstractQ2FailOpenAdmits answers: if the rule were "no abstract → never
// exclude", which works would be admitted that the gate excludes today?
//
// Names and citation counts only. Whether each admission is an improvement or a
// regression is not decided here.
func TestNoAbstractQ2FailOpenAdmits(t *testing.T) {
	facts := collectNoAbstractFacts(t)

	n := 0
	t.Logf("works with no abstract that the gate excludes today, and that a fail-open rule would admit:")
	for _, f := range facts {
		if f.passesGate {
			continue
		}
		n++
		t.Logf("  %2d. cites=%-6d %-11s [missing in title: %s]\n      %s",
			n, f.study.CitedByCount, f.corpus, missingSideLabel(f), f.study.Title)
	}
	t.Logf("\ntotal admitted by fail-open: %d (of %d abstract-less works)", n, len(facts))
}

// missingSideLabel names which side of the claim the title fails to carry.
func missingSideLabel(f noAbstractFact) string {
	switch {
	case !f.ivInTitle && !f.outInTitle:
		return "intervention AND outcome"
	case !f.ivInTitle:
		return "intervention"
	default:
		return "outcome"
	}
}

// TestNoAbstractQ3NarrowingInteraction splits the Q2 list by whether a
// title_and_abstract-narrowed search would still have fetched the work.
//
// The works in the "dropped by narrowing" group never reach the gate at all
// once the search is narrowed, so fail-open cannot admit them. The works in the
// "survives narrowing" group would still be admitted by fail-open even after
// the search change — those are the ones whose admission has to be judged on
// its merits.
func TestNoAbstractQ3NarrowingInteraction(t *testing.T) {
	facts := collectNoAbstractFacts(t)

	var survives, dropped []noAbstractFact
	for _, f := range facts {
		if f.passesGate {
			continue
		}
		if f.ivInTitle {
			survives = append(survives, f)
		} else {
			dropped = append(dropped, f)
		}
	}

	t.Logf("SURVIVES a title_and_abstract-narrowed search (intervention token is in the title) — %d works:", len(survives))
	for i, f := range survives {
		t.Logf("  %2d. cites=%-6d %-11s %s", i+1, f.study.CitedByCount, f.corpus, f.study.Title)
	}
	t.Logf("\nDROPPED by a title_and_abstract-narrowed search (no intervention token in the title) — %d works:", len(dropped))
	for i, f := range dropped {
		t.Logf("  %2d. cites=%-6d %-11s %s", i+1, f.study.CitedByCount, f.corpus, f.study.Title)
	}
	t.Logf("\nsplit: %d survive narrowing, %d removed by it", len(survives), len(dropped))
}

// gateVerdictExport is the machine-readable form of one corpus's gate verdicts.
type gateVerdictExport struct {
	Corpus    string   `json:"corpus"`
	Claim     string   `json:"claim"`
	IVTokens  []string `json:"iv_tokens"`
	OutTokens []string `json:"out_tokens"`
	Kept      []string `json:"kept_dois"`
	Excluded  []string `json:"excluded_dois"`
	NoDOI     []string `json:"titles_without_doi"`
}

// TestExportGateVerdicts writes the gate's per-work verdict to a JSON file so
// downstream analysis reads structured data instead of scraping this test's log
// output. A scraper that silently matches nothing produces empty sets and
// numbers that look like findings; a JSON file either parses or fails loudly.
//
// Skipped unless PICO_EXPORT names an output path, so a normal `go test` run
// never writes files:
//
//	PICO_EXPORT=/tmp/verdicts.json go test ./internal/scengine/ -run TestExportGateVerdicts
//
// DOIs are lowercased and stripped of any resolver prefix here, so the consumer
// never has to re-derive that normalization — one normalizer, one place.
func TestExportGateVerdicts(t *testing.T) {
	path := os.Getenv("PICO_EXPORT")
	if path == "" {
		t.Skip("PICO_EXPORT not set; nothing to export")
	}

	out := make([]gateVerdictExport, 0, len(benefitCorpora))
	for _, name := range benefitCorpora {
		full := mustLoadFullCorpus(t, name)
		iv, o := fixedPICOTokens(full.Claim)
		e := gateVerdictExport{
			Corpus: name, Claim: full.Claim, IVTokens: iv, OutTokens: o,
			Kept: []string{}, Excluded: []string{}, NoDOI: []string{},
		}
		for _, s := range full.AllStudies {
			d := normalizeDOI(s.DOI)
			if d == "" {
				e.NoDOI = append(e.NoDOI, s.Title)
				continue
			}
			if IsPICORelevant(s.Abstract, s.Title, iv, o) {
				e.Kept = append(e.Kept, d)
			} else {
				e.Excluded = append(e.Excluded, d)
			}
		}
		t.Logf("%-12s kept=%d excluded=%d no_doi=%d", name, len(e.Kept), len(e.Excluded), len(e.NoDOI))
		out = append(out, e)
	}

	blob, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(blob))
}

// normalizeDOI reduces a DOI to the bare lowercase form used for set
// comparison against OpenAlex responses, which return the resolver URL.
func normalizeDOI(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "https://doi.org/")
	s = strings.TrimPrefix(s, "http://doi.org/")
	s = strings.TrimPrefix(s, "doi:")
	return s
}
