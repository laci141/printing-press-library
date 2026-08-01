package scengine

import (
	"strings"
	"testing"
)

// TestNormalizeDashImpact measures how many works in the full corpora contain a
// non-ASCII dash, and how many of those would match their claim's hyphenated
// subject only after folding.
//
// It changes nothing. It reports.
func TestNormalizeDashImpact(t *testing.T) {
	corpora := []struct {
		name    string
		subject string // the claim's subject as an ASCII-hyphenated string
	}{
		{"omega3", "omega-3"},
		{"vitaminc", "vitamin c"},
		{"vitamind", "vitamin d"},
		{"cellphones", "cell phone"},
		{"redmeat_run1", "red meat"},
	}

	for _, c := range corpora {
		res := mustLoadFullCorpus(t, c.name)

		withOddDash, rescued := 0, 0
		for _, s := range res.AllStudies {
			raw := strings.ToLower(s.Title + " " + s.Abstract)
			folded := strings.ToLower(normalizeText(s.Title + " " + s.Abstract))

			if raw != folded {
				withOddDash++
			}
			// Works the subject match finds only after folding.
			if !strings.Contains(raw, c.subject) && strings.Contains(folded, c.subject) {
				rescued++
				t.Logf("  [%s] RESCUED BY FOLD stance=%-12s cites=%-5d %s",
					c.name, s.Stance, s.CitedByCount, s.Title)
			}
		}

		t.Logf("MEASURED %-14s subject=%q: %d of %d works contain a non-ASCII dash, %d rescued",
			c.name, c.subject, withOddDash, len(res.AllStudies), rescued)
	}
}
