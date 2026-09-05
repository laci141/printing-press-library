package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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

// TestRecallRecordCodeInfoRoundTrip covers the whole path the Lots/Expiry line
// travels: raw openFDA JSON -> enforcementEnvelope/recallRecord decode -> the
// rendered record block. Constructing recallRecord literally would skip the
// json tag, which is exactly the wiring most likely to break silently, so the
// fixture is decoded rather than built.
func TestRecallRecordCodeInfoRoundTrip(t *testing.T) {
	// Long enough to force wrapping at the shipped 80/16 geometry.
	const codeInfo = "Lot: a) 09JA2530, 31JA2507, expires: 04/30/2027; b) Lot: 09DE2412, " +
		"09JA2528, 29JA2511, expires: 04/30/2027; c) Lot: 17FE2533, expires: 05/31/2027"

	payload := `{
	  "meta": {"results": {"total": 1}},
	  "results": [
	    {
	      "recall_number": "D-1234-2026",
	      "classification": "Class II",
	      "status": "Ongoing",
	      "recalling_firm": "Example Pharma Inc.",
	      "reason_for_recall": "Subpotent drug product.",
	      "product_description": "Ibuprofen tablets, 200 mg, 100-count bottle.",
	      "code_info": "` + codeInfo + `",
	      "recall_initiation_date": "20260115"
	    }
	  ]
	}`

	var env enforcementEnvelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		t.Fatalf("decode enforcement payload: %v", err)
	}
	if len(env.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(env.Results))
	}
	// Guards the struct tag itself: a wrong or missing `json:"code_info"` leaves
	// this empty and every assertion below would pass against a dash.
	if env.Results[0].CodeInfo != codeInfo {
		t.Fatalf("CodeInfo did not decode from code_info: got %q", env.Results[0].CodeInfo)
	}

	var buf bytes.Buffer
	printRecallRecord(&buf, env.Results[0])
	out := buf.String()

	lines := strings.Split(out, "\n")
	first := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "  Lots/Expiry:") {
			first = i
			break
		}
	}
	if first == -1 {
		t.Fatalf("no Lots/Expiry: line in output:\n%s", out)
	}
	if strings.Contains(lines[first], "-") && strings.TrimSpace(lines[first]) == "Lots/Expiry:  -" {
		t.Fatalf("Lots/Expiry rendered the empty placeholder:\n%s", out)
	}

	// Every token of the source value must survive: a truncated lot number is
	// not partially useful, so nothing may be dropped or split.
	for _, tok := range strings.Fields(codeInfo) {
		if !strings.Contains(out, tok) {
			t.Errorf("token %q from code_info missing from output:\n%s", tok, out)
		}
	}

	// Continuation lines belong under the value column, not at the left margin,
	// and must not be mistaken for a new label row.
	cont := 0
	for _, l := range lines[first+1:] {
		if strings.HasPrefix(l, "  Reason:") {
			break
		}
		if strings.TrimSpace(l) == "" {
			continue
		}
		cont++
		if !strings.HasPrefix(l, strings.Repeat(" ", recallLabelWidth)) {
			t.Errorf("continuation line not indented to column %d: %q", recallLabelWidth, l)
		}
		if len(l) > recallLabelWidth && l[recallLabelWidth] == ' ' {
			t.Errorf("continuation line over-indented past the value column: %q", l)
		}
	}
	if cont == 0 {
		t.Errorf("fixture did not wrap; test would not prove indentation:\n%s", out)
	}
}
