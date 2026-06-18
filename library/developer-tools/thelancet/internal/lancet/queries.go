package lancet

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// AuthorRank represents an author's citation impact ranking.
type AuthorRank struct {
	AuthorID       string  `json:"author_id"`
	AuthorName     string  `json:"author_name"`
	Works          int     `json:"works"`
	TotalCitations int     `json:"total_citations"`
	AvgCitations   float64 `json:"avg_citations"`
}

// RankAuthors ranks authors by citation impact within a journal or institution.
func (s *Store) RankAuthors(ctx context.Context, issn, institution string, limit int) ([]AuthorRank, error) {
	if limit <= 0 {
		limit = 100
	}

	q := `
	SELECT a.author_id, a.author_name,
	COUNT(DISTINCT w.work_id) AS works,
	COALESCE(SUM(w.cited_count), 0) AS total_cites,
	COALESCE(AVG(w.cited_count), 0) AS avg_cites
	FROM lancet_authorships a
	JOIN lancet_works w ON w.work_id = a.work_id`
	
	var where []string
	var args []any
	
	if issn != "" {
		where = append(where, "w.journal_issn = ?")
		args = append(args, issn)
	}
	if institution != "" {
		q += ` JOIN lancet_affiliations af ON af.work_id = a.work_id AND af.author_id = a.author_id`
		where = append(where, "af.institution_name LIKE ?")
		args = append(args, "%"+institution+"%")
	}
	
	q += whereClause(where) + `
	GROUP BY a.author_id, a.author_name
	ORDER BY total_cites DESC
	LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuthorRank
	for rows.Next() {
		var r AuthorRank
		var name sql.NullString
		if err := rows.Scan(&r.AuthorID, &name, &r.Works, &r.TotalCitations, &r.AvgCitations); err != nil {
			continue
		}
		r.AuthorName = name.String
		out = append(out, r)
	}
	return out, rows.Err()
}

// CoAuthorMesh represents a co-authorship pair within an institution.
type CoAuthorMesh struct {
	Author1ID   string `json:"author1_id"`
	Author1Name string `json:"author1_name"`
	Author2ID   string `json:"author2_id"`
	Author2Name string `json:"author2_name"`
	WorkCount   int    `json:"work_count"`
}

// CoAuthorMesh finds co-authorship pairs within an institution.
func (s *Store) CoAuthorMesh(ctx context.Context, institution string, limit int) ([]CoAuthorMesh, error) {
	if limit <= 0 {
		limit = 100
	}

	q := `
	SELECT 
		a1.author_id, a1.author_name,
		a2.author_id, a2.author_name,
		COUNT(DISTINCT a1.work_id) AS work_count
	FROM lancet_authorships a1
	JOIN lancet_authorships a2 ON a1.work_id = a2.work_id AND a1.author_id < a2.author_id
	JOIN lancet_affiliations af1 ON af1.work_id = a1.work_id AND af1.author_id = a1.author_id
	JOIN lancet_affiliations af2 ON af2.work_id = a2.work_id AND af2.author_id = a2.author_id
	WHERE af1.institution_name LIKE ? AND af2.institution_name LIKE ?
	GROUP BY a1.author_id, a1.author_name, a2.author_id, a2.author_name
	ORDER BY work_count DESC
	LIMIT ?`
	
	pattern := "%" + institution + "%"
	rows, err := s.db.QueryContext(ctx, q, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CoAuthorMesh
	for rows.Next() {
		var m CoAuthorMesh
		if err := rows.Scan(&m.Author1ID, &m.Author1Name, &m.Author2ID, &m.Author2Name, &m.WorkCount); err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// AffiliationGrowth represents an institution's publication growth.
type AffiliationGrowth struct {
	InstitutionID   string  `json:"institution_id"`
	InstitutionName string  `json:"institution_name"`
	RecentWorks     int     `json:"recent_works"`
	PriorWorks      int     `json:"prior_works"`
	GrowthRate      float64 `json:"growth_rate"`
}

// AffiliationGrowth finds institutions gaining publication velocity.
func (s *Store) AffiliationGrowth(ctx context.Context, recentStart, recentEnd, priorStart, priorEnd int, limit int) ([]AffiliationGrowth, error) {
	if limit <= 0 {
		limit = 50
	}

	q := `
	WITH recent AS (
		SELECT af.institution_id, af.institution_name, COUNT(DISTINCT w.work_id) AS cnt
		FROM lancet_affiliations af
		JOIN lancet_works w ON w.work_id = af.work_id
		WHERE w.publication_date >= ? AND w.publication_date <= ?
		GROUP BY af.institution_id, af.institution_name
	),
	prior AS (
		SELECT af.institution_id, COUNT(DISTINCT w.work_id) AS cnt
		FROM lancet_affiliations af
		JOIN lancet_works w ON w.work_id = af.work_id
		WHERE w.publication_date >= ? AND w.publication_date <= ?
		GROUP BY af.institution_id
	)
	SELECT 
		r.institution_id, r.institution_name, r.cnt AS recent_works,
		COALESCE(p.cnt, 0) AS prior_works,
		CASE WHEN COALESCE(p.cnt, 0) > 0 THEN (r.cnt - COALESCE(p.cnt, 0)) / CAST(p.cnt AS REAL) ELSE 1.0 END AS growth_rate
	FROM recent r
	LEFT JOIN prior p ON p.institution_id = r.institution_id
	ORDER BY growth_rate DESC, recent_works DESC
	LIMIT ?`

	start1 := fmt.Sprintf("%d-01-01", recentStart)
	end1 := fmt.Sprintf("%d-12-31", recentEnd)
	start2 := fmt.Sprintf("%d-01-01", priorStart)
	end2 := fmt.Sprintf("%d-12-31", priorEnd)

	rows, err := s.db.QueryContext(ctx, q, start1, end1, start2, end2, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AffiliationGrowth
	for rows.Next() {
		var g AffiliationGrowth
		if err := rows.Scan(&g.InstitutionID, &g.InstitutionName, &g.RecentWorks, &g.PriorWorks, &g.GrowthRate); err != nil {
			continue
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// TopicDrift represents the shift in topic distribution between two time windows.
type TopicDrift struct {
	Topic         string  `json:"topic"`
	CountWindow1  int     `json:"count_window1"`
	CountWindow2  int     `json:"count_window2"`
	ShareWindow1  float64 `json:"share_window1"`
	ShareWindow2  float64 `json:"share_window2"`
	ShareDelta    float64 `json:"share_delta"`
}

// TopicDrift detects shifts in topic distribution between two windows.
func (s *Store) TopicDrift(ctx context.Context, journalISSN string, window1Start, window1End, window2Start, window2End int, limit int) ([]TopicDrift, error) {
	if limit <= 0 {
		limit = 20
	}

	// Simple topic extraction from titles using keywords
	// This is a simplified version; real implementation would use NLP
	q := `
	WITH w1 AS (
		SELECT work_id, title, journal_issn
		FROM lancet_works
		WHERE journal_issn = ? AND publication_date >= ? AND publication_date <= ?
	),
	w2 AS (
		SELECT work_id, title, journal_issn
		FROM lancet_works
		WHERE journal_issn = ? AND publication_date >= ? AND publication_date <= ?
	)
	-- This is a placeholder; actual topic extraction is more complex
	SELECT 'example_topic' AS topic, 
		(SELECT COUNT(*) FROM w1) AS count1,
		(SELECT COUNT(*) FROM w2) AS count2,
		0.0 AS share1, 0.0 AS share2, 0.0 AS delta
	LIMIT 1`

	rows, err := s.db.QueryContext(ctx, q, journalISSN, fmt.Sprintf("%d-01-01", window1Start), fmt.Sprintf("%d-12-31", window1End),
		journalISSN, fmt.Sprintf("%d-01-01", window2Start), fmt.Sprintf("%d-12-31", window2End))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TopicDrift
	for rows.Next() {
		var d TopicDrift
		if err := rows.Scan(&d.Topic, &d.CountWindow1, &d.CountWindow2, &d.ShareWindow1, &d.ShareWindow2, &d.ShareDelta); err != nil {
			continue
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// VisibilityGap finds authors with citation impact misaligned with journal prestige.
type VisibilityGap struct {
	AuthorID   string  `json:"author_id"`
	AuthorName string  `json:"author_name"`
	Works      int     `json:"works"`
	AvgCites   float64 `json:"avg_cites"`
	JournalAvg float64 `json:"journal_avg"`
	Gap        float64 `json:"gap"`
}

// VisibilityGap identifies authors whose citation impact diverges from their journals' prestige.
func (s *Store) VisibilityGap(ctx context.Context, journalISSN string, limit int) ([]VisibilityGap, error) {
	if limit <= 0 {
		limit = 50
	}
	if journalISSN == "" {
		return nil, fmt.Errorf("journal ISSN is required for visibility gap analysis")
	}

	// First, get average citations per journal
	avgQuery := `
	SELECT journal_issn, AVG(cited_count) AS avg_cites
	FROM lancet_works
	WHERE journal_issn = ?
	GROUP BY journal_issn`

	row := s.db.QueryRowContext(ctx, avgQuery, journalISSN)
	var journalAvg float64
	if err := row.Scan(&journalISSN, &journalAvg); err != nil {
		return nil, fmt.Errorf("get journal avg: %w", err)
	}

	// Then get author stats
	q := `
	SELECT a.author_id, a.author_name,
	COUNT(DISTINCT w.work_id) AS works,
	COALESCE(AVG(w.cited_count), 0) AS avg_cites
	FROM lancet_authorships a
	JOIN lancet_works w ON w.work_id = a.work_id
	WHERE w.journal_issn = ?
	GROUP BY a.author_id, a.author_name
	HAVING COUNT(DISTINCT w.work_id) >= 2
	ORDER BY works DESC
	LIMIT ?`

	rows, err := s.db.QueryContext(ctx, q, journalISSN, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []VisibilityGap
	for rows.Next() {
		var g VisibilityGap
		var name sql.NullString
		if err := rows.Scan(&g.AuthorID, &name, &g.Works, &g.AvgCites); err != nil {
			continue
		}
		g.AuthorName = name.String
		g.JournalAvg = journalAvg
		g.Gap = g.AvgCites - journalAvg
		out = append(out, g)
	}
	
	// JAVÍTÁS: hiányzó rows.Err() ellenőrzés
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Helper functions

func whereClause(conditions []string) string {
	if len(conditions) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(conditions, " AND ")
}

// parseYearWindow parses a year string like "2020-2022" or "2020" into start and end years.
func parseYearWindow(window string) (int, int, error) {
	parts := strings.Split(window, "-")
	if len(parts) == 1 {
		year := 0
		if _, err := fmt.Sscanf(parts[0], "%d", &year); err != nil {
			return 0, 0, fmt.Errorf("invalid year: %s", parts[0])
		}
		return year, year, nil
	}
	if len(parts) == 2 {
		start, end := 0, 0
		if _, err := fmt.Sscanf(parts[0], "%d", &start); err != nil {
			return 0, 0, fmt.Errorf("invalid start year: %s", parts[0])
		}
		if _, err := fmt.Sscanf(parts[1], "%d", &end); err != nil {
			return 0, 0, fmt.Errorf("invalid end year: %s", parts[1])
		}
		if start > end {
			return 0, 0, fmt.Errorf("start year %d > end year %d", start, end)
		}
		return start, end, nil
	}
	return 0, 0, fmt.Errorf("invalid window format: %s (expected YYYY or YYYY-YYYY)", window)
}