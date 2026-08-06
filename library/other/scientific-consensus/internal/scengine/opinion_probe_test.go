package scengine

// opinion_probe_test.go records a MEASURED-AND-REJECTED proposal, so nobody
// spends another evening rediscovering it. Nothing here asserts; it logs.
//
// THE PROBLEM, observed live on 2026-08-06 on corpova.pubvera.com:
//
//	"Evaluating Social Media Networks in Medicines Safety Surveillance"
//	voted SUPPORTING on "HPV vaccine causes infertility".
//
// The firing sentence is in RESULTS, so the B2 framing gate has no purchase on
// it — no "objective:", no "whether", no "belief":
//
//	"Assertion analysis predominantly showed affirmation of the association
//	 of rosiglitazone/cardiovascular events (72%; n = 1821) and of HPV
//	 vaccine/infertility ..."
//
// It is a real finding. What it finds is that 84% of tweets asserted the
// association — a measurement of public belief, not of physiology. The engine
// reads it as evidence for the claim.
//
// THE PROPOSAL: a fourth gate excluding sentences that report opinion
// measurement rather than physiological finding.
//
// THE MEASUREMENT, 2026-08-06, on the twelve calibrated corpora
// (testdata/corpora_full, untruncated abstracts):
//
//	tier     works hit   voters   control-group voters
//	strong       2          0            0
//	medium      10          7            2
//	weak        27         21            8
//
// VERDICT: REJECTED. The only safe tier is inert, and the tiers with any reach
// break the control group.
//
//   - weak would exclude four voters from the `vaccines` corpus, including
//     "The Genetics of Autism" and "Effective Messages in Vaccine Promotion" —
//     the works holding the calibrated -1.00 — plus eight ESC guideline papers
//     in omega3. All are legitimate science that merely contains the word
//     "awareness" or "knowledge".
//   - medium would drop "Parental Vaccine Safety Concerns in 2009" and
//     "Vaccine Criticism on the World Wide Web" over the words "web"/"online".
//   - strong touches two works in `vaccines` and NEITHER VOTES, so the gate
//     would change nothing at all.
//
// WHY THE REACH IS ZERO WHERE IT IS SAFE: the motivating work is not in any of
// the twelve corpora. It came from the HPV claim, which was archived on the
// same day as hpvinfertility.json precisely so this class has a measurable
// home. The failure class is REAL but RARE; one rare miss does not justify a
// gate that damages the common case.
//
// WHEN TO REVISIT: when two or three more cases of the same shape have been
// collected. Then TestOpinionCueReachHPV below has something to measure and the
// trade-off can be recomputed. Until then, leave the engine alone.
//
// Per the "subsets are measured as subsets" rule, the three tiers must never be
// summed: gates interact, and one work can carry cues from several tiers.

import (
	"regexp"
	"strings"
	"testing"
)

// opinionCuesStrong name the object of study so specifically that a finding
// about them is almost certainly a finding about what people said.
var opinionCuesStrong = regexp.MustCompile(`(?i)(assertion analys\w*|social media|\btweets?\b|\btwitter\b|\bfacebook\b|\breddit\b|\byoutube\b)`)

// opinionCuesMedium are suggestive but appear in legitimate work too.
var opinionCuesMedium = regexp.MustCompile(`(?i)(\bposts?\b|\bonline\b|\bweb pages?\b|\bblogs?\b|\bforums?\b)`)

// opinionCuesWeak are the risky ones: an acceptability study or a knowledge
// survey is ordinary epidemiology.
var opinionCuesWeak = regexp.MustCompile(`(?i)(\bacceptability\b|\bawareness\b|\bsurvey\w*|\battitudes?\b|\bwillingness\b|\bknowledge\b|\bperception\w*)`)

// harmCorpora is the calibrated control group. vaccines sits at -1.00 and must
// not move; a tier that touches a voter here is disqualified regardless of what
// it gains elsewhere.
var harmCorpora = []string{"vaccines", "cellphones", "coffee", "sweeteners", "redmeat_run1", "redmeat_run2"}

func isHarmCorpus(name string) bool {
	for _, h := range harmCorpora {
		if h == name {
			return true
		}
	}
	return false
}

// votes reports whether a stance label counts toward the score. Only voters
// matter: gating a work that is already inconclusive changes nothing.
func votes(stance string) bool {
	s := strings.ToLower(stance)
	return s == "supporting" || s == "refuting"
}

type opinionTier struct {
	name string
	re   *regexp.Regexp
}

var opinionTiers = []opinionTier{
	{"strong", opinionCuesStrong},
	{"medium", opinionCuesMedium},
	{"weak", opinionCuesWeak},
}

// TestOpinionCueReach is the rejection evidence: per corpus and per tier, how
// many works carry a cue and how many of those actually vote. Log-only.
func TestOpinionCueReach(t *testing.T) {
	for _, tier := range opinionTiers {
		t.Logf("=== tier: %s ===", tier.name)
		t.Logf("%-14s %6s %6s %8s %8s %s",
			"corpus", "works", "hit", "voters", "control", "voting titles")

		totalHit, totalVoters, controlVoters := 0, 0, 0

		for _, name := range allCorpora {
			full := mustLoadFullCorpus(t, name)
			hit, voters := 0, 0
			var titles []string

			for _, w := range full.AllStudies {
				hay := strings.ToLower(w.Title + " " + w.Abstract)
				if !tier.re.MatchString(hay) {
					continue
				}
				hit++
				if votes(w.Stance) {
					voters++
					title := w.Title
					if len(title) > 60 {
						title = title[:60] + "..."
					}
					titles = append(titles, w.Stance+": "+title)
				}
			}

			control := ""
			if isHarmCorpus(name) {
				control = "CONTROL"
				controlVoters += voters
			}
			totalHit += hit
			totalVoters += voters

			t.Logf("%-14s %6d %6d %8d %8s %s",
				name, len(full.AllStudies), hit, voters, control,
				strings.Join(titles, " | "))
		}

		t.Logf("TOTAL %s: hit=%d voters=%d  CONTROL-GROUP VOTERS=%d",
			tier.name, totalHit, totalVoters, controlVoters)
		if controlVoters > 0 {
			t.Logf("  ^^ tier %s breaks the control group — disqualified", tier.name)
		}
	}
}

// TestOpinionCueReachHPV measures the same three tiers on the corpus that
// actually contains the failure. Kept separate from allCorpora on purpose: the
// twelve calibrated corpora were exported by one method and mixing in a corpus
// produced another way would make every earlier measurement incomparable.
//
// hpvinfertility.json: exported 2026-08-06 from the container with the API key,
// `consensus "HPV vaccine causes infertility" --json --limit 50
// --full-abstracts`, heuristic stance path. 29 works in all_studies, verdict evidence-refutes
// at -0.81, confidence 0.65, apex meta-analysis.
func TestOpinionCueReachHPV(t *testing.T) {
	full, err := loadFullCorpus("hpvinfertility")
	if err != nil {
		t.Skipf("hpvinfertility corpus not present: %v", err)
	}

	t.Logf("corpus: hpvinfertility, %d works", len(full.AllStudies))

	for _, tier := range opinionTiers {
		hit, voters := 0, 0
		var titles []string

		for _, w := range full.AllStudies {
			hay := strings.ToLower(w.Title + " " + w.Abstract)
			if !tier.re.MatchString(hay) {
				continue
			}
			hit++
			if votes(w.Stance) {
				voters++
				title := w.Title
				if len(title) > 70 {
					title = title[:70] + "..."
				}
				titles = append(titles, w.Stance+": "+title)
			}
		}

		t.Logf("%-8s hit=%-3d voters=%-3d %s",
			tier.name, hit, voters, strings.Join(titles, " | "))
	}
}
