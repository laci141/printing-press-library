package scengine

import (
	"regexp"
	"strings"
)

// Stance is a work's reported finding relative to an intervention/claim
// "working" or being positively associated. It is a heuristic signal, not a
// trained classifier — output always carries the method used.
type Stance string

const (
	StanceSupporting   Stance = "supporting"   // reports a positive/significant effect or association
	StanceRefuting     Stance = "refuting"     // reports null, no effect, or harm
	StanceMixed        Stance = "mixed"        // both positive and negative signals
	StanceInconclusive Stance = "inconclusive" // no clear directional signal
)

var (
	// Positive/effect cues: the intervention did something beneficial or a
	// positive association was found. "increas*" is matched plainly here; Go's
	// RE2 engine has no negative lookahead, so harm-context phrasing like
	// "increased risk" / "increased mortality" is excluded per-match in
	// ClassifyStance by re-inspecting the matched window against
	// increaseHarmContext (that phrasing belongs to harmCues instead).
	supportCues = regexp.MustCompile(`(?i)\b(improv\w+|increas\w+|reduc\w+ (the )?risk|lower\w* (the )?risk|effective\b|efficac\w+|beneficial|benefit\w*|protect\w+|associated with (a )?(reduc\w+|lower|decreas\w+)|significant\w* (improv|increas|reduc|benefit)|positive (effect|association|impact)|enhanc\w+|alleviat\w+|prevent\w+)`)

	// increaseHarmContext detects when an "increas*" support match is actually
	// harm-context phrasing ("increased risk", "increases the mortality",
	// "increased incidence of ..."). Applied to a short window starting at the
	// support match; a hit means the match is not counted as support. This is
	// the RE2-safe replacement for a negative lookahead — do not reintroduce
	// (?!...) here.
	increaseHarmContext = regexp.MustCompile(`(?i)\bincreas\w+\s+(the\s+|a\s+)?(risk|mortality|morbidity|incidence|harm\w*|adverse|complication\w*|death\w*|odds of)`)

	// Null / no-effect cues.
	nullCues = regexp.MustCompile(`(?i)\b(no (significant )?(association|effect|difference|benefit|evidence|impact|correlation)|not (significant\w*|associated|effective)|did not (significantly )?(improv|increas|reduc|affect|differ|change)|ineffective|no statistically significant|failed to|without (a )?(significant )?(effect|benefit)|null (result|effect|finding))`)

	// Harm / negative-effect cues (treated as refuting an intervention claim).
	harmCues = regexp.MustCompile(`(?i)\b(increas\w+ (the )?(risk|mortality|morbidity|incidence)|harmful|adverse (effect|event|outcome)|worsen\w*|associated with (a )?(higher|increas\w+|greater) (risk|mortality|incidence)|detrimental|toxic\w*|negative (effect|impact|association)|deteriorat\w+)`)
)

// ClassifyStance scores a single work's title+abstract. claim is currently used
// only as context for future AI-assisted modes; the heuristic baseline derives
// stance from the reported finding's polarity. confidence is 0..1.
func ClassifyStance(title, abstract, claim string) (Stance, float64) {
	hay := strings.ToLower(title + ". " + abstract)
	// Count support matches, excluding "increas*" hits whose immediate context
	// is a harm claim ("increased risk/mortality/..."). RE2 has no negative
	// lookahead, so each match window is re-inspected instead.
	support := 0
	for _, loc := range supportCues.FindAllStringIndex(hay, -1) {
		if strings.Contains(hay[loc[0]:loc[1]], "increas") {
			end := loc[1] + 24 // room for "increased" + " the mortality" etc.
			if end > len(hay) {
				end = len(hay)
			}
			if increaseHarmContext.MatchString(hay[loc[0]:end]) {
				continue
			}
		}
		support++
	}
	null := len(nullCues.FindAllString(hay, -1))
	harm := len(harmCues.FindAllString(hay, -1))

	// Null cues frequently overlap with support phrasing ("no significant
	// reduction in risk"); count net.
	pos := support
	neg := null + harm
	total := pos + neg

	if total == 0 {
		return StanceInconclusive, 0.2
	}

	// Mixed when both sides have meaningful signal and neither dominates.
	if pos > 0 && neg > 0 {
		ratio := float64(max(pos, neg)) / float64(total)
		if ratio < 0.65 {
			return StanceMixed, 0.4 + 0.2*ratio
		}
	}

	if pos > neg {
		return StanceSupporting, confidenceFrom(pos, total)
	}
	if neg > pos {
		return StanceRefuting, confidenceFrom(neg, total)
	}
	return StanceMixed, 0.45
}

func confidenceFrom(dominant, total int) float64 {
	if total == 0 {
		return 0.2
	}
	c := float64(dominant) / float64(total)
	// Scale into a calibrated-feeling 0.3..0.9 band; more cues = more confident.
	conf := 0.3 + 0.6*c
	if total >= 4 {
		conf += 0.05
	}
	if conf > 0.95 {
		conf = 0.95
	}
	return conf
}
