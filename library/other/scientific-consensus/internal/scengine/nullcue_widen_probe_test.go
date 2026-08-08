package scengine

// nullcue_widen_probe_test.go MEASURES a one-word candidate change before it
// is made. Log-only, no assertions — it exists to answer "is this safe to
// ship", not to pin behavior.
//
// WHY this candidate exists. A live keyless run of "homeopathy treats
// allergies" at limit 10 scores 0.99 evidence-supports. Two of the ten works
// vote the wrong way and they are the heaviest:
//
//	weight 28.88  RCT, 35 citations, stance=supporting, confidence 0.90
//	  "No beneficial effect of isopathic prophylactic treatment for birch
//	   pollen allergy during a low-pollen season"
//	weight 11.27  cross-sectional, 33 citations, stance=supporting
//	  "Selling falsehoods? A cross-sectional study of Canadian naturopathy,
//	   homeopathy, chiropractic and acupuncture"
//
// Total weight over the ten works is 120.06, so those two carry a third of the
// mass on the wrong side. The second is a different problem and out of scope
// here. The first is one word: nullCues matches "no benefit" but the
// alternative is the literal "benefit", so "No beneficial effect" does not
// match — verified directly against the compiled regex.
//
// The candidate is to widen that one alternative to "benefi\w*" inside the
// "no (significant )?(...)" group only. The group still requires a preceding
// "no", so "Beneficial effect of vitamin D on bone density" stays unmatched.
//
// THE RISK this probe measures. nullCues also feeds the claim-agnostic
// baseline that every benefit-asserting claim uses (stance.go:179) and the
// harm-claim negative count (stance.go:279). A newly firing cue can therefore
// move works in corpora that have nothing to do with homeopathy. The probe
// swaps the package-level nullCues var and re-runs the real ClassifyStance, so
// what it reports is the engine's own answer under the widened regex, not a
// re-implementation of it.

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// baselineNullCues is nullCues as it stood BEFORE the widening: the single
// alternative is the literal "benefit". It is pinned here rather than read
// from stance.go so this probe keeps measuring the same delta after the change
// ships — otherwise the comparison would quietly become a no-op against
// itself, which is exactly the failure mode a measuring instrument must not
// have.
var baselineNullCues = regexp.MustCompile(`(?i)\b(no (significant )?(association|effect|difference|benefit|evidence|impact|correlation)|not (significant\w*|associated|effective)|did not (significantly )?(improv|increas|reduc|affect|differ|change)|ineffective|no statistically significant|failed to|without (a )?(significant )?(effect|benefit)|null (result|effect|finding))`)

// widenedNullCues is the shipped regex — baselineNullCues with "benefit"
// widened to "benefi\w*" inside the "no (significant )?(...)" group only.
var widenedNullCues = nullCues

// widenProbeCorpora lists every corpus in testdata/corpora_full. It is read
// from disk rather than reusing allCorpora because corpora_full carries three
// exports (hpvinfertility, vitaminc) that allCorpora does not, and a blind
// spot in a probe about blind spots is worth avoiding.
func widenProbeCorpora(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(fullCorpusDir)
	if err != nil {
		t.Fatalf("read %s: %v", fullCorpusDir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names
}

// widenProbeWeight is score.go:82's formula, reproduced here so the probe can
// report how much mass each affected work carries without exporting anything.
func widenProbeWeight(design string, citedBy int, conf float64) float64 {
	return TierWeight(Design(design)) * (1 + math.Log1p(float64(max(citedBy, 0)))) * clamp01(conf, 0.3)
}

// widenProbeNewHits returns the spans the widened regex matches that the
// current one does not, as the matched text itself.
func widenProbeNewHits(hay string) []string {
	current := map[string]bool{}
	for _, loc := range baselineNullCues.FindAllStringIndex(hay, -1) {
		current[itoa(loc[0])+":"+itoa(loc[1])] = true
	}
	var added []string
	for _, loc := range widenedNullCues.FindAllStringIndex(hay, -1) {
		if !current[itoa(loc[0])+":"+itoa(loc[1])] {
			added = append(added, hay[loc[0]:loc[1]])
		}
	}
	return added
}

// TestNullCueWidenProbe reports, per corpus, every work whose text the widened
// regex newly matches, what the engine actually returns for that work under
// both regexes, and the weight at stake — followed by the corpus verdict and
// score as they stand today, so a reader can see whether any verdict is close
// enough to be at risk.
func TestNullCueWidenProbe(t *testing.T) {
	// Instrument self-check. A probe that reports "nothing moves" is only
	// worth reading if it can still see the case that motivated it, so the
	// homeopathy RCT title is run through the same comparison first. This one
	// check does assert: a silent zero from a broken comparison would be
	// indistinguishable from a safe change.
	const motivatingTitle = "No beneficial effect of isopathic prophylactic treatment for birch " +
		"pollen allergy during a low-pollen season"
	probeHay := strings.ToLower(normalizeText(motivatingTitle))
	if baselineNullCues.MatchString(probeHay) {
		t.Fatalf("instrument: baseline nullCues already matches %q — the premise of this probe is gone", motivatingTitle)
	}
	if got := widenProbeNewHits(probeHay); len(got) == 0 {
		t.Fatalf("instrument: widened nullCues does not match %q — the comparison is broken", motivatingTitle)
	} else {
		t.Logf("instrument OK: widened regex newly matches %q on the motivating title", got)
	}

	names := widenProbeCorpora(t)

	var touched, untouched []string
	totalFlips := 0

	for _, name := range names {
		full := mustLoadFullCorpus(t, name)

		type affected struct {
			s           corpusStudy
			hits        []string
			wasStance   Stance
			wasConf     float64
			nowStance   Stance
			nowConf     float64
			weightWas   float64
			weightWould float64
		}
		var rows []affected

		for _, s := range full.AllStudies {
			hay := strings.ToLower(normalizeText(s.Title + ". " + s.Abstract))
			hits := widenProbeNewHits(hay)
			if len(hits) == 0 {
				continue
			}
			// Swap the live var so the engine itself answers the
			// counterfactual instead of a re-implementation of it;
			// restore immediately.
			saved := nullCues
			nullCues = baselineNullCues
			wasStance, wasConf := ClassifyStance(s.Title, s.Abstract, full.Claim)
			nullCues = saved

			nowStance, nowConf := ClassifyStance(s.Title, s.Abstract, full.Claim)

			rows = append(rows, affected{
				s: s, hits: hits,
				wasStance: wasStance, wasConf: wasConf,
				nowStance: nowStance, nowConf: nowConf,
				weightWas:   widenProbeWeight(s.Design, s.CitedByCount, s.StanceConfidence),
				weightWould: widenProbeWeight(s.Design, s.CitedByCount, nowConf),
			})
		}

		t.Logf("")
		t.Logf("=== %s  (%d works)  claim: %q", name, len(full.AllStudies), full.Claim)
		t.Logf("    today: verdict=%s score=%.4f confidence=%.2f  sup=%d ref=%d mix=%d inc=%d",
			full.Verdict, full.ConsensusScore, full.Confidence,
			full.Supporting, full.Refuting, full.Mixed, full.Inconclusive)

		if len(rows) == 0 {
			untouched = append(untouched, name)
			t.Logf("    no work gains a cue — UNTOUCHED")
			continue
		}
		touched = append(touched, name)

		flips := 0
		for _, r := range rows {
			if r.wasStance != r.nowStance {
				flips++
				totalFlips++
			}
			title := r.s.Title
			if len(title) > 100 {
				title = title[:100] + "..."
			}
			t.Logf("    - %q", title)
			t.Logf("        design=%s cited_by=%d  fixture stance=%s conf=%.2f",
				r.s.Design, r.s.CitedByCount, r.s.Stance, r.s.StanceConfidence)
			t.Logf("        engine now:  %s conf=%.2f   ->  widened: %s conf=%.2f   %s",
				r.wasStance, r.wasConf, r.nowStance, r.nowConf,
				map[bool]string{true: "FLIPS", false: "no flip"}[r.wasStance != r.nowStance])
			t.Logf("        weight (fixture conf) %.2f | weight (widened conf) %.2f",
				r.weightWas, r.weightWould)
			t.Logf("        newly matched: %q", r.hits)
		}
		t.Logf("    affected works: %d, of which stance flips: %d", len(rows), flips)
	}

	t.Logf("")
	t.Logf("SUMMARY: %d corpora measured, %d touched, %d untouched, %d works flip stance",
		len(names), len(touched), len(untouched), totalFlips)
	t.Logf("  touched:   %v", touched)
	t.Logf("  untouched: %v", untouched)
}
