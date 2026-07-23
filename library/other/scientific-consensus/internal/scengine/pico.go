package scengine

import "strings"

// picoGateEnabled controls the Phase 3 PICO relevance gate.
// Always true in production; tests may toggle it to measure impact.
var picoGateEnabled = true

// IsPICORelevant checks if a study text (abstract + title) names both sides of
// the claim: at least one intervention token AND at least one outcome token
// must appear, case-insensitive, anywhere in the text.
//
// The relation between the two sides is AND, but WITHIN a side it is OR. That
// asymmetry is the whole design. A claim side is usually a modifier plus a head
// noun ("artificial sweeteners"), and the literature routinely swaps the
// modifier while keeping the head: "low-calorie sweeteners", "low-energy
// sweetener", "non-nutritive sweeteners". Requiring every token of a side — or
// only its first token — throws away exactly the papers most on point. The
// corpus measurement made this concrete: matching on "artif" alone dropped 69%
// of the sweetener corpus, including the meta-analyses on sweeteners and body
// weight. Requiring both SIDES still keeps the gate's purpose intact: a paper
// that names only the intervention or only the outcome is not evidence about
// the relation the claim asserts.
//
// Exported (unlike claimSides) because the gate runs in internal/cli, where
// the fetched works are still available before scoring.
func IsPICORelevant(workAbstract, workTitle string, ivTokens, outTokens []string) bool {
	if !picoGateEnabled {
		return true
	}
	if len(ivTokens) == 0 || len(outTokens) == 0 {
		// Empty claim tokens → assume all studies are relevant (bypass gate).
		// A claim with no polarity verb cannot be split into sides, and a gate
		// that cannot name what it is filtering for must not filter.
		return true
	}

	text := strings.ToLower(workAbstract + " " + workTitle)
	return containsAnyToken(text, ivTokens) && containsAnyToken(text, outTokens)
}

// containsAnyToken reports whether text contains any of the tokens, matching
// case-insensitively. text is expected to be lowercase already.
func containsAnyToken(text string, tokens []string) bool {
	for _, tok := range tokens {
		if tok == "" {
			continue
		}
		if strings.Contains(text, strings.ToLower(tok)) {
			return true
		}
	}
	return false
}

// picoStopwords are function words that must never satisfy a side of the gate.
// They occur in essentially every abstract, so a side that reduced to one of
// them would make the gate a no-op for that side while still looking enforced.
var picoStopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "of": {}, "in": {}, "and": {}, "or": {},
	"to": {}, "for": {}, "with": {}, "on": {}, "at": {}, "by": {},
}

// PICOTokens derives the intervention and outcome token lists for the gate from
// a claim, reusing the same claimSides split that the stance classifier's
// pairing gate uses — so the gate and the classifier never disagree about which
// side of the claim is which.
//
// Every content token of each side is returned, not just the first: within a
// side the gate is an OR, so more tokens mean more ways for a paper to name
// that side. All tokens are claimStemLen-truncated stems ("sweeteners" →
// "sweet"), which is what makes a plain strings.Contains match tolerate
// inflection and compounding ("sweetener", "sweeteners", "non-nutritive
// sweeteners").
//
// Returns empty slices when the claim cannot be split, which callers must treat
// as "bypass the gate".
func PICOTokens(claim string) (ivTokens, outTokens []string) {
	intervention, outcome := claimSides(claim)
	if len(intervention) == 0 || len(outcome) == 0 {
		return nil, nil
	}
	return dropPICOStopwords(intervention), dropPICOStopwords(outcome)
}

// dropPICOStopwords removes function words from one side's token list. If the
// side consists of nothing else, the original list is returned unchanged: a
// side that survives the split is real, and silently emptying it here would
// bypass the gate for BOTH sides rather than tighten it.
func dropPICOStopwords(tokens []string) []string {
	kept := make([]string, 0, len(tokens))
	for _, tok := range tokens {
		if _, stop := picoStopwords[tok]; stop {
			continue
		}
		kept = append(kept, tok)
	}
	if len(kept) == 0 {
		return tokens
	}
	return kept
}
