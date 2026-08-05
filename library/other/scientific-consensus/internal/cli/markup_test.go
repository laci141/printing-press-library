package cli

import "testing"

func TestCleanMarkupText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "live HPV title measured 2026-08-05",
			in:   "<scp>HPV</scp>‐<scp>DNA</scp> sperm infection and infertility: from a systematic literature review to a possible clinical management proposal",
			want: "HPV‐DNA sperm infection and infertility: from a systematic literature review to a possible clinical management proposal",
		},
		{
			name: "italic species name",
			in:   "Ageing in <i>C. elegans</i>",
			want: "Ageing in C. elegans",
		},
		{
			name: "entities decoded",
			in:   "Vitamin D &amp; immunity",
			want: "Vitamin D & immunity",
		},
		{
			name: "double-escaped tag removed, not shown",
			in:   "&lt;i&gt;Escherichia coli&lt;/i&gt; in water",
			want: "Escherichia coli in water",
		},
		{
			name: "subscript and superscript",
			in:   "CO<sub>2</sub> and O<sup>2-</sup> levels",
			want: "CO2 and O2- levels",
		},
		{
			name: "less-than in statistics survives",
			in:   "Mortality reduced (p<0.05) in treated group",
			want: "Mortality reduced (p<0.05) in treated group",
		},
		{
			name: "greater-than in sample size survives",
			in:   "Cohort of n>100 patients",
			want: "Cohort of n>100 patients",
		},
		{
			name: "tag with attributes removed",
			in:   `<span class="italic">Helicobacter pylori</span> eradication`,
			want: "Helicobacter pylori eradication",
		},
		{
			name: "whitespace collapsed after removal",
			in:   "Vitamin <i> </i> D supplementation",
			want: "Vitamin D supplementation",
		},
		{
			name: "plain title untouched",
			in:   "Vitamin D supplementation to prevent acute respiratory tract infections",
			want: "Vitamin D supplementation to prevent acute respiratory tract infections",
		},
		{
			name: "empty stays empty",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanMarkupText(tc.in); got != tc.want {
				t.Errorf("cleanMarkupText(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}