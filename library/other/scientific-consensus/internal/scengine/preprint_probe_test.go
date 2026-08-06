package scengine

// preprint_probe_test.go measures how often the same research appears more
// than once in a corpus under different DOIs, and how much of that a preprint
// rule could safely remove. Log-only; nothing here asserts.
//
// WHY, measured 2026-08-06 on the HPV corpus at limit 100:
//
//	50 works, 48 distinct titles, duplicate_excluded = 1.
//
// dedupeVersions (internal/cli/dedupe.go:71) groups strictly by DOI family and
// skips any work without a DOI. That is deliberate — two papers can share a
// title without being the same work.
//
// The one real family it caught was two Zenodo uploads of the same paper
// (10.5281/zenodo.17528083 and ...84). The one it could not catch is the
// ordinary publication path of a single study across three publishers:
//
//	10.1101/19012344                 medRxiv preprint       2019,  1 cite
//	10.1097/01.aog.0000559416...     conference abstract    2019,  0 cites
//	10.1016/j.vaccine.2020.03.035    final journal article  2020, 37 cites
//
// All three are "No Association between HPV Vaccination and Infertility in
// U.S. Females 18-33 Years Old". One study, three DOI prefixes, three votes.
// Here all three vote refuting, which happens to be correct — but that is
// luck. On another claim the same triplication would amplify a wrong answer.
//
// THE MEASUREMENT, 2026-08-06, thirteen corpora, 275 works:
//
//	colliding titles     = 6
//	preprints            = 2
//	preprint-superseded  = 2   (a preprint whose title also appears under a
//	                            non-preprint DOI in the same corpus)
//
// The three collisions, one by one:
//
//	vitamind        pub:10.3390:supporting   + PRE:10.20944:supporting
//	hpvinfertility  pub:10.1016:refuting     + PRE:10.1101:refuting
//	vaccines        pub:10.1007:inconclusive + pub:(no DOI):inconclusive
//
// VERDICT: NOT REJECTED, BUT NOT WORTH BUILDING YET.
//
// Unlike the opinion gate and the title-override rule, both of which were
// measured and found actively harmful, this one is clean. It requires two
// conditions at once — normalized titles match AND one side carries a
// non-preprint DOI — so a preprint with no published counterpart is untouched.
// It reaches none of the six calibrated harm corpora: the vaccines collision
// is not a preprint case. In both real cases the two editions vote the same
// way, so no verdict would move; only the doubled weight would go.
//
// It is simply too small to justify code today: two works out of 275, neither
// changing an outcome. The ratio grows with corpus size, and today's limit
// measurement showed a limit of 100 yields 50 relevant works where 10 yields
// 6 — so this is worth recomputing after any limit change.
//
// ⚠️ THE MORE INTERESTING FINDING is the third collision. In `vaccines`, one
// of the two colliding works HAS NO DOI AT ALL, and dedupeVersions skips
// those outright (dedupe.go:79-80, "no DOI: never deduplicable"). That is the
// deduplicator's real blind spot, and it has nothing to do with preprints.
// Measuring how many DOI-less works exist across the corpora is the next
// question, not this one.
//
// WHEN TO REVISIT: after a study-limit increase, or when preprint-superseded
// reaches roughly ten.

import (
	"regexp"
	"strings"
	"testing"
)

// preprintPrefixes are DOI prefixes that publish preprints only.
var preprintPrefixes = []string{
	"10.1101",  // bioRxiv / medRxiv
	"10.21203", // Research Square
	"10.20944", // Preprints.org
	"10.31219", // OSF Preprints
	"10.48550", // arXiv
	"10.2139",  // SSRN
	"10.31235", // SocArXiv
	"10.5281",  // Zenodo — not strictly a preprint server, but self-deposit
}

// bracketSuffix and nonAlnum strip the punctuation and case differences that
// stop two renderings of one title from matching: "18-33" vs "18\u201333",
// trailing session markers like "[33B]", and capitalisation.
var (
	bracketSuffix = regexp.MustCompile(`\s*\[[^\]]*\]\s*$`)
	nonAlnum      = regexp.MustCompile(`[^a-z0-9]+`)
)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = bracketSuffix.ReplaceAllString(s, "")
	s = nonAlnum.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func doiPrefix(doi string) string {
	d := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(doi)), "https://doi.org/")
	if i := strings.IndexByte(d, '/'); i > 0 {
		return d[:i]
	}
	return ""
}

func isPreprintDOI(doi string) bool {
	p := doiPrefix(doi)
	for _, pp := range preprintPrefixes {
		if p == pp {
			return true
		}
	}
	return false
}

// TestTitleCollisionsAcrossDOIs reports, per corpus: how many works share a
// normalized title with at least one other work, how many of those collisions
// involve a preprint DOI, and how many works are preprints at all.
//
// The preprint column is the ceiling on what a preprint-supersession rule
// could remove. The gap between the two columns is what such a rule would
// still miss.
func TestTitleCollisionsAcrossDOIs(t *testing.T) {
	corpora := append([]string{}, allCorpora...)
	corpora = append(corpora, "hpvinfertility")

	totalWorks, totalColliding, totalPreprints, totalPreprintSuperseded := 0, 0, 0, 0

	t.Logf("%-14s %6s %6s %10s %12s", "corpus", "works", "collid", "preprints", "superseded")

	for _, name := range corpora {
		full, err := loadFullCorpus(name)
		if err != nil {
			t.Logf("%s: skipped (%v)", name, err)
			continue
		}

		byTitle := map[string][]int{}
		for i, w := range full.AllStudies {
			key := normalizeTitle(w.Title)
			if key == "" {
				continue
			}
			byTitle[key] = append(byTitle[key], i)
		}

		colliding, preprints, superseded := 0, 0, 0
		for _, w := range full.AllStudies {
			if isPreprintDOI(w.DOI) {
				preprints++
			}
		}

		for key, idxs := range byTitle {
			if len(idxs) < 2 {
				continue
			}
			colliding += len(idxs)

			hasPublished := false
			for _, i := range idxs {
				if !isPreprintDOI(full.AllStudies[i].DOI) {
					hasPublished = true
					break
				}
			}
			for _, i := range idxs {
				w := full.AllStudies[i]
				if hasPublished && isPreprintDOI(w.DOI) {
					superseded++
				}
			}

			title := key
			if len(title) > 60 {
				title = title[:60] + "..."
			}
			var detail []string
			for _, i := range idxs {
				w := full.AllStudies[i]
				tag := "pub"
				if isPreprintDOI(w.DOI) {
					tag = "PRE"
				}
				detail = append(detail, tag+":"+doiPrefix(w.DOI)+":"+w.Stance)
			}
			t.Logf("  %-12s COLLISION x%d  %s  [%s]",
				name, len(idxs), title, strings.Join(detail, " "))
		}

		totalWorks += len(full.AllStudies)
		totalColliding += colliding
		totalPreprints += preprints
		totalPreprintSuperseded += superseded

		t.Logf("%-14s %6d %6d %10d %12d",
			name, len(full.AllStudies), colliding, preprints, superseded)
	}

	t.Logf("")
	t.Logf("TOTALS: works=%d  colliding=%d  preprints=%d  preprint-superseded=%d",
		totalWorks, totalColliding, totalPreprints, totalPreprintSuperseded)
	t.Logf("preprint-superseded is the ceiling on a preprint rule: a preprint")
	t.Logf("whose title also appears under a non-preprint DOI in the same corpus.")
	t.Logf("colliding minus preprint-superseded is what such a rule would miss.")
}
