package cli

// prisma_note_test.go pins the exclusion ledger: that every gate which removed
// works says so, that the number it reports is the number actually removed, and
// that the counts add up.
//
// The reason this needs its own suite is the V6 relevance gate. On the vitamin C
// corpus it takes the analysed set from 49 works to 32 — silent removal goes
// from 1 work to 17. One work quietly vanishing is forgivable; seventeen is a
// different corpus than the one the reader thinks they are looking at, and
// invisibility of exactly this kind is why the dead PICO gate went unnoticed.
//
// SCOPE, STATED HONESTLY. These tests exercise the ledger and the note builders
// (gateLedger, gateNotes, noSurvivorsNote) over the real gate and the real
// corpus. They do NOT drive newNovelConsensusCmd's RunE, because it obtains its
// client from flags.newClient() (root.go), which returns a concrete
// *client.Client with no seam. Making that injectable would have to be repeated
// in compare.go, which runs its own computeConsensus — a larger production
// surface than the repair. What is therefore NOT covered here is the wiring
// itself: that RunE builds the ledger from the same numbers it reports. That
// wiring is a handful of straight-line assignments in one function, reviewable
// by reading; the arithmetic and the phrasing are what needed tests.

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

// findNote returns the single note fragment containing marker, or "" if none
// does. Fails if more than one matches, since an assertion about "the relevance
// fragment" is meaningless when there are two.
func findNote(t *testing.T, notes []string, marker string) string {
	t.Helper()
	var hits []string
	for _, n := range notes {
		if strings.Contains(n, marker) {
			hits = append(hits, n)
		}
	}
	switch len(hits) {
	case 0:
		return ""
	case 1:
		return hits[0]
	default:
		t.Fatalf("%d note fragments mention %q, want at most 1:\n  %s",
			len(hits), marker, strings.Join(hits, "\n  "))
		return ""
	}
}

// leadingInt reads the integer a note fragment opens with ("17 off-topic ...").
func leadingInt(t *testing.T, note string) int {
	t.Helper()
	field, _, _ := strings.Cut(note, " ")
	n, err := strconv.Atoi(field)
	if err != nil {
		t.Fatalf("note does not begin with a count: %q", note)
	}
	return n
}

// ledgerFor builds a closed ledger: every fetched work is accounted for.
func ledgerFor(fetched, relExcluded, picoExcluded, retracted int, stems []string) gateLedger {
	relevant := fetched - relExcluded - picoExcluded
	return gateLedger{
		relevance: relevanceReport{
			Fetched:             fetched,
			Kept:                fetched - relExcluded,
			Excluded:            relExcluded,
			Stems:               stems,
			MinTokens:           relevanceMinTokens,
			MinTokensNoAbstract: relevanceMinTokensNoAbstract,
		},
		picoExcluded:      picoExcluded,
		relevantCount:     relevant,
		retractedExcluded: retracted,
		studyCount:        relevant - retracted,
	}
}

// TestGateNotesReportTheRelevanceExclusionCount is the core PRISMA assertion:
// the number in the note is the number of works removed, at any magnitude.
//
// The 1 and 17 rows are the two ends of the change under test — 1 exclusion is
// what the corpus produced before V6, 17 is what it produces after — and the
// point is that the reporting behaves identically at both. A note that only
// appears for small counts, or that reports a stale number once the count grows,
// is the failure this pins.
func TestGateNotesReportTheRelevanceExclusionCount(t *testing.T) {
	stems := []string{"vitam", "commo", "cold"}
	for _, excluded := range []int{0, 1, 17, 48} {
		t.Run(fmt.Sprintf("excluded=%d", excluded), func(t *testing.T) {
			g := ledgerFor(49, excluded, 0, 0, stems)
			notes := gateNotes(g)
			note := findNote(t, notes, "relevance gate")

			if excluded == 0 {
				if note != "" {
					t.Errorf("relevance fragment emitted for a gate that excluded nothing: %q", note)
				}
				return
			}
			if note == "" {
				t.Fatalf("%d works were excluded and no note mentions the relevance gate; notes = %v",
					excluded, notes)
			}
			if got := leadingInt(t, note); got != excluded {
				t.Errorf("note reports %d exclusions, want %d\n  %s", got, excluded, note)
			}
			// The rule, not just the count. A reader has to be able to check the
			// claim against the study list; "off-topic" alone is unfalsifiable.
			for _, want := range []string{
				strconv.Itoa(relevanceMinTokens),
				strconv.Itoa(relevanceMinTokensNoAbstract),
				"vitam", "abstract+title+topic",
			} {
				if !strings.Contains(note, want) {
					t.Errorf("note does not state %q, so the exclusion cannot be audited\n  %s",
						want, note)
				}
			}
		})
	}
}

// TestGateLedgerInvariants pins both arithmetic chains:
//
//	fetched        = relevance_excluded + pico_excluded + relevant_count
//	relevant_count = retracted_excluded + study_count
//
// and pins that a broken ledger says so in the notes rather than reporting
// totals that do not add up.
func TestGateLedgerInvariants(t *testing.T) {
	stems := []string{"vitam", "commo", "cold"}

	t.Run("closed ledger is consistent and warns about nothing", func(t *testing.T) {
		g := ledgerFor(49, 17, 0, 2, stems)
		if !g.consistent() {
			t.Fatalf("a closed ledger reports inconsistent: %+v", g)
		}
		if g.relevance.Fetched != g.relevance.Excluded+g.picoExcluded+g.relevantCount {
			t.Errorf("chain 1 broken: %d != %d + %d + %d", g.relevance.Fetched,
				g.relevance.Excluded, g.picoExcluded, g.relevantCount)
		}
		if g.relevantCount != g.retractedExcluded+g.studyCount {
			t.Errorf("chain 2 broken: %d != %d + %d", g.relevantCount,
				g.retractedExcluded, g.studyCount)
		}
		if n := findNote(t, gateNotes(g), "WARNING"); n != "" {
			t.Errorf("consistent ledger emitted a warning: %q", n)
		}
	})

	t.Run("a work lost between the gates is caught", func(t *testing.T) {
		g := ledgerFor(49, 17, 0, 2, stems)
		g.relevantCount-- // one work vanished with no gate claiming it
		if g.consistent() {
			t.Error("chain 1 violated but consistent() returned true")
		}
		if n := findNote(t, gateNotes(g), "WARNING"); n == "" {
			t.Errorf("inconsistent ledger emitted no warning; notes = %v", gateNotes(g))
		}
	})

	t.Run("a work lost after the retraction gate is caught", func(t *testing.T) {
		g := ledgerFor(49, 17, 0, 2, stems)
		g.studyCount-- // scored fewer works than survived, with none retracted
		if g.consistent() {
			t.Error("chain 2 violated but consistent() returned true")
		}
		if n := findNote(t, gateNotes(g), "WARNING"); n == "" {
			t.Errorf("inconsistent ledger emitted no warning; notes = %v", gateNotes(g))
		}
	})
}

// vitaminCExpected pins the real numbers on the real corpus. These are the
// figures the PR description quotes, and pinning them here means a future gate
// change that alters the corpus size cannot land while the prose still claims
// the old one.
const (
	vitaminCFetched  = 49
	vitaminCExcluded = 17
	vitaminCKept     = 32
)

// corpusWorks rebuilds the production input from an archived corpus, so the gate
// under test is fed what a real run feeds it.
func corpusWorks(c lexCorpus) []scWork {
	works := make([]scWork, 0, len(c.AllStudies))
	for _, s := range c.AllStudies {
		works = append(works, scWork{
			Title: s.Title, DOI: s.DOI, Topic: s.Topic, Abstract: s.Abstract,
		})
	}
	return works
}

// TestRelevanceLedgerOnVitaminCCorpus runs the real gate over the real corpus
// and checks that the note reports what the gate actually did — at 17
// exclusions, which is the magnitude the change introduces.
func TestRelevanceLedgerOnVitaminCCorpus(t *testing.T) {
	c := loadLexCorpus(t, "vitaminc")
	kept, rep := filterRelevantReport(c.Claim, corpusWorks(c))

	if rep.Fetched != vitaminCFetched {
		t.Errorf("fetched %d works, want %d (corpus changed?)", rep.Fetched, vitaminCFetched)
	}
	if rep.Excluded != vitaminCExcluded {
		t.Errorf("relevance gate excluded %d works, want %d", rep.Excluded, vitaminCExcluded)
	}
	if rep.Kept != vitaminCKept || len(kept) != vitaminCKept {
		t.Errorf("relevance gate kept %d works (slice len %d), want %d",
			rep.Kept, len(kept), vitaminCKept)
	}
	if rep.Kept != len(kept) {
		t.Errorf("report disagrees with the returned slice: Kept=%d len=%d", rep.Kept, len(kept))
	}

	// The PICO gate contributes nothing here, and the reason matters — see
	// TestPICOGateIsDeadForBenefitClaims below.
	g := gateLedger{relevance: rep, relevantCount: rep.Kept, studyCount: rep.Kept}
	if !g.consistent() {
		t.Errorf("ledger does not close on the real corpus: %d != %d + %d + %d",
			rep.Fetched, rep.Excluded, g.picoExcluded, g.relevantCount)
	}

	note := findNote(t, gateNotes(g), "relevance gate")
	if note == "" {
		t.Fatalf("%d works were excluded from the vitamin C corpus and nothing said so",
			rep.Excluded)
	}
	if got := leadingInt(t, note); got != rep.Excluded {
		t.Errorf("note reports %d exclusions, gate performed %d\n  %s", got, rep.Excluded, note)
	}
	t.Logf("vitaminc: %d fetched -> %d kept, %d excluded", rep.Fetched, rep.Kept, rep.Excluded)
	t.Logf("note: %s", note)
}

// TestNoSurvivorsNoteExplainsWhy covers the branch that matters most and is
// hardest to notice: every work excluded, nothing to score.
//
// Before this change that branch blamed the PICO gate unconditionally, which was
// wrong twice — the relevance exclusions were absent from the report entirely,
// and on a benefit claim the PICO gate had not run at all. An empty answer with
// no explanation is indistinguishable from a broken query, and V6 makes this
// branch more common rather than less.
func TestNoSurvivorsNoteExplainsWhy(t *testing.T) {
	c := loadLexCorpus(t, "vitaminc")

	// An off-topic claim, chosen with care. The first attempt here was "quantum
	// chromodynamics explains colour confinement", and it did NOT empty the
	// corpus: one work survived on `quant` -> "quantifying", `chrom` ->
	// "chromatography" (in an abbreviation glossary) and `confi` ->
	// "confirmed". None of those is subject matter; all three are methods prose,
	// and the work that carried them has the corpus's longest abstract by a wide
	// margin (24,724 chars).
	//
	// That is a real property of the V6 gate, recorded as a fragility in the PR:
	// widening the haystack to the abstract also admits the methodological
	// boilerplate every abstract carries regardless of topic, and the exposure
	// grows with abstract length. It did not surface during calibration because
	// every measured claim was on-topic for its corpus.
	//
	// So this claim is picked so that only ONE of its stems (`follo`, from
	// "followed"/"following") can plausibly hit boilerplate, while the bar is
	// two. Measured over the same corpus: 0 of 49 survive.
	const claim = "sonnets follow iambic pentameter"

	kept, rep := filterRelevantReport(claim, corpusWorks(c))
	if len(kept) != 0 || rep.Kept != 0 {
		// Name the survivors and what they matched: a failure here is far more
		// likely to be the boilerplate effect above than a broken gate, and the
		// two demand opposite responses.
		var why []string
		for _, w := range kept {
			hay := strings.ToLower(w.Abstract + " " + w.Title + " " + w.Topic)
			var hits []string
			for _, stem := range claimRelevanceStems(claim) {
				if strings.Contains(hay, stem) {
					hits = append(hits, stem)
				}
			}
			why = append(why, fmt.Sprintf("%s — matched %v (abstract %d chars)",
				w.Title, hits, len(w.Abstract)))
		}
		t.Fatalf("an off-topic claim did not empty the corpus; %d work(s) survived:\n    %s\n"+
			"  If the matched stems are methods words rather than subject matter, this is the "+
			"boilerplate fragility described above — pick a claim with fewer boilerplate-prone "+
			"stems rather than weakening the gate.",
			rep.Kept, strings.Join(why, "\n    "))
	}
	if rep.Excluded != vitaminCFetched {
		t.Fatalf("excluded %d of %d works", rep.Excluded, vitaminCFetched)
	}

	g := gateLedger{relevance: rep, relevantCount: 0, studyCount: 0}
	if !g.consistent() {
		t.Errorf("ledger does not close when everything is excluded: %+v", g)
	}

	note := noSurvivorsNote(g)
	if note == "" {
		t.Fatal("a run with zero survivors produced no note at all")
	}
	// It must say how many were fetched, name the gate that removed them, and
	// report the count — otherwise the reader cannot tell an empty literature
	// from an over-strict gate.
	for _, want := range []string{
		strconv.Itoa(vitaminCFetched),
		"relevance gate",
		strconv.Itoa(rep.Excluded) + " off-topic",
	} {
		if !strings.Contains(note, want) {
			t.Errorf("zero-survivor note does not state %q\n  %s", want, note)
		}
	}
	t.Logf("note: %s", note)
}

// TestPICOGateSplitsBenefitClaims guards the PICO gate against silently going
// dead on benefit-shaped claims — the state it was in until 2026-08-02.
//
// History, because the inversion of this test is the whole point. PICOTokens
// used to split a claim with claimSides, which searches only claimHarmCues, so
// the benefit verbs ("prevents", "improves", "reduces the risk") never matched.
// A benefit claim yielded no sides, PICOTokens returned (nil, nil), and
// IsPICORelevant short-circuited to true for every work: the gate passed
// everything and reported 0 exclusions, which reads identically to "the gate ran
// and found nothing". That ambiguity is how the dead gate went unnoticed for
// months, and the predecessor of this test existed to document it.
//
// 4d81ac382 ended it by routing PICOTokens through polarityVerbCues, which is
// direction-neutral and fires on benefit verbs as well as harm verbs. Measured
// on the vitaminc corpus (49 works): 0 exclusions before, 23 after, all 23
// verified correct (vitamin D/K/A papers and cold-unrelated works), with no
// regression across the 12 harm corpora.
//
// So the assertion is now the opposite one: these claims MUST split. A future
// change that returns empty token lists here has not merely altered a heuristic
// — it has reverted the gate to passing every work on the benefit path while
// still reporting a zero that looks like a measurement.
func TestPICOGateSplitsBenefitClaims(t *testing.T) {
	benefit := []string{
		"vitamin C prevents the common cold",
		"omega-3 improves cardiovascular health",
		"probiotics improve health",
	}
	for _, claim := range benefit {
		t.Run(claim, func(t *testing.T) {
			iv, out := scengine.PICOTokens(claim)
			if len(iv) == 0 || len(out) == 0 {
				t.Fatalf("PICOTokens(%q) = (iv=%v, out=%v) — a benefit claim no longer splits, "+
					"so IsPICORelevant short-circuits to true for every work and pico_excluded "+
					"reports 0 because the gate never ran, not because it excluded nothing",
					claim, iv, out)
			}
			// The consequence of a live gate: a work naming neither side is
			// excluded, so the count is a measurement rather than a constant.
			if scengine.IsPICORelevant("", "utterly unrelated title", iv, out) {
				t.Errorf("with both PICO sides present (iv=%v out=%v) an unrelated work must be "+
					"excluded; the gate is passing everything again", iv, out)
			}
		})
	}

	// The control: a harm claim DOES split, which is what keeps the gate alive
	// on the calibrated corpora.
	iv, outTok := scengine.PICOTokens("cell phones cause brain cancer")
	if len(iv) == 0 || len(outTok) == 0 {
		t.Errorf("a harm claim no longer splits (iv=%v out=%v); the PICO gate is now dead "+
			"everywhere, not just on benefit claims", iv, outTok)
	}
}
