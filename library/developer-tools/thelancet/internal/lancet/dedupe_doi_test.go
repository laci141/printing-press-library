package lancet

import (
	"context"
	"database/sql"
	"testing"
)

func dedupeWork(id, doi, title string) decodedWork {
	return decodedWork{
		ID: id, DOI: doi, Title: title, Year: 2024, Date: "2024-01-01", Cited: 1,
		Authors: []decodedAuthor{{ID: "A" + id, Name: "Author " + id,
			Institutions: []decodedInstitution{{ID: "I" + id, Name: "Inst " + id, Country: "GB"}}}},
		YearCounts:   []yearCount{{Year: 2024, Cited: 1}},
		CountsSynced: true,
	}
}

func countWhere(t *testing.T, db *sql.DB, table, workID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE work_id = ?`, workID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func childRows(t *testing.T, db *sql.DB, workID string) int {
	t.Helper()
	return countWhere(t, db, "lancet_authorships", workID) +
		countWhere(t, db, "lancet_affiliations", workID) +
		countWhere(t, db, "lancet_work_year_counts", workID)
}

func store(t *testing.T, db *sql.DB, works ...decodedWork) {
	t.Helper()
	if _, err := StoreWorks(context.Background(), db, works, "0140-6736", "The Lancet"); err != nil {
		t.Fatal(err)
	}
}

func curateAll(t *testing.T, db *sql.DB, topic string) []WorkRow {
	t.Helper()
	rows, err := Curate(context.Background(), db, topic, "", "citations", false, 100)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestDedupeDOIReplacesOldWorkIDCaseInsensitive(t *testing.T) {
	db := wordDB(t)
	store(t, db, dedupeWork("W1", "10.1016/ABC", "Ivermectin old"))
	if childRows(t, db, "W1") != 3 {
		t.Fatalf("setup: W1 child rows = %d, want 3", childRows(t, db, "W1"))
	}
	store(t, db, dedupeWork("W2", "10.1016/abc", "Ivermectin new"))
	if n := countWhere(t, db, "lancet_works", "W1"); n != 0 {
		t.Errorf("W1 work row still present (%d)", n)
	}
	if n := childRows(t, db, "W1"); n != 0 {
		t.Errorf("W1 child rows left: %d", n)
	}
	if n := countWhere(t, db, "lancet_works", "W2"); n != 1 {
		t.Errorf("W2 missing (%d)", n)
	}
	if n := childRows(t, db, "W2"); n != 3 {
		t.Errorf("W2 child rows = %d, want 3", n)
	}
	rows := curateAll(t, db, "ivermectin")
	if len(rows) != 1 || rows[0].Title != "Ivermectin new" {
		t.Errorf("curate(ivermectin) = %+v, want exactly the new row", rows)
	}
	// FTS must not retain the old text either.
	if rows := curateAll(t, db, "old"); len(rows) != 0 {
		t.Errorf("curate(old) = %+v, stale FTS entry", rows)
	}
}

func TestDedupeDOIKeepsWorksWithoutDOI(t *testing.T) {
	db := wordDB(t)
	store(t, db, dedupeWork("N1", "", "Nodoi one"), dedupeWork("N2", "", "Nodoi two"))
	store(t, db, dedupeWork("N3", "", "Nodoi three"))
	for _, id := range []string{"N1", "N2", "N3"} {
		if countWhere(t, db, "lancet_works", id) != 1 || childRows(t, db, id) != 3 {
			t.Errorf("%s was touched", id)
		}
	}
	if rows := curateAll(t, db, "nodoi"); len(rows) != 3 {
		t.Errorf("curate(nodoi) = %d rows, want 3", len(rows))
	}
}

func TestDedupeDOIDifferentDOIsBothStay(t *testing.T) {
	db := wordDB(t)
	store(t, db, dedupeWork("D1", "10.1/aaa", "Distinct alpha"))
	store(t, db, dedupeWork("D2", "10.1/bbb", "Distinct beta"))
	for _, id := range []string{"D1", "D2"} {
		if countWhere(t, db, "lancet_works", id) != 1 || childRows(t, db, id) != 3 {
			t.Errorf("%s was touched", id)
		}
	}
	if rows := curateAll(t, db, "distinct"); len(rows) != 2 {
		t.Errorf("curate(distinct) = %d rows, want 2", len(rows))
	}
}

// Rule: when two works in one batch share a DOI, the one stored last wins.
func TestDedupeDOISameBatchLastWins(t *testing.T) {
	db := wordDB(t)
	store(t, db,
		dedupeWork("B1", "10.1/Same", "Batch first"),
		dedupeWork("B2", "10.1/same", "Batch last"))
	if countWhere(t, db, "lancet_works", "B1") != 0 || childRows(t, db, "B1") != 0 {
		t.Errorf("B1 should be gone")
	}
	if countWhere(t, db, "lancet_works", "B2") != 1 || childRows(t, db, "B2") != 3 {
		t.Errorf("B2 should remain with children")
	}
	if rows := curateAll(t, db, "batch"); len(rows) != 1 || rows[0].Title != "Batch last" {
		t.Errorf("curate(batch) = %+v", rows)
	}
}
