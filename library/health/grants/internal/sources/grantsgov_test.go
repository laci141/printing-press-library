package sources

import "testing"

// TestNormalizeText pins the two defect shapes measured in live Grants.gov
// data on 2026-09-03, plus the non-breaking space that html.UnescapeString
// produces from &nbsp;.
//
// The last case is the reason this uses strings.Fields rather than a regexp:
// Go's regexp \s class is ASCII-only, so a `\s+` implementation passes every
// case here except that one.
func TestNormalizeText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			// DFOP0018819, measured.
			name: "doubled interior space",
			in:   "Egypt  Annual Program Statement",
			want: "Egypt Annual Program Statement",
		},
		{
			// SFOP0008547, measured.
			name: "doubled space after colon",
			in:   "Notice of Intent:  Program to End Modern Slavery FY 2022",
			want: "Notice of Intent: Program to End Modern Slavery FY 2022",
		},
		{
			// HM047623BAA0001, measured.
			name: "trailing space",
			in:   "National Geospatial-Intelligence Agency ",
			want: "National Geospatial-Intelligence Agency",
		},
		{
			// 693JJ323NF00014, measured.
			name: "trailing space on agency",
			in:   "DOT Federal Highway Administration ",
			want: "DOT Federal Highway Administration",
		},
		{
			name: "non-breaking space from &nbsp;",
			in:   "Forest Legacy\u00a0Program",
			want: "Forest Legacy Program",
		},
		{
			name: "tab and newline",
			in:   "Broad Agency\tAnnouncement\n(BAA)",
			want: "Broad Agency Announcement (BAA)",
		},
		{
			name: "already clean is unchanged",
			in:   "Museums for America (2027)",
			want: "Museums for America (2027)",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
		{
			name: "whitespace only collapses to empty",
			in:   "   ",
			want: "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeText(c.in); got != c.want {
				t.Errorf("normalizeText(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestNormalizeTextPreservesSingleSpaces guards the mutation that would make
// normalizeText strip ALL whitespace instead of collapsing runs — a
// strings.Join with "" rather than " " still passes a trailing-space test.
func TestNormalizeTextPreservesSingleSpaces(t *testing.T) {
	const in = "Water, Landscape, and Critical Zone Processes"
	if got := normalizeText(in); got != in {
		t.Errorf("normalizeText(%q) = %q; single spaces must survive", in, got)
	}
}
