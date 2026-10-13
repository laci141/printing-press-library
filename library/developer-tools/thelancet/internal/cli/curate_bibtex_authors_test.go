// Hand-authored coverage for the BibTeX author field of curate. Not generated.

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/thelancet/internal/lancet"
)

func TestRenderCurateBibtexAuthors(t *testing.T) {
	rows := []lancet.WorkRow{
		{Title: "Work A", Journal: "The Lancet", Year: 2020, DOI: "10.1/a", Authors: []string{"N0", "N1", "N2"}},
		{Title: "Work B", Journal: "The Lancet", Year: 2021, DOI: "10.1/b", Authors: []string{}},
	}
	cmd := newNovelCurateCmd(&rootFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := renderCurateBibtex(cmd, rows); err != nil {
		t.Fatal(err)
	}
	entries := strings.Split(strings.TrimSpace(out.String()), "\n\n")
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2:\n%s", len(entries), out.String())
	}
	if !strings.Contains(entries[0], "  author  = {N0 and N1 and N2},\n") {
		t.Errorf("entry A lacks the author line:\n%s", entries[0])
	}
	if !strings.Contains(entries[0], "title   = {Work A},\n  author  =") {
		t.Errorf("author line is not right after the title line:\n%s", entries[0])
	}
	if strings.Contains(entries[1], "author") {
		t.Errorf("entry B has an author line:\n%s", entries[1])
	}
}

func TestCurateJSONEmitsEmptyAuthorsForLiveRows(t *testing.T) {
	// Live rows (CurateLive does not select authorships) arrive with nil Authors.
	_, _, out, _ := runCurateStubbed(t, "live", nil, []lancet.WorkRow{{Title: "Live paper", Year: 2024, Cited: 5}})
	if !strings.Contains(out, `"authors": []`) && !strings.Contains(out, `"authors":[]`) {
		t.Fatalf("stdout lacks an empty authors array:\n%s", out)
	}
}
