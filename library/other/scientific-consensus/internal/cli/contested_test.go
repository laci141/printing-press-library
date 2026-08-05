package cli

import "testing"

// contestedFor mirrors the rule in the controversies command so the thresholds
// can be pinned without a live API call.
func contestedFor(supporting, refuting int) (score float64, contested bool) {
	directional := supporting + refuting
	minor := supporting
	if refuting < minor {
		minor = refuting
	}
	if directional > 0 {
		score = round2f(2 * float64(minor) / float64(directional))
	}
	return score, score >= 0.5 && directional >= 4 && minor >= 2
}

// TestContestedNeedsTwoOnTheMinoritySide pins the three thresholds against the
// live measurements of 2026-08-05, limit 10, keyless.
func TestContestedNeedsTwoOnTheMinoritySide(t *testing.T) {
	cases := []struct {
		claim      string
		supporting int
		refuting   int
		wantScore  float64
		want       bool
		why        string
	}{
		{
			claim: "HPV vaccine causes infertility", supporting: 1, refuting: 3,
			wantScore: 0.5, want: false,
			why: "both old thresholds met at their exact boundary by one work, and that work was excluded as off-topic by the AI filter in the same run",
		},
		{
			claim: "cell phones cause brain cancer", supporting: 2, refuting: 3,
			wantScore: 0.8, want: true,
			why: "genuinely disputed, and the minority is two",
		},
		{
			claim: "aspartame causes cancer", supporting: 3, refuting: 2,
			wantScore: 0.8, want: true,
			why: "disputed with the majority on the other side",
		},
		{
			claim: "red meat causes colon cancer", supporting: 4, refuting: 4,
			wantScore: 1.0, want: true,
			why: "an even split is the clearest controversy there is",
		},
		{
			claim: "vaccines cause autism", supporting: 1, refuting: 6,
			wantScore: 0.29, want: false,
			why: "already false on score alone",
		},
		{
			claim: "statins cause muscle damage", supporting: 1, refuting: 5,
			wantScore: 0.33, want: false,
			why: "already false on score alone",
		},
		{
			claim: "5G causes health problems", supporting: 0, refuting: 6,
			wantScore: 0, want: false,
			why: "one-sided",
		},
		{
			claim: "fluoride lowers IQ", supporting: 0, refuting: 0,
			wantScore: 0, want: false,
			why: "nothing directional at all; the note carries this, not the flag",
		},
	}
	for _, tc := range cases {
		t.Run(tc.claim, func(t *testing.T) {
			score, contested := contestedFor(tc.supporting, tc.refuting)
			if score != tc.wantScore {
				t.Errorf("score = %.2f, want %.2f", score, tc.wantScore)
			}
			if contested != tc.want {
				t.Errorf("contested = %t, want %t (%d supporting, %d refuting)\n  %s",
					contested, tc.want, tc.supporting, tc.refuting, tc.why)
			}
		})
	}
}

// TestContestedFloorIsTwoNotThree guards the choice of floor. redmeat is a real
// controversy at 5 supporting against 2 refuting, and a floor of three would
// erase it.
func TestContestedFloorIsTwoNotThree(t *testing.T) {
	_, contested := contestedFor(5, 2)
	if !contested {
		t.Fatal("redmeat shape (5/2) must stay contested; the minority floor is two, not three")
	}
}
