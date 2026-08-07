package scengine

import "testing"

// TestConfidenceFromIsRoundedToTwoDecimals pins the exported shape of the
// counting path's confidence. Before rounding, the runtime division
// float64(dominant)/float64(total) leaked a float tail into the JSON Corpova
// writes to disk (dominant=1 total=1 gave 0.8999999999999999, dominant=5
// total=8 gave 0.7250000000000001). The first three cases collapse to the same
// value on purpose: any unanimous count earns the same confidence.
//
// Equality here is exact, not approximate — the point of the test is that the
// returned float64 is the one a two-decimal literal denotes.
func TestConfidenceFromIsRoundedToTwoDecimals(t *testing.T) {
	tests := []struct {
		name     string
		dominant int
		total    int
		want     float64
	}{
		{"unanimous single cue", 1, 1, 0.9},
		{"unanimous two cues", 2, 2, 0.9},
		{"unanimous three cues", 3, 3, 0.9},
		{"five of eight, volume bonus", 5, 8, 0.73},
		{"three of four, volume bonus", 3, 4, 0.8},
		{"two of three", 2, 3, 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := confidenceFrom(tt.dominant, tt.total)
			if got != tt.want {
				t.Errorf("confidenceFrom(%d, %d) = %v, want %v", tt.dominant, tt.total, got, tt.want)
			}
		})
	}
}

// TestRefutationConfidenceMatchesSingleDominantCue makes the claim on
// refutationConfidence's declaration checkable: it is documented as matching
// what a single dominant cue earns from confidenceFrom, which was true in
// intent and false in bytes until the counting path was rounded.
func TestRefutationConfidenceMatchesSingleDominantCue(t *testing.T) {
	if got := confidenceFrom(1, 1); got != refutationConfidence {
		t.Errorf("confidenceFrom(1, 1) = %v, want refutationConfidence %v", got, refutationConfidence)
	}
}
