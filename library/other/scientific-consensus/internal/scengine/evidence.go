// Package scengine implements the scientific-consensus analytic engines:
// study-design (evidence) classification, stance detection, and consensus
// scoring. It is pure logic with no network or store dependencies so it can be
// unit-tested in isolation and reused across commands.
package scengine

import (
	"regexp"
	"strings"
)

// Design is a study-design classification, ordered loosely from strongest to
// weakest evidence. The zero value is DesignUnknown.
type Design string

const (
	DesignUmbrellaReview   Design = "umbrella-review"
	DesignMetaAnalysis     Design = "meta-analysis"
	DesignSystematicReview Design = "systematic-review"
	DesignRCT              Design = "randomized-controlled-trial"
	DesignCohort           Design = "cohort-study"
	DesignCaseControl      Design = "case-control-study"
	DesignCrossSectional   Design = "cross-sectional-study"
	DesignCaseSeries       Design = "case-series"
	DesignCaseReport       Design = "case-report"
	DesignNarrativeReview  Design = "narrative-review"
	DesignUnknown          Design = "unclassified"
)

// PyramidOrder is the evidence pyramid from apex (strongest) to base (weakest).
// Render order for `evidence` output and tier weighting both derive from this.
var PyramidOrder = []Design{
	DesignUmbrellaReview,
	DesignMetaAnalysis,
	DesignSystematicReview,
	DesignRCT,
	DesignCohort,
	DesignCaseControl,
	DesignCrossSectional,
	DesignCaseSeries,
	DesignCaseReport,
	DesignNarrativeReview,
	DesignUnknown,
}

// tierWeight assigns each design an evidence weight used by the consensus
// engine. Higher = stronger evidence. Kept on a 1..10 scale.
var tierWeight = map[Design]float64{
	DesignUmbrellaReview:   10,
	DesignMetaAnalysis:     9,
	DesignSystematicReview: 8,
	DesignRCT:              7,
	DesignCohort:           5,
	DesignCaseControl:      4,
	DesignCrossSectional:   3,
	DesignCaseSeries:       2,
	DesignCaseReport:       1,
	DesignNarrativeReview:  1,
	DesignUnknown:          1,
}

// TierWeight returns the evidence weight for a design (>=1).
func TierWeight(d Design) float64 {
	if w, ok := tierWeight[d]; ok {
		return w
	}
	return 1
}

// TierRank returns the apex-to-base rank of a design (0 = strongest). Unknown
// designs rank last.
func TierRank(d Design) int {
	for i, o := range PyramidOrder {
		if o == d {
			return i
		}
	}
	return len(PyramidOrder)
}

// Method records how a classification was reached, so output never presents a
// heuristic guess as if it were authoritative.
type Method string

const (
	MethodPubMedPubType Method = "pubmed-pubtype" // authoritative MeSH publication type
	MethodPublication   Method = "publication-type"
	MethodHeuristic     Method = "title-abstract-heuristic"
	MethodOpenAlexType  Method = "openalex-type"
	MethodNone          Method = "none"
)

// Classification is the result of classifying one work.
type Classification struct {
	Design Design  `json:"design"`
	Tier   float64 `json:"tier_weight"`
	Method Method  `json:"method"`
}

// pubtypeMap maps authoritative publication-type labels (PubMed MeSH pubtype or
// Semantic Scholar publicationTypes) to a Design. Keys are lowercased.
var pubtypeMap = map[string]Design{
	"meta-analysis":               DesignMetaAnalysis,
	"systematic review":           DesignSystematicReview,
	"randomized controlled trial": DesignRCT,
	"controlled clinical trial":   DesignRCT,
	"clinical trial":              DesignRCT,
	"observational study":         DesignCohort,
	"case reports":                DesignCaseReport,
	"review":                      DesignNarrativeReview,
}

// Heuristic regexes, evaluated apex-first; first match wins.
var heuristicTiers = []struct {
	re     *regexp.Regexp
	design Design
}{
	{regexp.MustCompile(`(?i)\bumbrella review\b|\boverview of (systematic )?reviews\b`), DesignUmbrellaReview},
	{regexp.MustCompile(`(?i)\bmeta-?analy[sz]is\b|\bmeta-?analytic\b`), DesignMetaAnalysis},
	{regexp.MustCompile(`(?i)\bsystematic review\b|\bsystematic literature review\b`), DesignSystematicReview},
	{regexp.MustCompile(`(?i)\brandomi[sz]ed (controlled |clinical )?trial\b|\bdouble-?blind\b|\bplacebo-?controlled\b|\b\brct\b`), DesignRCT},
	{regexp.MustCompile(`(?i)\bcohort (study|studies)\b|\bprospective cohort\b|\bretrospective cohort\b|\blongitudinal (study|cohort)\b`), DesignCohort},
	{regexp.MustCompile(`(?i)\bcase[- ]control (study|studies)\b|\bnested case[- ]control\b`), DesignCaseControl},
	{regexp.MustCompile(`(?i)\bcross[- ]sectional\b|\bprevalence (study|survey)\b`), DesignCrossSectional},
	{regexp.MustCompile(`(?i)\bcase series\b`), DesignCaseSeries},
	{regexp.MustCompile(`(?i)\bcase report\b|\ba case of\b`), DesignCaseReport},
	{regexp.MustCompile(`(?i)\bnarrative review\b|\bliterature review\b|\bscoping review\b`), DesignNarrativeReview},
}

// hostTrialMention matches a phrase that credits a DIFFERENT study for the
// randomisation: a work carried out inside, or drawn from, someone else's
// trial. The abstract of a nested study routinely names its host, and the
// design heuristics cannot tell whose design they just read.
//
// Measured on 10.1371/journal.pone.0045231, whose title ends "A Case Control
// Study": its abstract says the participants were "within a phase IV
// cluster-randomised trial of HPV vaccination", and that one phrase promoted
// an eight-tier jump, from case-control to RCT. The work then entered the
// evidence pyramid at its apex and carried a 0.90-confidence stance.
//
// The determiner is what makes it someone else's trial. "enrolled in A
// randomised trial" names a study this work is reporting from; "enrolled in
// THIS randomised trial" is the work describing itself, and an earlier
// version of this pattern demoted a genuine RCT for saying so — measured on
// the saffron corpus, where it was the only match the pattern had. Requiring
// a/an/the immediately after the phrase excludes this, our, the present and
// the current without listing them.
//
// The 60-character bound keeps the two halves in one clause, and counts
// characters rather than words because the randomisation word is often
// hyphenated into another: "cluster-randomised" is one token, and a
// word-stepping pattern never reaches inside it. Unbounded, the
// pattern would join a "within" early in a long abstract to a genuine
// randomisation sentence much later.
var hostTrialMention = regexp.MustCompile(`(?i)\b(within|nested (in|within)|participants (in|of)|recruited (from|within)|enrolled (in|within)|sub-?study of|secondary analysis of|ancillary to)\s+(?:an?|the)\s[^.]{0,60}?(randomi[sz]ed|\brct\b|placebo-?controlled|double-?blind)`)

// titleClaimsRCT reports whether the TITLE itself calls the work randomised.
// A work that says so in its own title is stating its own design, so a host
// trial mentioned in the abstract must not overrule it.
var titleClaimsRCT = regexp.MustCompile(`(?i)\brandomi[sz]ed (controlled |clinical )?trial\b|\bdouble-?blind\b|\bplacebo-?controlled\b|\brct\b`)

// hostTrialOnly reports whether the ONLY randomisation signal in the text is
// the one credited to another study. It removes the host-trial phrases and
// asks whether an RCT signal survives in what is left.
//
// Removing the phrase rather than the sentence it sits in is the point. A
// single sentence can do both jobs — "participants were drawn from within a
// larger placebo-controlled trial and we then randomised 200 of them in a
// double-blind design" names a host AND reports this work's own allocation.
// Dropping the whole sentence would take the second with the first and demote
// a genuine RCT; dropping just the matched span leaves "and we then randomised
// 200 of them in a double-blind design" standing, and the RCT tier holds.
//
// The replacement is a space rather than an empty string so the text on either
// side cannot fuse into a word that was never written.
func hostTrialOnly(hay string) bool {
	if !hostTrialMention.MatchString(hay) {
		return false
	}
	return !rctSignal.MatchString(hostTrialMention.ReplaceAllString(hay, " "))
}

// rctSignal is the RCT tier's own pattern, named so hostTrialOnly can ask the
// same question the cascade asks rather than keeping a second copy that could
// drift away from it.
var rctSignal = regexp.MustCompile(`(?i)\brandomi[sz]ed (controlled |clinical )?trial\b|\bdouble-?blind\b|\bplacebo-?controlled\b|\b\brct\b`)

// ClassifyDesign determines a study design using the cascade documented in the
// research brief: authoritative publication types first (PubMed MeSH / Semantic
// Scholar), then title+abstract heuristics, then the coarse OpenAlex type, and
// finally Unknown. pubTypes may be nil.
func ClassifyDesign(title, abstract, openalexType string, pubTypes []string) Classification {
	// 1. Authoritative publication types (apex-first by tier rank).
	best := DesignUnknown
	bestRank := len(PyramidOrder)
	for _, pt := range pubTypes {
		if d, ok := pubtypeMap[strings.ToLower(strings.TrimSpace(pt))]; ok {
			if r := TierRank(d); r < bestRank {
				best, bestRank = d, r
			}
		}
	}
	// Every pubtype except the generic "review" names a specific design and is
	// trusted outright. "review" is the one label a source can apply to a work
	// whose own title and abstract call it a meta-analysis: measured on
	// 10.3389/fnut.2022.1084455, a 55-RCT meta-analysis tagged only "Review",
	// that costs the work eight tier points (9 -> 1). So when the generic label
	// is all that came back, the heuristics still run and the stronger of the
	// two answers wins.
	if best != DesignUnknown && best != DesignNarrativeReview {
		return Classification{Design: best, Tier: TierWeight(best), Method: MethodPubMedPubType}
	}

	// 2. Title + abstract heuristics.
	hay := strings.ToLower(title + ". " + abstract)
	// A randomisation phrase that belongs to a host trial is not this work's
	// design. Skipping the RCT tier lets the cascade fall through to the label
	// the work gives itself, which for a nested study is usually in the title.
	//
	// The suppression is scoped to the sentences that name a host. An abstract
	// can do both — report the trial it was drawn from AND describe its own
	// randomisation — and a work-wide flag would discard the second along with
	// the first, demoting a genuine RCT for having said where its participants
	// came from. Only when nothing outside the host sentences carries an RCT
	// signal is the tier skipped.
	skipRCT := !titleClaimsRCT.MatchString(title) && hostTrialOnly(hay)
	for _, t := range heuristicTiers {
		if t.design == DesignRCT && skipRCT {
			continue
		}
		if t.re.MatchString(hay) {
			if TierRank(t.design) < bestRank {
				return Classification{Design: t.design, Tier: TierWeight(t.design), Method: MethodHeuristic}
			}
			break
		}
	}
	// The generic pubtype stands when the heuristics found nothing stronger.
	if best != DesignUnknown {
		return Classification{Design: best, Tier: TierWeight(best), Method: MethodPubMedPubType}
	}

	// 3. Coarse OpenAlex type fallback.
	switch strings.ToLower(strings.TrimSpace(openalexType)) {
	case "review":
		return Classification{Design: DesignNarrativeReview, Tier: TierWeight(DesignNarrativeReview), Method: MethodOpenAlexType}
	case "article", "preprint":
		return Classification{Design: DesignUnknown, Tier: TierWeight(DesignUnknown), Method: MethodOpenAlexType}
	}

	return Classification{Design: DesignUnknown, Tier: TierWeight(DesignUnknown), Method: MethodNone}
}

// Pyramid counts classifications by design and returns counts in apex-to-base
// order alongside the apex design actually present.
type PyramidLevel struct {
	Design Design `json:"design"`
	Count  int    `json:"count"`
}

// Pyramid aggregates classifications into apex-to-base levels (zero-count
// levels included so the shape is stable).
func Pyramid(cs []Classification) []PyramidLevel {
	counts := map[Design]int{}
	for _, c := range cs {
		counts[c.Design]++
	}
	out := make([]PyramidLevel, 0, len(PyramidOrder))
	for _, d := range PyramidOrder {
		out = append(out, PyramidLevel{Design: d, Count: counts[d]})
	}
	return out
}

// ApexDesign returns the strongest design present in the set, or Unknown.
func ApexDesign(cs []Classification) Design {
	apex := DesignUnknown
	apexRank := len(PyramidOrder)
	for _, c := range cs {
		if r := TierRank(c.Design); r < apexRank {
			apex, apexRank = c.Design, r
		}
	}
	return apex
}
