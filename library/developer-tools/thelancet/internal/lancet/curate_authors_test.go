// Hand-authored coverage for per-work authors on curate rows. Not generated.

package lancet

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

// authorsWant is seq order. Names, author_ids and insertion order are all chosen
// to differ from it, so only ORDER BY seq can produce it.
var authorsWant = []string{"Zed N0", "Mia N1", "Abe N2"}

// authorsDB seeds work A (three authors inserted out of seq order, plus a
// NULL-name row) and work B (no authorship rows) through EnsureSchema.
func authorsDB(t *testing.T) *sql.DB {
	t.Helper()
	db := wordDB(t)
	ctx := context.Background()
	year := time.Now().Year() - 5
	synced := time.Now().UTC().Format("2006-01-02")
	for _, w := range []struct {
		id, title string
		cited     int
	}{{"WA", "Alpha authors study", 20}, {"WB", "Beta authors study", 10}} {
		if _, err := db.ExecContext(ctx, `INSERT INTO lancet_works
			(work_id, doi, title, journal_issn, journal_name, pub_year, pub_date, cited_count, is_oa, topic, counts_synced_at)
			VALUES (?, ?, ?, '0140-6736', 'The Lancet', ?, ?, ?, 0, 'T', ?)`,
			w.id, "10.1/"+strings.ToLower(w.id), w.title, year, fmt.Sprintf("%d-03-01", year), w.cited, synced); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO lancet_work_year_counts (work_id, year, cited_by_count) VALUES (?, ?, 5)`,
			w.id, time.Now().Year()-1); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range []struct {
		id, name any
		seq      int
	}{
		{"C", "Abe N2", 2}, {"B", "Zed N0", 0}, {"A", "Mia N1", 1}, {"D", nil, 3},
	} {
		if _, err := db.ExecContext(ctx, `INSERT INTO lancet_authorships (work_id, author_id, author_name, seq) VALUES ('WA', ?, ?, ?)`,
			a.id, a.name, a.seq); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func authorsByTitle(t *testing.T, db *sql.DB, sortBy string) map[string]WorkRow {
	t.Helper()
	rows, err := Curate(context.Background(), db, "authors", "", sortBy, false, 10)
	if err != nil {
		t.Fatalf("Curate(%s): %v", sortBy, err)
	}
	out := map[string]WorkRow{}
	for _, r := range rows {
		out[r.Title] = r
	}
	if len(out) != 2 {
		t.Fatalf("Curate(%s) returned %d works, want 2", sortBy, len(out))
	}
	return out
}

func TestCurateFillsAuthorsOrderedBySeq(t *testing.T) {
	got := authorsByTitle(t, authorsDB(t), "citations")
	a, b := got["Alpha authors study"], got["Beta authors study"]
	if !reflect.DeepEqual(a.Authors, authorsWant) {
		t.Fatalf("A.Authors = %#v, want %#v", a.Authors, authorsWant)
	}
	if b.Authors == nil || len(b.Authors) != 0 {
		t.Fatalf("B.Authors = %#v, want non-nil and empty", b.Authors)
	}
}

func TestCurateEmptyAuthorsMarshalAsArray(t *testing.T) {
	b := authorsByTitle(t, authorsDB(t), "citations")["Beta authors study"]
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"authors":[]`) {
		t.Fatalf("JSON = %s, want it to contain \"authors\":[]", raw)
	}
}

func TestCurateAuthorsOnEverySort(t *testing.T) {
	db := authorsDB(t)
	for _, sortBy := range []string{"citations", "date", "per-year", "velocity"} {
		got := authorsByTitle(t, db, sortBy)
		if a := got["Alpha authors study"].Authors; !reflect.DeepEqual(a, authorsWant) {
			t.Errorf("%s: A.Authors = %#v, want %#v", sortBy, a, authorsWant)
		}
		if b := got["Beta authors study"].Authors; b == nil || len(b) != 0 {
			t.Errorf("%s: B.Authors = %#v, want non-nil and empty", sortBy, b)
		}
	}
}

// TestCurateAuthorsAreChunked lowers the chunk size to 2 and seeds 5 works, so
// the authors lookup needs 3 queries. Every work's authors must still come back
// exactly, in seq order. The query count proves the chunking: without it the
// results are identical, because SQLite accepts 5 variables.
func TestCurateAuthorsAreChunked(t *testing.T) {
	oldChunk, oldHook := authorsChunk, authorsQueryHook
	t.Cleanup(func() { authorsChunk, authorsQueryHook = oldChunk, oldHook })
	authorsChunk = 2
	queries := 0
	authorsQueryHook = func() { queries++ }

	db := wordDB(t)
	ctx := context.Background()
	year := time.Now().Year() - 5
	synced := time.Now().UTC().Format("2006-01-02")
	want := map[string][]string{}
	for i := 0; i < 5; i++ {
		id, title := fmt.Sprintf("W%d", i), fmt.Sprintf("Chunkword study %d", i)
		if _, err := db.ExecContext(ctx, `INSERT INTO lancet_works
			(work_id, doi, title, journal_issn, journal_name, pub_year, pub_date, cited_count, is_oa, topic, counts_synced_at)
			VALUES (?, ?, ?, '0140-6736', 'The Lancet', ?, ?, ?, 0, 'T', ?)`,
			id, "10.1/"+strings.ToLower(id), title, year, fmt.Sprintf("%d-03-01", year), 100-i, synced); err != nil {
			t.Fatal(err)
		}
		// Inserted out of seq order; author_ids differ from seq order too.
		for _, s := range []int{2, 0, 1} {
			name := fmt.Sprintf("A%d-%d", i, s)
			if _, err := db.ExecContext(ctx, `INSERT INTO lancet_authorships (work_id, author_id, author_name, seq) VALUES (?, ?, ?, ?)`,
				id, fmt.Sprintf("%s-%d", id, 2-s), name, s); err != nil {
				t.Fatal(err)
			}
		}
		want[title] = []string{fmt.Sprintf("A%d-0", i), fmt.Sprintf("A%d-1", i), fmt.Sprintf("A%d-2", i)}
	}
	rows, err := Curate(ctx, db, "chunkword", "", "citations", false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 {
		t.Fatalf("Curate returned %d works, want 5", len(rows))
	}
	for _, r := range rows {
		if !reflect.DeepEqual(r.Authors, want[r.Title]) {
			t.Errorf("%s: Authors = %#v, want %#v", r.Title, r.Authors, want[r.Title])
		}
	}
	if queries != 3 {
		t.Fatalf("authors queries = %d, want 3 (5 ids in chunks of 2)", queries)
	}
}
