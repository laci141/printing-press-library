package cli

import "testing"

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
