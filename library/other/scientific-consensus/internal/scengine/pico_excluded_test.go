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
