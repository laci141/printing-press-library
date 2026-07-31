package scengine

// benefit_gate_test.go pins the benefit-branch stance gates (the "N8b" change).
//
// Background: ClassifyStance routes HARM-asserting claims through
// classifyAgainstHarmClaim, which applies four gates (dedup / framing /
// negation / pairing) before a positive cue is allowed to count. BENEFIT- and
// ambiguous-shaped claims fall through to the claim-agnostic baseline, which
// counted cues with no gates at all. The measured consequence was that works
// reporting a NULL result scored as `supporting`, because the verb naming the
// intervention ("increasing LCn3 intake", "for preventing the common cold") is
// itself a supportCues match.
//
// This file asserts the three-part fix and, just as importantly, its blast
// radius: the six calibrated harm corpora and the meditation benefit control
// must not move at all.
//
// Fixtures: testdata/corpora_full/ (fullCorpusDir) — 13 archived `consensus`
// runs, 295 works, with UNTRUNCATED abstracts. The sibling testdata/corpora/
// must not be used here: it holds 12 corpora serialized through the 1500-rune
// clipAbstract cap, so cue counts computed from it would not match what the
// engine actually scored. loadFullCorpus, mustLoadFullCorpus and normalizeDOI
// already exist in this package (pico_impact_full_test.go, pico_excluded_test.go)
// and are reused rather than redeclared.

import (
	"strings"
	"testing"
)

// benefitGateCorpora is every corpus this suite sweeps, including vitaminc,
// which exists only in corpora_full.
var benefitGateCorpora = []string{
	"cellphones", "coffee", "meditation", "melatonin", "omega3", "probiotics",
	"redmeat_run1", "redmeat_run2", "saffron", "sweeteners", "vaccines",
	"vitaminc", "vitamind",
}

// benefitGateTotalWorks is the number of works across all 13 corpora. Every
// sweeping test in this file counts what it actually examined and fails if the
// count is short: `go test -run` silently passes when zero tests match, and a
// table test that quietly stops finding its fixtures looks exactly like
// success. The number is an assertion, not documentation.
const benefitGateTotalWorks = 295

// harmControlCorpora are the six harm-claim corpora. They route through
// classifyAgainstHarmClaim, which this change does not touch — except that the
// nullCues lexicon is shared with the harm branch (see the neg count in
// classifyAgainstHarmClaim), so "unchanged" has to be measured, not assumed.
var harmControlCorpora = []string{
	"cellphones", "coffee", "redmeat_run1", "redmeat_run2", "sweeteners", "vaccines",
}

// nullResultVocabulary is the phrasing a work uses when it reports a null
// finding. It exists for one assertion: a work carrying this vocabulary must
// never be pushed into `inconclusive`, because that would be the fix eating
// exactly the dissent it was built to surface.
var nullResultVocabulary = []string{
	"no significant difference", "did not reduce", "no benefit", "failed to meet",
	"null result", "no effect on", "did not differ", "little or no effect",
	"little or no difference", "no difference", "not confirmed", "little evidence",
	"failed to show", "did not improve", "inconsistent", "no association",
	"not associated",
}

// mustLoadBenefitCorpus is mustLoadFullCorpus plus the two emptiness guards
// this suite needs: a corpus that loads as zero studies, or with an empty
// claim, turns every sweep below into a no-op that reports success.
func mustLoadBenefitCorpus(t *testing.T, name string) corpusResult {
	t.Helper()
	res := mustLoadFullCorpus(t, name)
	if len(res.AllStudies) == 0 {
		t.Fatalf("corpus %q loaded with zero studies — fixture or loader is broken", name)
	}
	if strings.TrimSpace(res.Claim) == "" {
		t.Fatalf("corpus %q has an empty claim — stance classification would take the ambiguous path for the wrong reason", name)
	}
	return res
}

// findByDOI locates a work by normalized DOI. Identification is DOI-based
// throughout this file: the vitaminc corpus contains five works sharing one
// title, which a title-substring lookup cannot tell apart.
func findByDOI(res corpusResult, doi string) (corpusStudy, bool) {
	want := normalizeDOI(doi)
	for _, s := range res.AllStudies {
		if normalizeDOI(s.DOI) == want {
			return s, true
		}
	}
	return corpusStudy{}, false
}

// classifyCorpus re-runs the live classifier over every work in a corpus and
// returns the stances in corpus order alongside ScoredWork values ready for
// Consensus.
func classifyCorpus(res corpusResult) ([]Stance, []ScoredWork) {
	stances := make([]Stance, 0, len(res.AllStudies))
	scored := make([]ScoredWork, 0, len(res.AllStudies))
	for _, s := range res.AllStudies {
		st, conf := ClassifyStance(s.Title, s.Abstract, res.Claim)
		stances = append(stances, st)
		scored = append(scored, ScoredWork{
			Stance: st, StanceConf: conf,
			Design: Design(s.Design), CitedBy: s.CitedByCount,
		})
	}
	return stances, scored
}

// TestBenefitGateInstrument is the pin. It does not exercise the change at all
// — it establishes that the fixtures are complete and that Consensus
// reproduces the archived score and verdict from the archived stances. Every
// other test in this file measures a DELTA against those archived stances, so
// if this one is wrong the deltas are meaningless.
func TestBenefitGateInstrument(t *testing.T) {
	works := 0
	for _, name := range benefitGateCorpora {
		res := mustLoadBenefitCorpus(t, name)
		works += len(res.AllStudies)

		scored := make([]ScoredWork, 0, len(res.AllStudies))
		for _, s := range res.AllStudies {
			scored = append(scored, ScoredWork{
				Stance: Stance(s.Stance), StanceConf: s.StanceConfidence,
				Design: Design(s.Design), CitedBy: s.CitedByCount,
			})
		}
		got := Consensus(scored)

		if !approxEq(got.ConsensusScore, res.ConsensusScore) {
			t.Errorf("%s: Consensus over archived stances = %v, archive records %v",
				name, got.ConsensusScore, res.ConsensusScore)
		}
		if string(got.Verdict) != res.Verdict {
			t.Errorf("%s: verdict = %q, archive records %q", name, got.Verdict, res.Verdict)
		}
		if got.StudyCount != res.StudyCount {
			t.Errorf("%s: study_count = %d, archive records %d", name, got.StudyCount, res.StudyCount)
		}
	}

	if works != benefitGateTotalWorks {
		t.Fatalf("swept %d works, expected %d — a corpus is missing or truncated",
			works, benefitGateTotalWorks)
	}
	t.Logf("pin: %d works across %d corpora, Consensus reproduces every archived score and verdict",
		works, len(benefitGateCorpora))
}

// TestBenefitGateControlCorporaUnmoved is criterion (a): the six calibrated
// harm-claim corpora must not move by a single work.
//
// This is not a formality. The fix extends the nullCues lexicon, and
// classifyAgainstHarmClaim counts nullCues on its negative side too, so a
// careless addition reaches straight into the harm corpora. An earlier
// candidate lexicon that also added "unclear" moved two works here
// (10.1056/nejmoa1112010 in coffee and 10.1111/j.1365-2214.2006.00655.x in
// vaccines) from inconclusive to refuting/0.90 — on a word that expresses
// uncertainty, not a null finding. "unclear" is deliberately NOT in the
// shipped lexicon; this test is what keeps it out.
func TestBenefitGateControlCorporaUnmoved(t *testing.T) {
	checked := 0
	for _, name := range harmControlCorpora {
		res := mustLoadBenefitCorpus(t, name)
		stances, _ := classifyCorpus(res)
		for i, s := range res.AllStudies {
			checked++
			if string(stances[i]) != s.Stance {
				t.Errorf("%s %s: stance moved %q -> %q (control corpus must not move)\n    %s",
					name, normalizeDOI(s.DOI), s.Stance, stances[i], s.Title)
			}
		}
	}
	if checked == 0 {
		t.Fatalf("checked zero works — the control sweep did not run")
	}
	t.Logf("control: %d works across %d harm corpora, zero movement", checked, len(harmControlCorpora))
}

// TestBenefitGateMeditationControlHolds is criterion (a2). meditation is the
// BENEFIT-claim control: corpus_regression_test.go's TestCorpusBaseline names
// it as the case the engine must keep supporting, and records "a change here
// means a real regression, not progress".
//
// That test asserts against the ARCHIVED json, so it cannot catch a stance.go
// change on its own. This test closes that hole: it re-runs the live
// classifier over the corpus and re-scores it, so a benefit-branch change that
// disturbs the control fails here.
//
// The measured stakes: a blunt variant that dropped every intent verb
// unconditionally scored 5/5 on the target works below — and drove meditation
// from +0.91/evidence-supports to -0.57/evidence-refutes. That variant was
// rejected because of this control.
func TestBenefitGateMeditationControlHolds(t *testing.T) {
	res := mustLoadBenefitCorpus(t, "meditation")
	_, scored := classifyCorpus(res)
	got := Consensus(scored)

	if !approxEq(got.ConsensusScore, 0.91) {
		t.Errorf("meditation consensus_score = %v, control pins 0.91", got.ConsensusScore)
	}
	if got.Verdict != VerdictSupports {
		t.Errorf("meditation verdict = %q, control pins %q", got.Verdict, VerdictSupports)
	}
	for _, c := range []struct {
		label     string
		got, want int
	}{
		{"supporting", got.Supporting, 5},
		{"refuting", got.Refuting, 0},
		{"mixed", got.Mixed, 1},
		{"inconclusive", got.Inconclusive, 4},
		{"study_count", got.StudyCount, 10},
	} {
		if c.got != c.want {
			t.Errorf("meditation %s = %d, control pins %d", c.label, c.got, c.want)
		}
	}
}

// TestBenefitGateTargetWorks is criterion (b). Each target is identified by
// DOI and must leave `supporting` for `refuting` or `mixed` — NOT for
// `inconclusive`, which would mean the classifier stopped seeing the work at
// all rather than reading it correctly.
//
// The two asserted targets are the ones the shipped gates actually reach. The
// three Hemilä-Chalker Cochrane editions are recorded, not asserted: their
// support cues come from `prevent*` in the TITLE ("Vitamin C for preventing
// and treating the common cold"), where the claim's own outcome side sits in
// scope, so no scope-based gate can fire on them. Separating an intent
// ("for preventing X") from a finding ("prevented X") needs grammatical mood,
// which is a different change. They are logged so the gap stays visible,
// following TestKnownMisclassifications' pattern.
func TestBenefitGateTargetWorks(t *testing.T) {
	asserted := []struct {
		corpus string
		doi    string
		want   Stance
		why    string
	}{
		{
			corpus: "omega3", doi: "10.1002/14651858.cd003177.pub3", want: StanceMixed,
			why: "Cochrane, 86 RCTs: \"little or no effect of increasing LCn3 on all-cause mortality\"",
		},
		{
			corpus: "vitamind", doi: "10.1111/irv.12615", want: StanceRefuting,
			why: "RCT whose abstract states the supplementation \"did not reduce\" infections",
		},
	}

	for _, tc := range asserted {
		res := mustLoadBenefitCorpus(t, tc.corpus)
		study, ok := findByDOI(res, tc.doi)
		if !ok {
			t.Fatalf("%s: target DOI %s is not in the corpus — this table test would silently no-op",
				tc.corpus, tc.doi)
		}
		if study.Stance != string(StanceSupporting) {
			t.Fatalf("%s %s: archive records %q, but this test is written against a %q baseline",
				tc.corpus, tc.doi, study.Stance, StanceSupporting)
		}

		got, conf := ClassifyStance(study.Title, study.Abstract, res.Claim)
		if got != tc.want {
			t.Errorf("%s %s: stance = %q (conf %.3f), want %q\n    %s\n    %s",
				tc.corpus, tc.doi, got, conf, tc.want, study.Title, tc.why)
		}
	}

	// Recorded, not asserted — see the doc comment.
	recorded := []struct{ corpus, doi string }{
		{"vitaminc", "10.1002/14651858.cd000980.pub4"},
		{"vitaminc", "10.1002/14651858.cd000980.pub3"},
		{"vitaminc", "10.1002/14651858.cd000980.pub2"},
	}
	for _, rc := range recorded {
		res := mustLoadBenefitCorpus(t, rc.corpus)
		study, ok := findByDOI(res, rc.doi)
		if !ok {
			t.Fatalf("%s: recorded DOI %s is not in the corpus — the record has gone stale",
				rc.corpus, rc.doi)
		}
		got, conf := ClassifyStance(study.Title, study.Abstract, res.Claim)
		note := "still supporting — support cues are prevent* in the title; needs mood, not scope"
		if got != StanceSupporting {
			note = "MOVED off supporting — re-check whether this can be promoted to an assertion"
		}
		t.Logf("%-9s %s -> %s (conf %.3f)  %s", rc.corpus, rc.doi, got, conf, note)
	}
}

// TestBenefitGateNoInconclusiveDrain is criterion (c). Removing positive cues
// is only correct when what is removed was not evidence; when a work loses its
// last cue and lands in `inconclusive`, the classifier has gone quiet about it.
//
// Two assertions, and the second is the load-bearing one:
//
//  1. the set of supporting -> inconclusive moves is exactly the works named
//     below, so an unnoticed widening of the gates shows up here; and
//  2. NONE of them carries null-result vocabulary. A work that says
//     "no significant difference" and ends up inconclusive is dissent the fix
//     swallowed instead of surfaced — the precise failure mode that ruled out
//     the more aggressive variants (one of which drained 43 works).
//
// The set grew from five to nineteen when the outcome-scope gate was extended
// from increas* to the measured set of intent verbs. That growth is the point
// of assertion 1 working, not a reason to stop trusting it: the alarm fired,
// the fourteen new works were read, and assertion 2 still holds for every one
// of them. What they have in common is that they are off-topic rather than
// dissenting — vitamin D and vitamin A papers sitting in the vitamin C corpus,
// and two ESC practice guidelines in the omega-3 one. A guideline document is
// not evidence for or against the claim, and going quiet about it is the right
// answer.
//
// That also names the real defect, which this change does not fix: relevance
// belongs to the PICO gate, and here the stance layer is doing its work. The
// user sees "this study reached no conclusion" where the truth is "this study
// is about something else". Moving it to the right layer is a separate change.
func TestBenefitGateNoInconclusiveDrain(t *testing.T) {
	wantDrained := map[string]string{
		// Recorded by the benefit-gate change (increas* only).
		"10.1016/j.cmet.2016.04.009":       "melatonin",
		"10.1111/j.1753-4887.2010.00287.x": "omega3",
		"10.3390/ijms18122645":             "omega3",
		"10.1105/tpc.111.095232":           "vitaminc",
		"10.3390/antiox8110519":            "vitaminc",

		// Added when the gate was extended to the full intent-verb set.
		"10.1038/emm.2014.121":      "melatonin",
		"10.1093/eurheartj/ehz486":  "omega3",
		"10.1093/eurheartj/ehz455":  "omega3",
		"10.3390/nu6020799":         "omega3",
		"10.1038/srep11276":         "omega3",
		"10.1074/jbc.m201151200":    "vitaminc",
		"10.3390/nu9111211":         "vitaminc",
		"10.1017/s0950268806007175": "vitaminc",
		"10.3390/nu12040988":        "vitaminc",
		"10.1007/s12291-013-0375-3": "vitaminc",
		"10.3390/nu5010111":         "vitaminc",
		"10.1172/jci29449":          "vitaminc",
		"10.1016/j.cct.2011.09.009": "vitamind",
		"10.1542/peds.2011-3029":    "vitamind",
	}

	checked := 0
	gotDrained := map[string]string{}
	for _, name := range benefitGateCorpora {
		res := mustLoadBenefitCorpus(t, name)
		stances, _ := classifyCorpus(res)
		for i, s := range res.AllStudies {
			checked++
			if s.Stance != string(StanceSupporting) || stances[i] != StanceInconclusive {
				continue
			}
			doi := normalizeDOI(s.DOI)
			gotDrained[doi] = name

			text := strings.ToLower(s.Title + " " + s.Abstract)
			for _, phrase := range nullResultVocabulary {
				if strings.Contains(text, phrase) {
					t.Errorf("%s %s: supporting -> inconclusive, but the work reports a null result (%q) — this is swallowed dissent\n    %s",
						name, doi, phrase, s.Title)
					break
				}
			}
		}
	}

	if checked != benefitGateTotalWorks {
		t.Fatalf("checked %d works, expected %d — the sweep did not cover every corpus",
			checked, benefitGateTotalWorks)
	}

	for doi, corpus := range wantDrained {
		if _, ok := gotDrained[doi]; !ok {
			t.Errorf("%s %s: expected supporting -> inconclusive, but it did not move that way", corpus, doi)
		}
	}
	for doi, corpus := range gotDrained {
		if _, ok := wantDrained[doi]; !ok {
			t.Errorf("%s %s: NEW supporting -> inconclusive move, not in the recorded set — the gates widened", corpus, doi)
		}
	}
	if len(gotDrained) != len(wantDrained) {
		t.Errorf("supporting -> inconclusive count = %d, want %d", len(gotDrained), len(wantDrained))
	}
}
