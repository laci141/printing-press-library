package scengine

// nodoi_probe_test.go measures the blind spot in dedupeVersions
// (internal/cli/dedupe.go:79-80), which skips every work without a DOI:
// "no DOI: never deduplicable". A work it skips can never be removed as a
// duplicate, so it always keeps its vote.
//
// THE MEASUREMENT, 2026-08-07, thirteen corpora, 249 works:
//
//	works without a DOI          = 5
//	of those, works that vote    = 4
//
// The four voters, with what they actually are:
//
//	meditation  [supporting] Meditation practices for health: state of the
//	            research — narrative-review, 1500-char abstract, 481 cites.
//	            A real AHRQ evidence report.
//	vaccines    [refuting]   Mercury and autism: accelerating evidence? —
//	            narrative-review, 1483-char abstract, 172 cites. A real
//	            journal article.
//	vaccines    [refuting]   Adverse Effects of Vaccines: Evidence and
//	            Causality — a BOOK (IOM report), 349 cites.
//	vaccines    [refuting]   Vaccines Did Not Cause Rachel's Autism — a BOOK.
//	            Its "abstract" is the table of contents: "Foreword, by Arthur
//	            L. Caplan Preface 1. Family Interrupted 2. Saving Lives..."
//
// WHY NO CODE IS BEING WRITTEN, and it is not the reason first guessed.
//
// The first guess was that all five are books and reports, so none is a real
// study. Measuring the design and abstract fields refuted that: two of the
// four are ordinary peer-reviewed work that simply predates or skipped DOI
// registration.
//
// The real reason is simpler. dedupeVersions removes DUPLICATES, which needs
// a PAIR. All five DOI-less works here are distinct from one another and from
// everything else in their corpus — preprint_probe_test.go measured 6 colliding
// titles across 275 works and none of them is one of these. A lone work with
// no twin would not be removed even if the deduplicator did handle missing
// DOIs. The blind spot is real and inert at the same time.
//
// WHAT THIS PROBE DID SURFACE, which is a different problem: a memoir whose
// abstract is its table of contents votes refuting at 0.9, the same weight as
// a meta-analysis. That is source-type quality, not deduplication, and it is
// kin to the homeopathy case — see the handover notes.
//
// Log-only. Nothing here asserts.
//
// NOTE ON CORPUS SHAPE: twelve corpora wrap the payload as
// {export, rows, response{result{all_studies}}}. vitaminc.json does not — its
// keys sit at the top level. Both shapes are handled below; a probe that
// assumed one shape would silently skip a corpus and under-report. The shape
// column in the log exists to prove no corpus was skipped.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

type nodoiStudy struct {
	Title            string  `json:"title"`
	Year             int     `json:"year"`
	DOI              string  `json:"doi"`
	Design           string  `json:"design"`
	Stance           string  `json:"stance"`
	StanceConfidence float64 `json:"stance_confidence"`
	Abstract         string  `json:"abstract"`
	CitedByCount     int     `json:"cited_by_count"`
}

type nodoiResult struct {
	AllStudies []nodoiStudy `json:"all_studies"`
}

type nodoiResponse struct {
	Result nodoiResult `json:"result"`
}

type nodoiWrapped struct {
	Response nodoiResponse `json:"response"`
}

// nodoiLoadStudies handles both corpus shapes. It returns the study list and
// the shape it matched, so the log can prove no corpus was silently skipped.
func nodoiLoadStudies(t *testing.T, path string) ([]nodoiStudy, string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	// Shape A: wrapped in response.result
	var wrapped nodoiWrapped
	if err := json.Unmarshal(raw, &wrapped); err == nil {
		if n := len(wrapped.Response.Result.AllStudies); n > 0 {
			return wrapped.Response.Result.AllStudies, "wrapped"
		}
	}

	// Shape B: all_studies at the top level (vitaminc.json)
	var flat nodoiResult
	if err := json.Unmarshal(raw, &flat); err == nil {
		if n := len(flat.AllStudies); n > 0 {
			return flat.AllStudies, "flat"
		}
	}

	return nil, "EMPTY"
}

// nodoiVotes reports whether a stance contributes to the consensus score.
// inconclusive works are carried in the list but do not move the number.
// Named with a prefix because opinion_probe_test.go already defines "votes"
// in this same package.
func nodoiVotes(stance string) bool {
	switch stance {
	case "supporting", "refuting", "mixed":
		return true
	default:
		return false
	}
}

// describeNoDOI renders one voter with the evidence needed to judge what kind
// of source it is. Title alone is not enough — "Adverse Effects of Vaccines:
// Evidence and Causality" reads like a review and is a book.
func describeNoDOI(corpus string, s nodoiStudy) string {
	return corpus + ": [" + s.Stance + "] " + s.Title +
		"  (design=" + s.Design +
		", abstract=" + strconv.Itoa(len(s.Abstract)) + " chars" +
		", cites=" + strconv.Itoa(s.CitedByCount) + ")"
}

func TestNoDOIWorksThatVote(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "corpora", "*.json"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no corpora found — wrong working directory?")
	}
	sort.Strings(paths)

	var totalWorks, totalNoDOI, totalNoDOIVoters int
	var voters []string

	t.Logf("%-18s %6s %8s %8s  %s", "corpus", "works", "no-DOI", "voting", "shape")

	for _, p := range paths {
		studies, shape := nodoiLoadStudies(t, p)
		name := filepath.Base(p)

		if shape == "EMPTY" {
			t.Errorf("%-18s  PARSED NOTHING — corpus shape not recognised", name)
			continue
		}

		var noDOI, noDOIVoters int
		for _, s := range studies {
			if s.DOI != "" {
				continue
			}
			noDOI++
			if nodoiVotes(s.Stance) {
				noDOIVoters++
				voters = append(voters, describeNoDOI(name, s))
			}
		}

		totalWorks += len(studies)
		totalNoDOI += noDOI
		totalNoDOIVoters += noDOIVoters

		t.Logf("%-18s %6d %8d %8d  %s", name, len(studies), noDOI, noDOIVoters, shape)
	}

	t.Logf("")
	t.Logf("TOTAL: %d works, %d without DOI, %d of those vote",
		totalWorks, totalNoDOI, totalNoDOIVoters)

	if totalNoDOIVoters == 0 {
		t.Logf("VERDICT: no undeduplicable voters in this sample.")
	} else {
		t.Logf("VERDICT: %d undeduplicable voters. Design and abstract length", totalNoDOIVoters)
		t.Logf("are shown so a book can be told apart from a journal article")
		t.Logf("without guessing from the title:")
		for _, v := range voters {
			t.Logf("   %s", v)
		}
	}
}
