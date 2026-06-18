package lancet

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store manages the SQLite database for Lancet analytics.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates the SQLite database at the given path.
func NewStore(path string) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}
	s := &Store{db: db}
	if err := s.EnsureSchema(); err != nil {
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return s, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database handle.
func (s *Store) DB() *sql.DB {
	return s.db
}

// EnsureSchema creates the necessary tables if they don't exist.
func (s *Store) EnsureSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS lancet_works (
		work_id TEXT PRIMARY KEY,
		title TEXT,
		doi TEXT,
		publication_date TEXT,
		journal_issn TEXT,
		journal_name TEXT,
		cited_count INTEGER DEFAULT 0,
		openalex_url TEXT,
		updated_at TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_works_journal_issn ON lancet_works(journal_issn);
	CREATE INDEX IF NOT EXISTS idx_works_publication_date ON lancet_works(publication_date);

	CREATE TABLE IF NOT EXISTS lancet_authorships (
		work_id TEXT,
		author_id TEXT,
		author_name TEXT,
		author_position INTEGER,
		PRIMARY KEY (work_id, author_id),
		FOREIGN KEY (work_id) REFERENCES lancet_works(work_id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_authorships_author_id ON lancet_authorships(author_id);
	CREATE INDEX IF NOT EXISTS idx_authorships_work_id ON lancet_authorships(work_id);

	CREATE TABLE IF NOT EXISTS lancet_affiliations (
		work_id TEXT,
		author_id TEXT,
		institution_id TEXT,
		institution_name TEXT,
		institution_country TEXT,
		UNIQUE(work_id, author_id, institution_id),
		FOREIGN KEY (work_id) REFERENCES lancet_works(work_id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_affiliations_institution_name ON lancet_affiliations(institution_name);
	CREATE INDEX IF NOT EXISTS idx_affiliations_work_id ON lancet_affiliations(work_id);

	CREATE TABLE IF NOT EXISTS lancet_sync_metadata (
		key TEXT PRIMARY KEY,
		value TEXT
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Work represents a Lancet article from OpenAlex.
type Work struct {
	ID              string
	Title           string
	DOI             string
	PublicationDate string
	JournalISSN     string
	JournalName     string
	CitedCount      int
	OpenAlexURL     string
	Authorships     []Authorship
}

// Authorship represents an author's contribution to a work.
type Authorship struct {
	AuthorID   string
	AuthorName string
	Position   int
	Affiliation
}

// Affiliation represents an institution affiliation for an authorship.
type Affiliation struct {
	InstitutionID      string
	InstitutionName    string
	InstitutionCountry string
}

// StoreWorks stores or updates a batch of works in the database.
func (s *Store) StoreWorks(works []Work) error {
	if len(works) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Prepare statements
	workStmt, err := tx.Prepare(`
	INSERT OR REPLACE INTO lancet_works 
	(work_id, title, doi, publication_date, journal_issn, journal_name, cited_count, openalex_url, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare work stmt: %w", err)
	}
	defer workStmt.Close()

	authStmt, err := tx.Prepare(`
	INSERT OR REPLACE INTO lancet_authorships (work_id, author_id, author_name, author_position)
	VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare authorship stmt: %w", err)
	}
	defer authStmt.Close()

	affilStmt, err := tx.Prepare(`
	INSERT OR IGNORE INTO lancet_affiliations (work_id, author_id, institution_id, institution_name, institution_country)
	VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("prepare affiliation stmt: %w", err)
	}
	defer affilStmt.Close()

	now := time.Now().Format(time.RFC3339)

	for _, w := range works {
		// Insert work
		if _, err := workStmt.Exec(
			w.ID, w.Title, w.DOI, w.PublicationDate,
			w.JournalISSN, w.JournalName, w.CitedCount,
			w.OpenAlexURL, now,
		); err != nil {
			return fmt.Errorf("insert work %s: %w", w.ID, err)
		}

		// Insert authorships and affiliations
		for _, a := range w.Authorships {
			if _, err := authStmt.Exec(w.ID, a.AuthorID, a.AuthorName, a.Position); err != nil {
				return fmt.Errorf("insert authorship %s/%s: %w", w.ID, a.AuthorID, err)
			}
			if a.InstitutionID != "" || a.InstitutionName != "" {
				if _, err := affilStmt.Exec(
					w.ID, a.AuthorID,
					a.InstitutionID, a.InstitutionName, a.InstitutionCountry,
				); err != nil {
					return fmt.Errorf("insert affiliation %s/%s: %w", w.ID, a.AuthorID, err)
				}
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// WorkCount returns the total number of works in the store.
func (s *Store) WorkCount() (int, error) {
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM lancet_works").Scan(&count)
	return count, err
}

// IsEmpty returns true if the store has no works.
func (s *Store) IsEmpty() (bool, error) {
	count, err := s.WorkCount()
	return count == 0, err
}

// DeleteAll removes all data from the store.
func (s *Store) DeleteAll() error {
	_, err := s.db.Exec("DELETE FROM lancet_works")
	return err
}