package cli

// lexical_gate_variants_test.go is the regression suite for the V6 lexical
// relevance gate (relevantToClaim, scwork.go). It began as a measurement of
// seven candidate settings; six were rejected and their code went with them.
// What remains asserts, and is meant to FAIL if the gate regresses.
//
//	V0  1 distinct claim token in Title+Topic              <- the pre-V6 gate
//	V4  2 distinct claim tokens in Abstract+Title+Topic    <- V6 without the guard
//	V6  V4, but 1 token when the work carries no abstract  <- production
//
// Each acceptance criterion below calls the REAL relevantToClaim, not the local
// harness. The harness appears only where production cannot express the
// comparison itself: V0 for "did the control corpora move", V4 for "what is the
// no-abstract guard allowed to touch".
//
// TestLexicalHarnessMatchesProduction pins the harness at the V6 setting to the
// real relevantToClaim over all 295 archived studies. Without that pin the two
// delta tests would describe a harness rather than the gate, so if it fails,
// treat every other failure in this file as unexplained until it is green.
//
// What the archive can and cannot show. The corpora were exported AFTER the
// lexical gate ran (consensus.go filters before the PICO gate), so works the
// pre-V6 gate already removed are absent. This suite can therefore measure V6's
// TIGHTENING and is structurally unable to measure its WIDENING; the widening
// measurement needs a raw pre-gate result set, which is not committed.

// V7, CLIPPED ABSTRACT — MEASURED AND REJECTED
//
// The premise was that the gate reads the FULL abstract while the scorer reads
// a 1500-rune clip, so a claim stem sitting in the tail — publisher
// boilerplate, funding statements, copyright lines — could keep a work whose
// body never discusses the claim. V7 was V6 over clipAbstract's output.
//
// Measured over all 295 archived studies: 127 abstracts run past 1500 runes,
// and clipping flips the gate on 10 of them. Every one of those ten loses a
// stem that sits in ORDINARY SCIENTIFIC PROSE, not boilerplate:
//
//	omega3 10.1161/cir.0000000000000709  "cardi" in "improving atherosclerotic
//	                                     cardiovascular disease risk"
//	vaccines 10.1542/peds.113.5.e472     "vacci" in "immunizations with the
//	                                     measles-mumps-rubella vaccine"
//	coffee 10.7326/m16-2472              "heart" in "deaths due to heart
//	                                     disease, cancer, respiratory disease"
//
// Nine of the ten are findings the corpus exists to weigh. V7 would drop them.
// The clip is a display budget, and enforcing it inside a relevance decision
// trades recall for nothing — do not reintroduce it.
//
// The measurement did surface one real defect, of a different class. In
// vitaminc 10.1074/jbc.m201151200 the stem "commo" (from "common cold")
// matched "one of the most COMMOn oxidized adducts" — a 5-character stem hits
// any occurrence of "common". Clipping would have masked that case by
// accident, not fixed it. Stem-truncation false positives are their own
// problem and want their own change.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

const lexCorpusDir = "../scengine/testdata/corpora_full"

// lexTotalStudies is the size of the archive every count below was calibrated
// against. Asserted rather than logged: a corpus file that silently stopped
// parsing would otherwise leave these tests passing over fewer works than they
// claim to check.
const lexTotalStudies = 295

// lexCorpora is the archived set. vitaminc is the 13th, archived specifically
// because the defect under repair was never in the test set: the vitamin K
// ferroptosis paper scored REFUTING at 0.90 confidence under a "vitamin C
// prevents the common cold" run that nobody had exported.
//
// Deliberately NOT added to scengine's allCorpora (corpus_regression_test.go):
// those consumers call mustLoadCorpus, which reads testdata/corpora/ — the
// truncated Corpova export — where no vitaminc.json exists. Adding the name
// there would fail committed regression tests on a missing file. This list
// reads corpora_full/.
var lexCorpora = []string{
	"cellphones", "coffee", "meditation", "melatonin", "omega3", "probiotics",
	"redmeat_run1", "redmeat_run2", "saffron", "sweeteners", "vaccines", "vitamind",
	"vitaminc",
}

// lexHarmCorpora is the control group: the gate and the calibrated stance path
// already run on these, and vaccines above all carries results tuned against
// them. Movement here is a regression, not an improvement.
var lexHarmCorpora = map[string]bool{
	"cellphones": true, "coffee": true, "redmeat_run1": true,
	"redmeat_run2": true, "sweeteners": true, "vaccines": true,
}

// lexApexDesigns are the tiers where a wrong exclusion costs most: the
// consensus score is design-weighted, so dropping a meta-analysis or an RCT
// removes more than one work's share of the evidence.
var lexApexDesigns = map[string]bool{
	"meta-analysis": true, "systematic-review": true,
	"randomized-controlled-trial": true,
}

// TOPIC DATA, AND THE FALSE FAILURE IT CAN CAUSE
//
// The archived exports carry no per-study topic (workBrief drops it), so every
// Topic below is empty and these runs exercise the Abstract+Title path only.
// Production also matches against scWork.Topic, which is populated at gate time
// (scwork.go fills it from primary_topic).
//
// Running without topic data is the STRICT direction, and that is why it is the
// committed configuration: adding text to the haystack is monotone toward KEEP,
// so a work that survives here cannot start failing once a topic is present.
// Every KEEP assertion in this file is therefore stronger than production, and
// every "0 works dropped" result is an upper bound on the drops with topics.
//
// The converse is the trap. A work whose SECOND stem exists only in its topic is
// dropped here and kept in production. The documented example is the omega3
// meta-analysis "Effect of omega-3 fatty acids supplementation on endothelial
// function", which draws `healt` from primary_topic rather than from its title.
// (Measured here it survives anyway, because it has no abstract and the guard
// then asks it for one token, which `omega` supplies — but a work with the same
// topic dependency AND an abstract would not be so lucky.)
//
// So: IF TestV6ControlCorporaUnmoved OR TestV6StanceBalance FAILS ON A WORK
// WHOSE SECOND STEM WOULD COME FROM ITS TOPIC, THAT IS A FALSE FAILURE. The fix
// is to re-measure with the fetched-topic sidecar, NOT to weaken the gate. Do
// not lower relevanceMinTokens, do not widen the guard, and do not delete the
// offending corpus entry on the strength of a run that never had the topic
// field in the first place.

type lexStudy struct {
	Title    string `json:"title"`
	DOI      string `json:"doi"`
	Abstract string `json:"abstract"`
	Design   string `json:"design"`
	// Topic is absent from every archived export; see the note above.
	Topic string `json:"topic"`
	// Stance is the archived classification. A relevance gate has no business
	// knowing it, which is exactly why it is the right instrument for auditing
	// one: a gate that removes dissent faster than agreement is manufacturing
	// consensus rather than filtering noise.
	Stance string `json:"stance"`
}

type lexCorpus struct {
	Claim      string     `json:"claim"`
	AllStudies []lexStudy `json:"all_studies"`
}

// lexVariant is one gate setting.
type lexVariant struct {
	name        string
	minTokens   int
	stemLen     int
	useAbstract bool
	// noAbsMinTokens, when > 0, replaces minTokens for a work with no abstract.
	// A work whose abstract is missing cannot supply a second token, so
	// demanding two of it is demanding something the data cannot provide.
	noAbsMinTokens int
}

// lexV0 is a FROZEN COPY OF THE PRE-V6 PRODUCTION GATE. It is no longer pinned
// to relevantToClaim by any test — the pin now anchors V6 — so nothing will tell
// you if you change it. Do not. It is the baseline the control-group delta in
// TestV6ControlCorporaUnmoved is computed against, and editing it would silently
// redefine what "the control corpora did not move" means.
//
// lexV4 is V6 without the no-abstract guard. Its only job is to bound what the
// guard is allowed to touch (TestV6NoAbstractGuardScope).
//
// lexV6 is the production rule, pinned by TestLexicalHarnessMatchesProduction.
var (
	lexV0 = lexVariant{"V0 1 token / 5 chars / title+topic (frozen pre-V6 gate)", 1, 5, false, 0}
	lexV4 = lexVariant{"V4 2 tokens / 5 chars / +abstract (no guard)", 2, 5, true, 0}
	lexV6 = lexVariant{"V6 V4 + 1 token when no abstract (production)", 2, 5, true, 1}
)

// lexDecide applies one variant to one work, choosing the haystack and the
// threshold together. Returns the verdict and the distinct stems that matched.
func lexDecide(v lexVariant, claim, abstract, title, topic string) (bool, []string) {
	min := v.minTokens
	if strings.TrimSpace(abstract) == "" && v.noAbsMinTokens > 0 {
		min = v.noAbsMinTokens
	}
	return lexKeepHay(claim, lexHay(abstract, title, topic, v.useAbstract), min, v.stemLen)
}

// lexHay builds the matched text. Field order cannot change a result — stems
// never contain a space, so none can straddle a join — but it mirrors production
// so the pin compares like with like.
func lexHay(abstract, title, topic string, useAbstract bool) string {
	if useAbstract {
		return strings.ToLower(abstract + " " + title + " " + topic)
	}
	return strings.ToLower(title + " " + topic)
}

// lexKeepHay is the gate over an arbitrary haystack. Stems are de-duplicated
// before matching, so one stem found twice does not satisfy a 2-token bar. It
// returns the matched stems as well as the verdict, so an exclusion can be
// explained rather than only counted.
func lexKeepHay(claim, hay string, minTokens, stemLen int) (bool, []string) {
	tokens := scengine.ClaimContentTokens(claim)
	if len(tokens) == 0 {
		return true, nil
	}
	var matched []string
	seen := map[string]bool{}
	for _, tok := range tokens {
		stem := tok
		if len(stem) > stemLen {
			stem = stem[:stemLen]
		}
		if seen[stem] {
			continue
		}
		seen[stem] = true
		if strings.Contains(hay, stem) {
			matched = append(matched, stem)
		}
	}
	return len(matched) >= minTokens, matched
}

func loadLexCorpus(t *testing.T, name string) lexCorpus {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(lexCorpusDir, name+".json"))
	if err != nil {
		t.Fatalf("read corpus %s: %v", name, err)
	}
	var c lexCorpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("parse corpus %s: %v", name, err)
	}
	if len(c.AllStudies) == 0 {
		t.Fatalf("corpus %s parsed to zero studies", name)
	}
	return c
}

// lexWork rebuilds the production input from an archived study, so the gate
// under test is fed what it is fed in a real run.
func lexWork(s lexStudy) scWork {
	return scWork{Title: s.Title, Topic: s.Topic, Abstract: s.Abstract}
}

func normDOI(doi string) string {
	d := strings.ToLower(strings.TrimSpace(doi))
	for _, p := range []string{"https://doi.org/", "http://doi.org/", "doi:"} {
		d = strings.TrimPrefix(d, p)
	}
	return d
}

func fmtStems(s []string) string {
	if len(s) == 0 {
		return "no claim token at all"
	}
	return "[" + strings.Join(s, " ") + "]"
}

func verdict(keep bool) string {
	if keep {
		return "KEPT"
	}
	return "DROPPED"
}

// TestLexicalHarnessMatchesProduction pins the harness at the V6 setting to the
// real relevantToClaim, over every archived study. It is the anchor for the two
// delta tests, and the first test to fail when production and the intended rule
// disagree.
func TestLexicalHarnessMatchesProduction(t *testing.T) {
	checked, drift := 0, 0
	for _, name := range lexCorpora {
		c := loadLexCorpus(t, name)
		for _, s := range c.AllStudies {
			want := relevantToClaim(c.Claim, lexWork(s))
			got, _ := lexDecide(lexV6, c.Claim, s.Abstract, s.Title, s.Topic)
			checked++
			if want == got {
				continue
			}
			drift++
			// Cap the noise: a wholesale rule difference makes every third work
			// drift, and 34 identical lines say nothing the first few do not.
			if drift <= 10 {
				t.Errorf("harness drift [%s] %q: relevantToClaim=%s harness(V6)=%s",
					name, s.Title, verdict(want), verdict(got))
			}
		}
	}
	if drift > 10 {
		t.Errorf("... and %d further drifting works (only the first 10 are listed)", drift-10)
	}
	if checked != lexTotalStudies {
		t.Errorf("archive size changed: checked %d studies, want %d — every count "+
			"asserted in this file was calibrated against %d",
			checked, lexTotalStudies, lexTotalStudies)
	}
	t.Logf("harness(V6) agrees with relevantToClaim on %d of %d studies", checked-drift, checked)
}

// vitaminCTargets is acceptance criterion (b): the two off-topic works that
// motivated the change must leave, and the apex evidence the corpus exists to
// find must stay.
//
// Keyed by DOI, never by a title fragment. Five of the keepers share one title,
// so a substring probe cannot tell them apart — and substring matching is the
// specific trap that produced six false readmissions during the topic-metadata
// measurement, where "Extracts" satisfied a search for "extra".
var vitaminCTargets = []struct {
	doi  string
	keep bool
	// title is for the failure message, and to catch a corpus whose DOIs were
	// re-pointed at different works. It is not the lookup key.
	title string
	role  string
}{
	{"10.1038/s41586-022-05022-3", false,
		"A non-canonical vitamin K cycle is a potent ferroptosis suppressor",
		"THE DEFECT: vitamin K, not C — reached the score as REFUTING at 0.90 confidence"},
	{"10.1105/tpc.111.095232", false,
		"Retrotransposons Control Fruit-Specific, Cold-Dependent Accumulation of Anthocyanins in Blood Oranges",
		"THE DEFECT: 'cold' is a storage temperature here — reached the score as SUPPORTING at 0.90"},

	{"10.1002/14651858.cd000980.pub4", true,
		"Vitamin C for preventing and treating the common cold",
		"apex evidence: Cochrane review, pub4"},
	{"10.1002/14651858.cd000980.pub3", true,
		"Vitamin C for preventing and treating the common cold",
		"apex evidence: Cochrane review, pub3"},
	{"10.1002/14651858.cd000980.pub2", true,
		"Vitamin C for preventing and treating the common cold",
		"apex evidence: Cochrane review, pub2"},
	{"10.1002/14651858.cd000980", true,
		"Vitamin C for preventing and treating the common cold",
		"apex evidence: Cochrane review, first version"},
	{"10.1371/journal.pmed.0020168", true,
		"Vitamin C for Preventing and Treating the Common Cold",
		"apex evidence: the PLoS Medicine version of the same review"},
	{"10.1002/ebch.266", true,
		"Cochrane review: Vitamin C for preventing and treating the common cold",
		"apex evidence: the Evidence-Based Child Health restatement of that review"},
	{"10.1002/ebch.261", true,
		"Commentaries on 'Vitamin C for preventing and treating the common cold' with responses from the review author",
		"apex evidence: the commentaries attached to that restatement"},
	{"10.5867/medwave.2018.04.7236", true,
		"Does vitamin C prevent the common cold?",
		"apex evidence: independent meta-analysis asking the claim verbatim"},
	{"10.1001/jama.1979.03290350028016", true,
		"Vitamin C Prophylaxis in Marine Recruits",
		"apex evidence: the RCT that V1 and V3 lost — the reason a title-only 2-token rule was rejected"},
}

// TestVitaminCTargetWorks is acceptance criterion (b), against production.
func TestVitaminCTargetWorks(t *testing.T) {
	c := loadLexCorpus(t, "vitaminc")
	byDOI := make(map[string]lexStudy, len(c.AllStudies))
	for _, s := range c.AllStudies {
		byDOI[normDOI(s.DOI)] = s
	}

	for _, tc := range vitaminCTargets {
		t.Run(tc.doi, func(t *testing.T) {
			s, ok := byDOI[normDOI(tc.doi)]
			if !ok {
				// A missing target silently turns this into a no-op, which is
				// the failure mode the corpus was archived to prevent.
				t.Fatalf("target work not in corpus: %s (%s)\n  role: %s",
					tc.doi, tc.title, tc.role)
			}
			got := relevantToClaim(c.Claim, lexWork(s))
			if got == tc.keep {
				return
			}
			_, matched := lexDecide(lexV6, c.Claim, s.Abstract, s.Title, s.Topic)
			t.Errorf("relevantToClaim %s, want %s\n  %s\n  doi   : %s\n  role  : %s\n"+
				"  stems : %s (abstract %d chars, design %s)",
				verdict(got), verdict(tc.keep), s.Title, s.DOI, tc.role,
				fmtStems(matched), len(s.Abstract), s.Design)
		})
	}
}

// Acceptance criterion (c): of the 12 apex-design works in vitaminc, 11 survive.
// The one that goes is named rather than left as arithmetic — a count that fell
// to 11 for a different reason would be a silent failure of exactly the kind
// this suite exists to catch.
const (
	vitaminCApexTotal = 12
	vitaminCApexKept  = 11
	// vitaminCApexDroppedDOI is "Vitamin A supplementation for preventing
	// morbidity and mortality in children from six months to five years of
	// age" — a Cochrane meta-analysis about vitamin A, matching only the
	// `vitam` stem of a vitamin C claim. Its removal is the gate working.
	vitaminCApexDroppedDOI = "10.1002/14651858.cd008524.pub3"
	// marineRecruitsDOI is the RCT whose loss disqualified V1 and V3.
	marineRecruitsDOI = "10.1001/jama.1979.03290350028016"
)

func TestVitaminCApexEvidenceRetained(t *testing.T) {
	c := loadLexCorpus(t, "vitaminc")
	total, kept := 0, 0
	marineKept := false
	var dropped []string

	for _, s := range c.AllStudies {
		if !lexApexDesigns[s.Design] {
			continue
		}
		total++
		if relevantToClaim(c.Claim, lexWork(s)) {
			kept++
			if normDOI(s.DOI) == marineRecruitsDOI {
				marineKept = true
			}
			continue
		}
		_, matched := lexDecide(lexV6, c.Claim, s.Abstract, s.Title, s.Topic)
		dropped = append(dropped, fmt.Sprintf("%s [%s] %s — has %s",
			s.DOI, s.Design, s.Title, fmtStems(matched)))
		if normDOI(s.DOI) != vitaminCApexDroppedDOI {
			t.Errorf("an unexpected apex work was excluded:\n  %s\n  %s\n"+
				"  the only apex exclusion this corpus tolerates is %s (a vitamin A review)",
				s.DOI, s.Title, vitaminCApexDroppedDOI)
		}
	}

	if total != vitaminCApexTotal {
		t.Errorf("apex-design works in vitaminc = %d, want %d (corpus changed?)",
			total, vitaminCApexTotal)
	}
	if kept != vitaminCApexKept {
		t.Errorf("%d apex-design works survive the gate, want %d\n  excluded:\n    %s",
			kept, vitaminCApexKept, strings.Join(append([]string{}, dropped...), "\n    "))
	}
	if !marineKept {
		t.Errorf("%q (Vitamin C Prophylaxis in Marine Recruits) was excluded — "+
			"this is the RCT whose loss disqualified V1 and V3", marineRecruitsDOI)
	}
	t.Logf("apex-design works kept: %d of %d; excluded: %v", kept, total, dropped)
}

// TestV6ControlCorporaUnmoved is acceptance criterion (a). The six harm corpora
// already run the gate and carry the calibrated results; any work production
// removes from them changes a number that was tuned against the old one.
//
// Measured without topic data, which is the strict direction — see the topic
// note near the top of this file before concluding a failure here is real.
func TestV6ControlCorporaUnmoved(t *testing.T) {
	movedCorpora := 0
	for _, name := range lexCorpora {
		if !lexHarmCorpora[name] {
			continue
		}
		c := loadLexCorpus(t, name)
		var moved []string
		for _, s := range c.AllStudies {
			before, _ := lexDecide(lexV0, c.Claim, s.Abstract, s.Title, s.Topic)
			after := relevantToClaim(c.Claim, lexWork(s))
			if before == after {
				continue
			}
			_, matched := lexDecide(lexV6, c.Claim, s.Abstract, s.Title, s.Topic)
			moved = append(moved, fmt.Sprintf("%s->%s [%s] %s — has %s",
				verdict(before), verdict(after), s.Design, s.Title, fmtStems(matched)))
		}
		if len(moved) == 0 {
			continue
		}
		movedCorpora++
		sort.Strings(moved)
		t.Errorf("control corpus %q moved (%d of %d works):\n    %s\n  claim: %q",
			name, len(moved), len(c.AllStudies), strings.Join(moved, "\n    "), c.Claim)
	}
	if movedCorpora != 0 {
		t.Errorf("%d of 6 control corpora moved; the acceptance bar is 0", movedCorpora)
	}
}

// TestV6NoAbstractGuardScope bounds what the guard is allowed to do. It exists
// so a work with no abstract is not asked for a token that could only come from
// text it does not have; it must therefore change verdicts ONLY on abstract-less
// works, and only in the KEEP direction. A guard that readmitted a work WITH an
// abstract would be doing something other than advertised.
func TestV6NoAbstractGuardScope(t *testing.T) {
	differing, guarded := 0, 0
	for _, name := range lexCorpora {
		c := loadLexCorpus(t, name)
		for _, s := range c.AllStudies {
			noGuard, _ := lexDecide(lexV4, c.Claim, s.Abstract, s.Title, s.Topic)
			prod := relevantToClaim(c.Claim, lexWork(s))
			if noGuard == prod {
				continue
			}
			differing++
			if strings.TrimSpace(s.Abstract) != "" {
				t.Errorf("a work WITH an abstract differs from the unguarded rule [%s]: "+
					"V4=%s production=%s\n  %s",
					name, verdict(noGuard), verdict(prod), s.Title)
				continue
			}
			if !prod {
				t.Errorf("the guard turned a KEEP into a DROP [%s]; it may only widen\n  %s",
					name, s.Title)
				continue
			}
			guarded++
		}
	}
	if differing == 0 {
		t.Errorf("the guard changed nothing on this archive — either the corpora " +
			"lost their abstract-less works or the guard is not wired up")
	}
	t.Logf("the guard readmitted %d abstract-less works and touched nothing else "+
		"(%d differing verdicts)", guarded, differing)
}

// lexDissentTolerancePP is how far dissent retention may fall below overall
// retention before the gate is judged to be tilting the result rather than
// filtering it.
//
// Measured on this archive when V6 landed: overall 88.5%, dissent 95.5%,
// difference +7.0 percentage points in dissent's favour. The V6 session that
// also had the fetched-topic sidecar loaded measured +5.0 pp (works 295 -> 267,
// dissent 67 -> 64); this suite runs without that sidecar, which removes text
// from the haystack and so drops a few more works. Both are well clear of the
// bar.
//
// The tolerance is a floor on acceptable behaviour, not a restatement of the
// measurement. Pinning the exact percentage would fail on any corpus refresh,
// while a refresh that inverted the SIGN is precisely what this must catch.
const lexDissentTolerancePP = 10.0

// TestV6StanceBalance audits the gate for manufactured unanimity. A gate that
// removes noise should remove it roughly evenly across stances. One that removes
// dissent faster than agreement is not filtering, it is tilting the score — and
// the tilt is invisible downstream, because the score is computed after the
// gate. NearUnanimous exists to warn about exactly this condition, so a gate
// that CAUSED it would be defeating its own alarm.
//
// The archive is post-V0, so the "before" column is the corpus as it stands.
func TestV6StanceBalance(t *testing.T) {
	before, after, dissentBefore, dissentAfter := 0, 0, 0, 0
	var removedDissent []string

	for _, name := range lexCorpora {
		c := loadLexCorpus(t, name)
		for _, s := range c.AllStudies {
			st := strings.ToLower(strings.TrimSpace(s.Stance))
			isDissent := st == "refuting" || st == "mixed"
			before++
			if isDissent {
				dissentBefore++
			}
			if relevantToClaim(c.Claim, lexWork(s)) {
				after++
				if isDissent {
					dissentAfter++
				}
				continue
			}
			if isDissent {
				// Named, not just counted: "3 of 67" is a ratio that means
				// nothing until you can read which three.
				_, matched := lexDecide(lexV6, c.Claim, s.Abstract, s.Title, s.Topic)
				removedDissent = append(removedDissent, fmt.Sprintf(
					"[%s] %s — %s (design %s, has %s)",
					name, strings.ToUpper(st), s.Title, s.Design, fmtStems(matched)))
			}
		}
	}

	if before != lexTotalStudies {
		t.Fatalf("counted %d studies, want %d", before, lexTotalStudies)
	}
	if dissentBefore == 0 {
		t.Fatal("no refuting or mixed works in the archive; the audit cannot run")
	}

	overall := 100 * float64(after) / float64(before)
	dissent := 100 * float64(dissentAfter) / float64(dissentBefore)
	t.Logf("works   : %d -> %d  (%.1f%% kept)", before, after, overall)
	t.Logf("dissent : %d -> %d  (%.1f%% kept)", dissentBefore, dissentAfter, dissent)
	t.Logf("difference: %+.1f pp (dissent may trail overall by at most %.1f pp)",
		dissent-overall, lexDissentTolerancePP)
	for _, r := range removedDissent {
		t.Logf("  removed dissent: %s", r)
	}

	if dissent < overall-lexDissentTolerancePP {
		t.Errorf("the gate removes dissent faster than the corpus as a whole: "+
			"overall retention %.1f%%, dissent retention %.1f%% (%+.1f pp, tolerance %.1f pp)\n"+
			"  removed dissenting works:\n    %s",
			overall, dissent, dissent-overall, lexDissentTolerancePP,
			strings.Join(removedDissent, "\n    "))
	}
}
