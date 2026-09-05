package cli

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestPhrase(t *testing.T) {
	tests := []struct {
		field, term, want string
	}{
		{"product_description", "ibuprofen", `product_description:"ibuprofen"`},
		{"recalling_firm", "Teva Pharma", `recalling_firm:"Teva Pharma"`},
		{"product_description", `bad"quote`, `product_description:"badquote"`}, // embedded quotes stripped
		{"recalling_firm", "  spaced  ", `recalling_firm:"spaced"`},            // trimmed
	}
	for _, tc := range tests {
		if got := phrase(tc.field, tc.term); got != tc.want {
			t.Errorf("phrase(%q,%q)=%q want %q", tc.field, tc.term, got, tc.want)
		}
	}
}

func TestNormalizeRecallDate(t *testing.T) {
	tests := []struct{ in, want string }{
		{"20260115", "2026-01-15"},
		{"2026-01-15", "2026-01-15"}, // already ISO → passthrough
		{"", ""},
		{"bad", "bad"},
		{"20261301", "20261301"}, // invalid month → passthrough
	}
	for _, tc := range tests {
		if got := normalizeRecallDate(tc.in); got != tc.want {
			t.Errorf("normalizeRecallDate(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestClassToLabel(t *testing.T) {
	for n, want := range map[int]string{1: "Class I", 2: "Class II", 3: "Class III"} {
		if got := classToLabel[n]; got != want {
			t.Errorf("classToLabel[%d]=%q want %q", n, got, want)
		}
	}
	if _, ok := classToLabel[4]; ok {
		t.Error("class 4 should not be valid")
	}
}

func TestWrapField(t *testing.T) {
	tests := []struct {
		in            string
		width, indent int
		want          string
	}{
		{"", 80, 16, ""},               // empty stays empty; dash() supplies the placeholder
		{" \t ", 80, 16, ""},           // whitespace-only trimmed away, as clip does
		{"abc def", 10, 10, "abc def"}, // no room for a value at all: emit, never loop
		{
			"Lot: A1, expires: 04/30/2027", 80, 16,
			"Lot: A1, expires: 04/30/2027", // fits the column, returned untouched
		},
		{
			"  Lot:  A1   B2  ", 40, 10,
			"Lot: A1 B2", // trimmed, and internal whitespace runs collapse to one space
		},
		{
			"aaaa bbbb cccc dddd eeee ffff gggg hhhh iiii jjjj kkkk", 30, 10,
			"aaaa bbbb cccc dddd\n          eeee ffff gggg hhhh\n          iiii jjjj kkkk",
		},
		{
			"supercalifragilisticexpialidocious", 20, 10,
			"supercalifragilisticexpialidocious", // lone oversized token overflows, never split
		},
		{
			"aaaaaaaaaaaaaaa bb cc", 20, 10,
			"aaaaaaaaaaaaaaa\n          bb cc", // an overflowing token still ends its line
		},
		{
			"\u03b1\u03b1\u03b1\u03b1 \u03b2\u03b2\u03b2\u03b2", 20, 10,
			"\u03b1\u03b1\u03b1\u03b1 \u03b2\u03b2\u03b2\u03b2", // 9 runes fits; 17 bytes would not — runes win
		},
		{ // real openFDA code_info at the shipped 80/16 geometry
			"Lot: a) 09JA2530, 31JA2507, expires: 04/30/2027; b) Lot: 09DE2412, 09JA2528, 29JA2511, expires: 04/30/2027",
			recallLineWidth, recallLabelWidth,
			"Lot: a) 09JA2530, 31JA2507, expires: 04/30/2027; b) Lot:\n                09DE2412, 09JA2528, 29JA2511, expires: 04/30/2027",
		},
	}
	for _, tc := range tests {
		if got := wrapField(tc.in, tc.width, tc.indent); got != tc.want {
			t.Errorf("wrapField(%q,%d,%d)=%q want %q", tc.in, tc.width, tc.indent, got, tc.want)
		}
	}
}

func TestClip(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under the cap", "short value", 20, "short value"},
		{"trims before measuring", "  short value  ", 20, "short value"},
		{"over the cap", "abcdefghij", 4, "abcd…"},
		{
			// A byte-based slice at n=3 would cut the 2-byte "®" in half and
			// emit an invalid rune; a rune-based one keeps it whole.
			"multi-byte value is never split",
			"a®bc",
			3,
			"a®b…",
		},
		{
			// Every rune is 3 bytes here, so len(s) would over-count by 3x and
			// clip a value that actually fits.
			"multi-byte value under the cap survives whole",
			"°°中中中",
			5,
			"°°中中中",
		},
	}
	for _, tc := range tests {
		if got := clip(tc.in, tc.n); got != tc.want {
			t.Errorf("%s: clip(%q,%d)=%q want %q", tc.name, tc.in, tc.n, got, tc.want)
		}
		if !utf8.ValidString(clip(tc.in, tc.n)) {
			t.Errorf("%s: clip(%q,%d) produced an invalid rune", tc.name, tc.in, tc.n)
		}
	}
}

// TestRenderRecordWrapsProse is the wiring check: Product and Reason must go
// through clip-then-wrap at the shipped geometry, and no line in the rendered
// block may exceed the 80-column budget.
func TestRenderRecordWrapsProse(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("alpha bravo charlie delta echo ", 30)) // ~890 chars
	var buf bytes.Buffer
	printRecallRecord(&buf, recallRecord{
		RecallNumber:         "D-0000-2026",
		Classification:       "Class II",
		RecallingFirm:        "Example Pharma Inc.",
		RecallInitiationDate: "20260101",
		Status:               "Ongoing",
		ProductDescription:   long,
		CodeInfo:             "Lot: A1, expires: 04/30/2027",
		ReasonForRecall:      long,
	})
	out := buf.String()

	lines := strings.Split(out, "\n")
	for _, label := range []string{"  Product:", "  Reason:"} {
		i := -1
		for k, line := range lines {
			if strings.HasPrefix(line, label) {
				i = k
				break
			}
		}
		if i < 0 {
			t.Fatalf("missing %q in output:\n%s", label, out)
		}
		// The value must have wrapped: the label line is followed by at least
		// one continuation line indented to the label width.
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], strings.Repeat(" ", recallLabelWidth)) {
			t.Errorf("%q did not wrap", strings.TrimSpace(label))
		}
	}
	if !strings.Contains(out, "…") {
		t.Error("a value past recallProseCap should have been clipped with an ellipsis")
	}
	for _, line := range strings.Split(out, "\n") {
		if n := utf8.RuneCountInString(line); n > recallLineWidth {
			t.Errorf("line exceeds the %d-column budget at %d runes: %q", recallLineWidth, n, line)
		}
	}
}
