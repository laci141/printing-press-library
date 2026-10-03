package lancet

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"
)

// The word-match index is an external-content FTS5 table over lancet_works
// (title, topic) using the same tokenizer as the generated store's
// resources_fts. External-content keeps a single copy of the text (the real DB
// is large) and lets 'rebuild' backfill from the base table; the triggers
// below keep it in sync. A contentless table would need the same triggers plus
// a stored copy of the old values for deletes, with no storage benefit here.
const ftsTable = "lancet_works_fts"

func ensureWorksFTS(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, ftsTable).Scan(&n); err != nil {
		return fmt.Errorf("lancet fts: %w", err)
	}
	created := n == 0
	stmts := []string{
		`CREATE VIRTUAL TABLE IF NOT EXISTS lancet_works_fts USING fts5(
			title, topic, content='lancet_works', content_rowid='rowid', tokenize='porter unicode61')`,
		`CREATE TRIGGER IF NOT EXISTS lancet_works_fts_ai AFTER INSERT ON lancet_works BEGIN
			INSERT INTO lancet_works_fts(rowid, title, topic) VALUES (new.rowid, new.title, new.topic);
		END`,
		`CREATE TRIGGER IF NOT EXISTS lancet_works_fts_ad AFTER DELETE ON lancet_works BEGIN
			INSERT INTO lancet_works_fts(lancet_works_fts, rowid, title, topic) VALUES ('delete', old.rowid, old.title, old.topic);
		END`,
		`CREATE TRIGGER IF NOT EXISTS lancet_works_fts_au AFTER UPDATE ON lancet_works BEGIN
			INSERT INTO lancet_works_fts(lancet_works_fts, rowid, title, topic) VALUES ('delete', old.rowid, old.title, old.topic);
			INSERT INTO lancet_works_fts(rowid, title, topic) VALUES (new.rowid, new.title, new.topic);
		END`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("lancet fts: %w (FTS5 is required for curate word matching)", err)
		}
	}
	if created {
		// One-time backfill of pre-existing rows; later opens skip it.
		if _, err := db.ExecContext(ctx, `INSERT INTO lancet_works_fts(lancet_works_fts) VALUES ('rebuild')`); err != nil {
			return fmt.Errorf("lancet fts backfill: %w", err)
		}
	}
	return nil
}

// ftsMatchQuery turns free text into an FTS5 MATCH expression: every
// whitespace-separated token becomes a double-quoted phrase (embedded quotes
// doubled), joined by implicit AND. Tokens without a letter or digit carry no
// searchable word and are dropped. Returns "" when nothing is left.
func ftsMatchQuery(topic string) string {
	var parts []string
	for _, tok := range strings.Fields(topic) {
		if !strings.ContainsFunc(tok, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(tok, `"`, `""`)+`"`)
	}
	return strings.Join(parts, " ")
}
