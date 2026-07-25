package scengine

// pico_impact_full_test.go re-runs the PICO gate measurement on UNTRUNCATED
// corpora, so the numbers stop being upper bounds and become real.
//
// pico_impact_test.go measures testdata/corpora/, whose abstracts were clipped
// at exactly 1500 runes by the CLI's serialization cap — 108 of 246 studies
// (43.9%). A token past character 1500 was invisible there, so every exclusion
// count it reports is an over-estimate of what the gate would really do.
//
// testdata/corpora_full/ is the same 12 queries re-exported through
// `consensus --full-abstracts --data-source live` on the heuristic stance path.
// The two exports were verified to agree on everything except abstract length:
// identical work sets (DOI-matched, zero drift), identical scores, verdicts,
// stance labels and counts in all 12 corpora. The ONLY variable between the two
// files is how much of each abstract survived serialization — which is exactly
// what makes the delta below attributable to truncation and nothing else.
//
// Shape note: corpora_full/*.json is the CLI's raw consensus output, so the
// analysis sits at the top level. corpora/*.json is a Corpova export that wraps
// the same object under {export, rows, response.result}.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fullCorpusDir holds the untruncated re-export.
const fullCorpusDir = "testdata/corpora_full"

// loadFullCorpus reads one untruncated export. Unlike loadCorpus it unmarshals
// the top-level object directly: there is no Corpova envelope around it.
func loadFullCorpus(name string) (corpusResult, error) {
	var res corpusResult
	raw, err := os.ReadFile(filepath.Join(fullCorpusDir, name+".json"))
	if err != nil {
		return corpusResult{}, err
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return corpusResult{}, err
	}
	return res, nil
}

func mustLoadFullCorpus(t *testing.T, name string) corpusResult {
	t.Helper()
	res, err := loadFullCorpus(name)
	if err != nil {
		t.Fatalf("loadFullCorpus(%q): %v", name, err)
	}
	return res
}

// benefitCorpora are the claims the current gate cannot split, because
// claimSides only matches claimHarmCues. These are the corpora where the
// widened splitter would newly start excluding works, so they are where the
// truncation correction matters most.
var benefitCorpora = []string{"saffron", "melatonin", "meditation", "omega3", "vitamind", "probiotics"}

// TestPICOImpactFull is the headline measurement: the same widened-splitter
// exclusion count computed on truncated vs untruncated abstracts. The truncated
// column is the upper bound previously reported; the full column is the real
// number. Log-only — this measures a proposed change, it does not assert one.
func TestPICOImpactFull(t *testing.T) {
	t.Logf("%-14s %5s | %-9s %-9s %-7s | %s",
		"corpus", "works", "trunc-drop", "full-drop", "delta", "tokens (iv / out)")

	for _, name := range allCorpora {
		trunc := mustLoadCorpus(t, name)
		full := mustLoadFullCorpus(t, name)

		if len(trunc.AllStudies) != len(full.AllStudies) {
			t.Errorf("%s: corpus size differs between exports (%d truncated vs %d full) — "+
				"the two files are no longer comparable",
				name, len(trunc.AllStudies), len(full.AllStudies))
			continue
		}

		iv, out := fixedPICOTokens(full.Claim)
		tg := runGate(trunc, iv, out)
		fg := runGate(full, iv, out)

		t.Logf("%-14s %5d | %-9d %-9d %+-7d | iv=%v out=%v",
			name, len(full.AllStudies), tg.dropped(), fg.dropped(),
			fg.dropped()-tg.dropped(), iv, out)
	}
}

// TestPICOImpactFullBenefitCorpora reports the six benefit-shaped corpora on
// their own, with the correction expressed as a share. These are the numbers
// that should drive the decision on whether to widen the verb recognizer: how
// much of yesterday's exclusion estimate was real, and how much was an artifact
// of reading a stump.
func TestPICOImpactFullBenefitCorpora(t *testing.T) {
	var sumTrunc, sumFull, sumWorks int

	for _, name := range benefitCorpora {
		trunc := mustLoadCorpus(t, name)
		full := mustLoadFullCorpus(t, name)

		iv, out := fixedPICOTokens(full.Claim)
		tg := runGate(trunc, iv, out)
		fg := runGate(full, iv, out)

		n := len(full.AllStudies)
		sumTrunc += tg.dropped()
		sumFull += fg.dropped()
		sumWorks += n

		recovered := tg.dropped() - fg.dropped()
		t.Logf("%-12s %2d works | upper bound %2d/%d (%.0f%%) -> real %2d/%d (%.0f%%) | %d work(s) recovered by the full text",
			name, n,
			tg.dropped(), n, pct(tg.dropped(), n),
			fg.dropped(), n, pct(fg.dropped(), n),
			recovered)
	}

	t.Logf("TOTAL benefit corpora: upper bound %d/%d (%.0f%%) -> real %d/%d (%.0f%%); "+
		"%d exclusions were truncation artifacts",
		sumTrunc, sumWorks, pct(sumTrunc, sumWorks),
		sumFull, sumWorks, pct(sumFull, sumWorks),
		sumTrunc-sumFull)
}

// pct is a small helper so the log lines stay readable.
func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

// TestPICOImpactFullMeditationControl answers the specific regression question.
//
// On truncated data the widened splitter excluded three meditation works, one
// of them — "Meditation practices for health: state of the research." — on an
// abstract cut at 1500 runes. That paper is a broad review that very plausibly
// discusses anxiety past the cut, so its exclusion could not be judged. With
// the full abstract the question is answerable, and the answer decides whether
// widening the gate costs the benefit-claim control a genuinely on-topic study.
func TestPICOImpactFullMeditationControl(t *testing.T) {
	const probe = "Meditation practices for health"

	full := mustLoadFullCorpus(t, "meditation")
	iv, out := fixedPICOTokens(full.Claim)
	fg := runGate(full, iv, out)

	t.Logf("meditation claim=%q -> iv=%v out=%v", full.Claim, iv, out)
	t.Logf("  widened splitter excludes %d of %d works with full abstracts",
		fg.dropped(), len(full.AllStudies))
	for _, title := range fg.droppedTitles {
		t.Logf("    - %s", title)
	}

	study, ok := findStudy(full, probe)
	if !ok {
		t.Logf("  probe %q not found in the full export", probe)
		return
	}

	passes := IsPICORelevant(study.Abstract, study.Title, iv, out)
	truncStudy, hadTrunc := findStudy(mustLoadCorpus(t, "meditation"), probe)
	truncPasses := hadTrunc && IsPICORelevant(truncStudy.Abstract, truncStudy.Title, iv, out)

	t.Logf("  probe: %q", study.Title)
	t.Logf("    abstract: %d runes truncated -> %d runes full",
		len([]rune(truncStudy.Abstract)), len([]rune(study.Abstract)))
	t.Logf("    passes gate on truncated text: %v", truncPasses)
	t.Logf("    passes gate on full text     : %v", passes)

	// Report which side of the claim the full text supplies that the stump did
	// not — that is the concrete mechanism, not just the verdict.
	fullText := strings.ToLower(study.Abstract + " " + study.Title)
	truncText := strings.ToLower(truncStudy.Abstract + " " + truncStudy.Title)
	t.Logf("    intervention token present: truncated=%v full=%v",
		containsAnyToken(truncText, iv), containsAnyToken(fullText, iv))
	t.Logf("    outcome token present     : truncated=%v full=%v",
		containsAnyToken(truncText, out), containsAnyToken(fullText, out))

	if passes && !truncPasses {
		t.Logf("  VERDICT: truncation alone caused this exclusion. Widening the gate does NOT " +
			"cost the meditation control this study.")
	} else if !passes {
		t.Logf("  VERDICT: excluded even on the full abstract — the paper never names both " +
			"sides of the claim. Read the title before treating this as a regression.")
	}
}

// TestPICOImpactFullBaselineIntact guards the premise of every number above:
// the untruncated re-export must describe the same analysis as the archive,
// differing only in abstract length. If a score or a stance label ever diverges,
// the two files stop being comparable and the truncated-vs-full delta stops
// being attributable to truncation.
func TestPICOImpactFullBaselineIntact(t *testing.T) {
	for _, name := range allCorpora {
		trunc := mustLoadCorpus(t, name)
		full := mustLoadFullCorpus(t, name)

		if !approxEq(trunc.ConsensusScore, full.ConsensusScore) {
			t.Errorf("%s: consensus_score differs — archived %v, re-export %v",
				name, trunc.ConsensusScore, full.ConsensusScore)
		}
		if trunc.StudyCount != full.StudyCount {
			t.Errorf("%s: study_count differs — archived %d, re-export %d",
				name, trunc.StudyCount, full.StudyCount)
		}
		if trunc.Supporting != full.Supporting || trunc.Refuting != full.Refuting ||
			trunc.Mixed != full.Mixed || trunc.Inconclusive != full.Inconclusive {
			t.Errorf("%s: stance counts differ — archived %d/%d/%d/%d, re-export %d/%d/%d/%d",
				name, trunc.Supporting, trunc.Refuting, trunc.Mixed, trunc.Inconclusive,
				full.Supporting, full.Refuting, full.Mixed, full.Inconclusive)
		}
		if trunc.Verdict != full.Verdict {
			t.Errorf("%s: verdict differs — archived %q, re-export %q",
				name, trunc.Verdict, full.Verdict)
		}

		// Every archived abstract must be a prefix of its untruncated twin.
		// This is the direct proof that the archive is the same text, cut.
		byTitle := make(map[string]string, len(full.AllStudies))
		for _, s := range full.AllStudies {
			byTitle[s.Title] = s.Abstract
		}
		for _, s := range trunc.AllStudies {
			fullAbs, ok := byTitle[s.Title]
			if !ok {
				t.Errorf("%s: work missing from the re-export: %s", name, s.Title)
				continue
			}
			if !strings.HasPrefix(fullAbs, s.Abstract) {
				t.Errorf("%s: archived abstract is not a prefix of the full one (%d vs %d runes): %s",
					name, len([]rune(s.Abstract)), len([]rune(fullAbs)), s.Title)
			}
		}
	}
}

// TestPICOImpactFullNoTruncationRemains is the acceptance check on the new
// fixture: not a single abstract may sit exactly on the old cap, and the corpus
// must contain abstracts past it — otherwise --full-abstracts did not take
// effect and every measurement in this file is meaningless.
func TestPICOImpactFullNoTruncationRemains(t *testing.T) {
	atCap, over, total := 0, 0, 0
	for _, name := range allCorpora {
		full := mustLoadFullCorpus(t, name)
		for _, s := range full.AllStudies {
			n := len([]rune(s.Abstract))
			total++
			if n == 1500 {
				atCap++
				t.Errorf("%s: abstract still sits exactly on the 1500-rune cap: %s", name, s.Title)
			}
			if n > 1500 {
				over++
			}
		}
	}
	if over == 0 {
		t.Errorf("no abstract exceeds 1500 runes in the whole re-export — --full-abstracts did not take effect")
	}
	t.Logf("re-export: %d studies, %d exactly at the old cap, %d past it", total, atCap, over)
}
