package cli

import (
	"os"
	"regexp"
	"strings"
)

// Measured against the live API on 2026-08-05, one work per page:
//
//	search=vitamin D reduces respiratory infections
//	  oql "works where full text has (...)"      121312 works, 457 ms
//	filter=title_and_abstract.search:vitamin D respiratory infections
//	  oql "works where title/abstract has (...)"   3318 works,  52 ms
//
// A 36x narrowing and 9x faster. The oql field proves the server really
// filtered on the title_and_abstract column: an OpenAlex blog post says
// filter-style search is deprecated and redirects to ?search=, but no such
// redirect happens, and the official deprecations page does not list this
// filter at all. Measured behaviour beats both documents, so the switch below
// defaults to OFF and the old ?search= path stays the shipped default until a
// live corpus measurement says otherwise.
//
// A control query confirmed the filter ANDs its terms: "xyzzyx cardiovascular"
// returns 0. Without that, the narrowed counts could have been anything.

// narrowSearchEnabled reports whether to send the claim as a
// title_and_abstract.search filter instead of the fulltext ?search= parameter.
//
// ON by default since 2026-08-05. Set SC_NARROW_SEARCH=0 to restore the old
// fulltext path. The escape hatch stays because fifteen claims are not the
// whole literature, and a claim whose words live only in an article's body
// would fetch less under narrowing than it does today.
func narrowSearchEnabled() bool {
	return os.Getenv("SC_NARROW_SEARCH") != "0"
}

// searchStopwords are dropped from the narrowed query. The polarity verbs go
// too: "reduces" and "prevents" describe the claim's direction, not its
// subject, and every paper that studies an intervention says one of them
// somewhere. Keeping them narrows on the wrong axis.
var searchStopwords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "of": {}, "in": {}, "and": {}, "or": {},
	"to": {}, "for": {}, "with": {}, "on": {}, "at": {}, "by": {}, "is": {},
	"are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "it": {}, "its": {},
	"that": {}, "this": {}, "from": {}, "as": {}, "not": {}, "no": {},
	"reduces": {}, "reduce": {}, "reduced": {}, "reducing": {},
	"prevents": {}, "prevent": {}, "prevented": {}, "preventing": {},
	"improves": {}, "improve": {}, "improved": {}, "improving": {},
	"increases": {}, "increase": {}, "increased": {}, "increasing": {},
	"decreases": {}, "decrease": {}, "decreased": {}, "decreasing": {},
	"causes": {}, "cause": {}, "caused": {}, "causing": {},
	"extends": {}, "extend": {}, "extended": {}, "extending": {},
	"lowers": {}, "lower": {}, "lowered": {}, "lowering": {},
	"raises": {}, "raise": {}, "raised": {}, "raising": {},
	"leads": {}, "lead": {}, "led": {}, "helps": {}, "help": {}, "helped": {},
	"protects": {}, "protect": {}, "protected": {}, "worsens": {}, "worsen": {},
}

// filterUnsafe strips the characters that would break out of a single
// title_and_abstract.search filter value. A comma starts a new filter clause in
// OpenAlex and a pipe starts an OR clause between different filters, so a claim
// containing either would silently become a different query.
var filterUnsafe = regexp.MustCompile(`[,|:+&?#]`)

var searchTokenSplit = regexp.MustCompile(`[^\p{L}\p{N}\-]+`)

// buildNarrowedSearch turns a claim into a title_and_abstract.search value.
//
// Tokens keep their full surface form. It does NOT reuse PICOTokens: those are
// stemmed to 5 characters for strings.Contains matching inside the gate, and
// "probi" or "coffe" match nothing at an API that indexes whole words. The two
// consumers need opposite things from the same word, which is why the gate is
// left completely untouched by this change.
//
// Hyphens survive: "omega-3" is how the literature writes it, and the gate's
// reason for dropping the numeric part does not apply here.
//
// Returns "" when nothing usable is left, and callers must then fall back to
// the fulltext search rather than sending an empty filter — an empty
// title_and_abstract.search matches everything, which is the opposite of the
// intent.
func buildNarrowedSearch(claim string) string {
	cleaned := filterUnsafe.ReplaceAllString(claim, " ")
	raw := searchTokenSplit.Split(strings.ToLower(cleaned), -1)

	out := make([]string, 0, len(raw))
	for _, tok := range raw {
		tok = strings.Trim(tok, "-")
		if tok == "" {
			continue
		}
		if _, stop := searchStopwords[tok]; stop {
			continue
		}
		out = append(out, tok)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, " ")
}
