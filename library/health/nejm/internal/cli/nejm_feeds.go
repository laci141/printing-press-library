// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-coded Phase 3: NEJM RSS feed parsing and article meta-tag extraction.

package cli

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/health/nejm/internal/client"
	"github.com/mvanhorn/printing-press-library/library/health/nejm/internal/store"
	xhtml "golang.org/x/net/html"
)

const (
	nejmEtocFeedURL   = "/action/showFeed?jc=nejm&type=etoc&feed=rss"
	nejmAxatocFeedURL = "/action/showFeed?jc=nejm&type=axatoc&feed=rss"
)

// nejmBaseURL is the origin RSS feeds and article pages are fetched from. It is
// a var rather than a const so tests can point it at an httptest server.
var nejmBaseURL = "https://www.nejm.org"

// nejmArticleRecord is the canonical shape stored in SQLite.
// The "id" field drives UpsertArticle's primary-key lookup.
type nejmArticleRecord struct {
	ID          string `json:"id"`
	DOI         string `json:"doi"`
	Title       string `json:"title"`
	Authors     string `json:"authors"`
	Abstract    string `json:"abstract"`
	ArticleType string `json:"article_type"`
	Specialties string `json:"specialties"`
	Date        string `json:"date"`
	IsFree      bool   `json:"is_free"`
	URL         string `json:"url"`
	Volume      string `json:"volume,omitempty"`
	Issue       string `json:"issue,omitempty"`
	Pages       string `json:"pages,omitempty"`
	CoverDate   string `json:"cover_date,omitempty"`
	Feed        string `json:"feed,omitempty"`
}

// --- RSS XML types ---

type nejmRSSFeed struct {
	XMLName xml.Name      `xml:"RDF"`
	Items   []nejmRSSItem `xml:"item"`
}

type nejmRSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Creator     string `xml:"creator"`      // dc:creator
	Date        string `xml:"date"`         // dc:date
	DOI         string `xml:"doi"`          // prism:doi
	URL         string `xml:"url"`          // prism:url
	Volume      string `xml:"volume"`       // prism:volume
	Number      string `xml:"number"`       // prism:number
	StartPage   string `xml:"startingPage"` // prism:startingPage
	EndPage     string `xml:"endingPage"`   // prism:endingPage
	CoverDate   string `xml:"coverDate"`    // prism:coverDate
	Description string `xml:"description"`  // bibliographic citation string
}

// nejmFetchFeed fetches and parses an NEJM RSS 1.0/RDF feed over plain HTTP.
// NEJM RSS endpoints return 200 without Cloudflare challenge.
func nejmFetchFeed(ctx context.Context, feedPath string) ([]nejmRSSItem, error) {
	url := nejmBaseURL + feedPath
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("building RSS request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; nejm-pp-cli/0.1)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching RSS feed %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RSS feed %s returned HTTP %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading RSS body: %w", err)
	}

	// NEJM RSS is RDF+PRISM with namespaced elements. xml.Unmarshal resolves
	// namespace prefixes only when they match the struct field names exactly or
	// via xml tags. Since the feed uses dc: and prism: prefixes we can't predict
	// the full namespace URI reliably across feed versions; instead we strip the
	// namespace prefix from each element name before parsing.
	cleaned := nejmStripPrefixesSimple(body)

	var feed nejmRSSFeed
	if err := xml.Unmarshal(cleaned, &feed); err != nil {
		return nil, fmt.Errorf("parsing RSS XML: %w", err)
	}
	return feed.Items, nil
}

// nejmStripPrefixesSimple does a clean pass replacing <prefix:tag and </prefix:tag
// with <tag and </tag, and removes xmlns:prefix="..." attributes.
func nejmStripPrefixesSimple(b []byte) []byte {
	s := string(b)
	// Remove namespace declarations like xmlns:dc="..." xmlns:prism="..."
	for _, ns := range []string{"dc", "prism", "rdf", "rdfs", "content", "taxo", "admin"} {
		s = strings.ReplaceAll(s, "xmlns:"+ns+"=", "xmlns_"+ns+"=")
		// Replace opening tags: <dc:foo → <foo, <prism:foo → <foo
		s = strings.ReplaceAll(s, "<"+ns+":", "<")
		// Replace closing tags: </dc:foo → </foo
		s = strings.ReplaceAll(s, "</"+ns+":", "</")
	}
	return []byte(s)
}

// nejmRSSItemToRecord converts a parsed RSS item to a store record.
func nejmRSSItemToRecord(item nejmRSSItem, feedName string) nejmArticleRecord {
	doi := strings.TrimSpace(item.DOI)
	if doi == "" {
		// Extract DOI from URL as fallback
		u := strings.TrimSpace(item.Link)
		if idx := strings.Index(u, "/doi/"); idx >= 0 {
			doi = strings.TrimPrefix(u[idx:], "/doi/full/")
			doi = strings.TrimPrefix(doi, "/doi/abs/")
			doi = strings.TrimPrefix(doi, "/doi/")
		}
	}

	articleURL := strings.TrimSpace(item.URL)
	if articleURL == "" {
		articleURL = strings.TrimSpace(item.Link)
	}
	if articleURL != "" && !strings.HasPrefix(articleURL, "http") {
		articleURL = nejmBaseURL + articleURL
	}

	pages := ""
	if item.StartPage != "" {
		pages = item.StartPage
		if item.EndPage != "" {
			pages += "-" + item.EndPage
		}
	}

	date := strings.TrimSpace(item.Date)
	if date == "" {
		date = strings.TrimSpace(item.CoverDate)
	}

	return nejmArticleRecord{
		ID:        doi,
		DOI:       doi,
		Title:     strings.TrimSpace(item.Title),
		Authors:   strings.TrimSpace(item.Creator),
		Date:      date,
		URL:       articleURL,
		Volume:    strings.TrimSpace(item.Volume),
		Issue:     strings.TrimSpace(item.Number),
		Pages:     pages,
		CoverDate: strings.TrimSpace(item.CoverDate),
		Feed:      feedName,
	}
}

// nejmSyncFeed fetches one NEJM RSS feed and upserts all articles into the store.
// Returns the count of newly inserted/updated records.
func nejmSyncFeed(ctx context.Context, db *store.Store, feedPath, feedName string, w io.Writer) (int, error) {
	items, err := nejmFetchFeed(ctx, feedPath)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, item := range items {
		rec := nejmRSSItemToRecord(item, feedName)
		if rec.DOI == "" {
			continue
		}
		data, err := json.Marshal(rec)
		if err != nil {
			continue
		}
		if err := db.UpsertArticle(json.RawMessage(data)); err != nil {
			if w != nil {
				fmt.Fprintf(w, "warning: upsert %s: %v\n", rec.DOI, err)
			}
			continue
		}
		// The generated upsert does not write the NEJM-specific feed column, so
		// set it explicitly here. This keeps the column authoritative even when
		// an article is re-synced (e.g. moves between feeds).
		if _, err := db.DB().ExecContext(ctx,
			`UPDATE "article" SET "feed" = ? WHERE "doi" = ?`, feedName, rec.DOI); err != nil {
			if w != nil {
				fmt.Fprintf(w, "warning: set feed for %s: %v\n", rec.DOI, err)
			}
		}
		count++
	}
	return count, nil
}

// nejmSyncAllFeeds syncs both eTOC and recently-published feeds.
func nejmSyncAllFeeds(ctx context.Context, db *store.Store, w io.Writer) (int, error) {
	// Ensure the feed column exists before per-row UPDATEs in nejmSyncFeed.
	if err := nejmEnsureFeedColumn(db); err != nil {
		return 0, fmt.Errorf("migrating local database: %w", err)
	}

	feeds := []struct{ path, name string }{
		{nejmEtocFeedURL, "etoc"},
		{nejmAxatocFeedURL, "axatoc"},
	}

	total := 0
	var errs []error
	for _, feed := range feeds {
		n, err := nejmSyncFeed(ctx, db, feed.path, feed.name, w)
		if err != nil {
			if w != nil {
				fmt.Fprintf(w, "warning: sync feed %s: %v\n", feed.name, err)
			}
			errs = append(errs, fmt.Errorf("%s: %w", feed.name, err))
			continue
		}
		total += n
	}

	// Only surface a hard error when every feed failed (e.g. network partition,
	// DNS outage, Cloudflare block). Partial success returns (total, nil) so a
	// single flaky feed doesn't fail the whole sync. The returned error lets the
	// caller emit a structured rss_fetch_error sync_warning in --json mode.
	if len(errs) == len(feeds) && len(errs) > 0 {
		return total, fmt.Errorf("all NEJM feeds failed: %w", errors.Join(errs...))
	}
	return total, nil
}

// --- Article detail page extraction ---

// nejmFetchAndParseArticle fetches an article detail page via the Surf client
// and extracts NEJM-specific meta tags (abstract, article_type, specialties, isFree).
// Merges results into an existing nejmArticleRecord (DOI already known from RSS).
func nejmFetchAndParseArticle(ctx context.Context, c *client.Client, doi string) (nejmArticleRecord, error) {
	path := "/doi/full/" + doi
	raw, err := c.GetWithHeaders(ctx, path, nil, nil)
	if err != nil {
		return nejmArticleRecord{}, fmt.Errorf("fetching article %s: %w", doi, err)
	}

	meta := nejmParseArticleMeta(raw)

	articleURL := nejmBaseURL + "/doi/full/" + doi

	rec := nejmArticleRecord{
		ID:          doi,
		DOI:         doi,
		Title:       meta["dc.Title"],
		Authors:     meta["dc.Creator"],
		Abstract:    firstNonEmpty(meta["description"], meta["shortAbstract"], meta["og:description"]),
		ArticleType: firstNonEmpty(meta["articleType"], meta["articleCategory"]),
		Specialties: meta["Specialties"],
		Date:        meta["dc.Date"],
		IsFree:      strings.EqualFold(meta["isFree"], "true"),
		URL:         articleURL,
	}
	return rec, nil
}

// nejmParseArticleMeta parses NEJM article page HTML and returns a map of
// meta name/property → content for NEJM-specific meta tags.
func nejmParseArticleMeta(htmlBytes []byte) map[string]string {
	meta := make(map[string]string)

	doc, err := xhtml.Parse(strings.NewReader(string(htmlBytes)))
	if err != nil {
		return meta
	}

	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode {
			tag := strings.ToLower(n.Data)
			if tag == "meta" {
				name := attrValue(n, "name")
				if name == "" {
					name = attrValue(n, "property")
				}
				content := attrValue(n, "content")
				if name != "" && content != "" {
					meta[name] = content
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return meta
}

// firstNonEmpty returns the first non-empty string from the arguments.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// nejmQueryArticles runs a SQL query against the article table and returns results.
// The where clause may reference typed columns: doi, title, authors, abstract,
// article_type, specialties, date, is_free, url, synced_at, and feed.
// The `feed` column is a real column (added by nejmEnsureFeedColumn) holding the
// source feed name ('etoc' for the current issue, 'axatoc' for recently published).
func nejmQueryArticles(db *store.Store, where string, args []any, limit int) ([]map[string]any, error) {
	q := `SELECT doi, title, authors, abstract, article_type, specialties, date, is_free, url, feed, synced_at FROM "article"`
	if where != "" {
		q += " WHERE " + where
	}
	q += " ORDER BY date DESC, synced_at DESC"
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var doi, title, authors, abstract, articleType, specialties, date, url, syncedAt string
		var feed *string
		var isFree bool
		if err := rows.Scan(&doi, &title, &authors, &abstract, &articleType, &specialties, &date, &isFree, &url, &feed, &syncedAt); err != nil {
			continue
		}
		feedVal := ""
		if feed != nil {
			feedVal = *feed
		}
		results = append(results, map[string]any{
			"doi":          doi,
			"title":        title,
			"authors":      authors,
			"abstract":     abstract,
			"article_type": articleType,
			"specialties":  specialties,
			"date":         date,
			"is_free":      isFree,
			"url":          url,
			"feed":         feedVal,
			"synced_at":    syncedAt,
		})
	}
	return results, rows.Err()
}

// nejmOpenStore opens the store for reading, returning a clear error if not synced.
// openStoreForRead opens the DB read-write (WAL), so the idempotent feed-column
// migration runs here too — guaranteeing `current` and friends never hit a
// "no such column: feed" error even if the user has not re-synced since upgrading.
func nejmOpenStore(ctx context.Context) (*store.Store, error) {
	db, err := openStoreForRead(ctx, "nejm-pp-cli")
	if err != nil {
		return nil, fmt.Errorf("opening local database: %w\nRun 'nejm-pp-cli sync' first", err)
	}
	if db == nil {
		return nil, fmt.Errorf("no local data. Run 'nejm-pp-cli sync' first to populate the corpus")
	}
	if err := nejmEnsureFeedColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating local database: %w", err)
	}
	return db, nil
}

// nejmPrintArticles marshals a []map[string]any slice and writes it to the command.
func nejmPrintArticles(cmd interface {
	OutOrStdout() io.Writer
	ErrOrStderr() io.Writer
}, items []map[string]any, flags *rootFlags) error {
	if len(items) == 0 {
		fmt.Fprintln(cmd.ErrOrStderr(), "hint: no articles found. Run 'nejm-pp-cli sync' to populate the corpus.")
		if flags.asJSON {
			return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage("[]"), flags)
		}
		return nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		return err
	}
	return printOutputWithFlags(cmd.OutOrStdout(), json.RawMessage(data), flags)
}

// parseSinceDuration converts strings like "48h", "7d", "2w" into a time.Time cutoff.
// Duplicated here (also exists in sync.go) to keep nejm_feeds.go self-contained.
func nejmParseSinceDuration(s string) (time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return time.Time{}, fmt.Errorf("empty duration")
	}
	unit := s[len(s)-1]
	numStr := s[:len(s)-1]
	switch unit {
	case 'h':
		var n int
		if _, err := fmt.Sscanf(numStr, "%d", &n); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		return time.Now().Add(-time.Duration(n) * time.Hour), nil
	case 'd':
		var n int
		if _, err := fmt.Sscanf(numStr, "%d", &n); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		return time.Now().AddDate(0, 0, -n), nil
	case 'w':
		var n int
		if _, err := fmt.Sscanf(numStr, "%d", &n); err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q", s)
		}
		return time.Now().AddDate(0, 0, -n*7), nil
	default:
		// Try standard Go duration
		d, err := time.ParseDuration(s)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid duration %q: use e.g. 48h, 7d, 2w", s)
		}
		return time.Now().Add(-d), nil
	}
}
