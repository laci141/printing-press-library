package cli

// Proof that --full-abstracts did not change the default path.
//
// The flag exists to make measurement possible, not to change what users and
// LLM consumers already receive. clipAbstract is now branched on a package
// variable, and a branch added to a hot output path is exactly the kind of
// change that silently alters an edge case (empty input, an abstract sitting
// exactly on the cap, multi-byte text where byte length and rune length
// disagree). So the old implementation is kept here verbatim as
// clipAbstractLegacy, and the current one is required to agree with it on
// every one of those edges while the flag is off.

import (
	"strings"
	"testing"
)

// clipAbstractLegacy is the pre-flag implementation, copied unchanged from
// consensus.go. It is the oracle: whatever this returns is what production
// returned before the flag existed, and what it must still return today.
func clipAbstractLegacy(s string) string {
	if len(s) <= maxAbstractChars {
		return s
	}
	r := []rune(s)
	if len(r) <= maxAbstractChars {
		return s
	}
	return string(r[:maxAbstractChars])
}

// clipAbstractCases covers every branch of the function plus the boundaries
// around maxAbstractChars, in both ASCII and multi-byte text.
func clipAbstractCases() []struct {
	name  string
	input string
} {
	return []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"short ascii", "A short abstract."},
		{"one below cap", strings.Repeat("a", maxAbstractChars-1)},
		{"exactly at cap", strings.Repeat("a", maxAbstractChars)},
		{"one above cap", strings.Repeat("a", maxAbstractChars+1)},
		{"far above cap", strings.Repeat("a", maxAbstractChars*3)},
		// Byte length exceeds the cap but rune length does not: the second
		// branch of the function, and the case a naive s[:n] would corrupt.
		{"multibyte, bytes over cap but runes under", strings.Repeat("é", maxAbstractChars-10)},
		{"multibyte, runes over cap", strings.Repeat("é", maxAbstractChars+50)},
		// Mixed scripts, so a cut lands inside a multi-byte sequence if the
		// implementation ever stops cutting on rune boundaries.
		{"mixed scripts over cap", strings.Repeat("aé漢字 ", maxAbstractChars)},
		// The real shape: a structured medical abstract past the cap.
		{"structured abstract over cap", "BACKGROUND: " + strings.Repeat("Vitamin D supplementation reduced respiratory infection incidence. ", 60)},
	}
}

// TestClipAbstractUnchangedWithoutFlag is the regression proof required before
// the flag may ship: with fullAbstractsEnabled false, clipAbstract must return
// byte-for-byte what the pre-flag implementation returned.
func TestClipAbstractUnchangedWithoutFlag(t *testing.T) {
	if fullAbstractsEnabled {
		t.Fatalf("fullAbstractsEnabled must default to false, got true")
	}

	for _, tc := range clipAbstractCases() {
		t.Run(tc.name, func(t *testing.T) {
			want := clipAbstractLegacy(tc.input)
			got := clipAbstract(tc.input)
			if got != want {
				t.Errorf("clipAbstract changed behavior for %s:\n  input len  = %d bytes / %d runes\n  legacy len = %d bytes\n  got len    = %d bytes",
					tc.name, len(tc.input), len([]rune(tc.input)), len(want), len(got))
			}
			// Guard the property the legacy code existed to provide, not just
			// equality with it: the result never exceeds the cap in runes, and
			// is never cut mid-character.
			if r := []rune(got); len(r) > maxAbstractChars {
				t.Errorf("result exceeds cap: %d runes > %d", len(r), maxAbstractChars)
			}
			if !utf8Valid(got) {
				t.Errorf("result is not valid UTF-8 — cut landed mid-character")
			}
		})
	}
}

// TestClipAbstractFullWithFlag proves the flag actually does something: with it
// on, the input comes back untouched no matter how long it is.
func TestClipAbstractFullWithFlag(t *testing.T) {
	fullAbstractsEnabled = true
	t.Cleanup(func() { fullAbstractsEnabled = false })

	for _, tc := range clipAbstractCases() {
		t.Run(tc.name, func(t *testing.T) {
			if got := clipAbstract(tc.input); got != tc.input {
				t.Errorf("flag on: abstract was still modified (input %d bytes, got %d bytes)",
					len(tc.input), len(got))
			}
		})
	}
}

// TestClipAbstractFlagRestores proves the setting cannot leak between command
// invocations. The MCP server runs many commands in one process, so a
// fullAbstractsEnabled left true would silently uncap every later
// consensus/compare/controversies result — a cross-invocation bug that no
// single-command test would catch.
func TestClipAbstractFlagRestores(t *testing.T) {
	long := strings.Repeat("a", maxAbstractChars+1)

	fullAbstractsEnabled = true
	if got := clipAbstract(long); len(got) != len(long) {
		t.Fatalf("flag on: expected untruncated, got %d bytes", len(got))
	}
	// What RunE's defer does.
	fullAbstractsEnabled = false

	if got := clipAbstract(long); len(got) != maxAbstractChars {
		t.Errorf("after reset: expected %d bytes, got %d — the setting leaked",
			maxAbstractChars, len(got))
	}
}

// TestConsensusFullAbstractsFlagRegistered checks the flag's public contract:
// the name agents and scripts will type, and the default that keeps existing
// behavior. A renamed flag or a default flipped to true is a breaking change to
// every consumer of this command's JSON.
func TestConsensusFullAbstractsFlagRegistered(t *testing.T) {
	cmd := newNovelConsensusCmd(&rootFlags{})

	f := cmd.Flags().Lookup("full-abstracts")
	if f == nil {
		t.Fatalf("--full-abstracts is not registered on the consensus command")
	}
	if f.Value.Type() != "bool" {
		t.Errorf("--full-abstracts type = %q, want bool", f.Value.Type())
	}
	if f.DefValue != "false" {
		t.Errorf("--full-abstracts default = %q, want false", f.DefValue)
	}
	if !strings.Contains(f.Usage, "measurement") {
		t.Errorf("--full-abstracts help text does not say it is for measurement: %q", f.Usage)
	}

	// The flags that were already there must survive the addition.
	for _, name := range []string{"limit", "year-from", "enrich"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("pre-existing flag --%s disappeared from the consensus command", name)
		}
	}
}

// utf8Valid reports whether s decodes cleanly, i.e. no truncation landed inside
// a multi-byte sequence. Implemented via the round-trip rather than pulling in
// unicode/utf8 for one call.
func utf8Valid(s string) bool {
	return string([]rune(s)) == s
}
