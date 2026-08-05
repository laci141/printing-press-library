package scengine

import "testing"

// TestVerdictWeakDirectionBand covers the 0.15..0.40 band in both directions,
// which no archived corpus reaches: all 38 archived scores are 0.62 or stronger,
// the narrowest being -0.62 and 0.65. The evidence has to be synthetic because
// the corpora cannot exercise this path at all.
func TestVerdictWeakDirectionBand(t *testing.T) {
	cases := []struct {
		name        string
		score       float64
		supporting  int
		refuting    int
		mixed       int
		studyCount  int
		wantVerdict Verdict
	}{
		{
			name:        "live vitamin C shape, 0.27",
			score:       0.27,
			supporting:  4,
			refuting:    1,
			mixed:       1,
			studyCount:  8,
			wantVerdict: VerdictMixed,
		},
		{
			name:        "same shape mirrored negative",
			score:       -0.27,
			supporting:  1,
			refuting:    4,
			mixed:       1,
			studyCount:  8,
			wantVerdict: VerdictMixed,
		},
		{
			name:        "just inside the weak band, positive",
			score:       0.15,
			supporting:  3,
			refuting:    2,
			studyCount:  6,
			wantVerdict: VerdictMixed,
		},
		{
			name:        "just inside the weak band, negative",
			score:       -0.15,
			supporting:  2,
			refuting:    3,
			studyCount:  6,
			wantVerdict: VerdictMixed,
		},
		{
			name:        "just below the weak band stays inconclusive",
			score:       0.14,
			supporting:  3,
			refuting:    3,
			studyCount:  6,
			wantVerdict: VerdictInconclusive,
		},
		{
			name:        "the strong threshold still supports at exactly 0.40",
			score:       0.4,
			supporting:  5,
			refuting:    1,
			studyCount:  6,
			wantVerdict: VerdictSupports,
		},
		{
			name:        "the strong threshold still refutes at exactly -0.40",
			score:       -0.4,
			supporting:  1,
			refuting:    5,
			studyCount:  6,
			wantVerdict: VerdictRefutes,
		},
		{
			name:        "a strong corpus is untouched",
			score:       0.95,
			supporting:  9,
			refuting:    0,
			studyCount:  9,
			wantVerdict: VerdictSupports,
		},
		{
			name:        "too few studies still yields insufficient",
			score:       0.27,
			supporting:  2,
			refuting:    0,
			studyCount:  2,
			wantVerdict: VerdictInsufficient,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := ConsensusResult{
				ConsensusScore: tc.score,
				Supporting:     tc.supporting,
				Refuting:       tc.refuting,
				Mixed:          tc.mixed,
				StudyCount:     tc.studyCount,
			}
			got := verdict(r, tc.supporting+tc.refuting)
			if got != tc.wantVerdict {
				t.Errorf("verdict(score=%.2f, sup=%d, ref=%d, mixed=%d, n=%d) = %q, want %q",
					tc.score, tc.supporting, tc.refuting, tc.mixed, tc.studyCount, got, tc.wantVerdict)
			}
			if r.ConsensusScore != tc.score {
				t.Errorf("verdict must not mutate ConsensusScore: got %v, want %v", r.ConsensusScore, tc.score)
			}
		})
	}
}
