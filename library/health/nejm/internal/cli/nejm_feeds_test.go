// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.
// Hand-coded: tests for NEJM RSS feed sync error aggregation.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/printing-press-library/library/health/nejm/internal/store"
)

// nejmFeedRSS renders a minimal RDF/PRISM feed carrying a single item with the
// given DOI and title. nejmFetchFeed strips the dc:/prism: prefixes before
// parsing, so the namespaced shape here mirrors the live NEJM feed.
func nejmFeedRSS(doi, title string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
         xmlns:dc="http://purl.org/dc/elements/1.1/"
         xmlns:prism="http://prismstandard.org/namespaces/basic/2.0/">
  <item>
    <title>%s</title>
    <link>https://www.nejm.org/doi/full/%s</link>
    <dc:creator>Jane Doe</dc:creator>
    <dc:date>2026-06-20</dc:date>
    <prism:doi>%s</prism:doi>
    <prism:volume>394</prism:volume>
    <prism:number>24</prism:number>
  </item>
</rdf:RDF>`, title, doi, doi)
}

func newFeedTestStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.OpenWithContext(context.Background(), filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// feedResponse describes what the mock NEJM server returns for one feed type.
type feedResponse struct {
	status int
	body   string
}

// startFeedServer stands up an httptest server that serves per-feed responses
// keyed by the RSS `type` query param ("etoc" / "axatoc"), and repoints
// nejmBaseURL at it for the duration of the test.
func startFeedServer(t *testing.T, byType map[string]feedResponse) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp, ok := byType[r.URL.Query().Get("type")]
		if !ok {
			http.Error(w, "unexpected feed", http.StatusNotFound)
			return
		}
		w.WriteHeader(resp.status)
		_, _ = w.Write([]byte(resp.body))
	}))
	t.Cleanup(srv.Close)

	prev := nejmBaseURL
	nejmBaseURL = srv.URL
	t.Cleanup(func() { nejmBaseURL = prev })
}

func TestSyncNEJMFeeds(t *testing.T) {
	t.Run("both feeds succeed", func(t *testing.T) {
		startFeedServer(t, map[string]feedResponse{
			"etoc":   {http.StatusOK, nejmFeedRSS("10.1056/etoc-1", "Etoc Article")},
			"axatoc": {http.StatusOK, nejmFeedRSS("10.1056/axatoc-1", "Axatoc Article")},
		})
		db := newFeedTestStore(t)
		var w bytes.Buffer

		total, err := nejmSyncAllFeeds(context.Background(), db, &w)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if total != 2 {
			t.Fatalf("total = %d, want 2 (one article per feed)", total)
		}
	})

	t.Run("partial failure returns no error", func(t *testing.T) {
		// etoc works, axatoc is down (e.g. transient 503). A single flaky feed
		// must not fail the whole sync, and the successful feed's count stands.
		startFeedServer(t, map[string]feedResponse{
			"etoc":   {http.StatusOK, nejmFeedRSS("10.1056/etoc-1", "Etoc Article")},
			"axatoc": {http.StatusServiceUnavailable, "down"},
		})
		db := newFeedTestStore(t)
		var w bytes.Buffer

		total, err := nejmSyncAllFeeds(context.Background(), db, &w)
		if err != nil {
			t.Fatalf("partial failure should not return an error, got: %v", err)
		}
		if total != 1 {
			t.Fatalf("total = %d, want 1 (only etoc synced)", total)
		}
		// The per-feed failure is still surfaced to the human/stderr writer.
		if got := w.String(); !strings.Contains(got, "axatoc") {
			t.Fatalf("warning writer = %q, want it to mention the failed feed", got)
		}
	})

	t.Run("total failure returns aggregated error", func(t *testing.T) {
		// Both feeds down (network partition / DNS outage / Cloudflare block).
		// This is the case the JSON sync_warning rss_fetch_error path depends on.
		startFeedServer(t, map[string]feedResponse{
			"etoc":   {http.StatusInternalServerError, "boom"},
			"axatoc": {http.StatusInternalServerError, "boom"},
		})
		db := newFeedTestStore(t)
		var w bytes.Buffer

		total, err := nejmSyncAllFeeds(context.Background(), db, &w)
		if err == nil {
			t.Fatalf("expected an error when every feed fails, got nil")
		}
		if total != 0 {
			t.Fatalf("total = %d, want 0", total)
		}
		// The aggregated error must name both failing feeds so a --json consumer
		// reading the rss_fetch_error message can see the full picture.
		msg := err.Error()
		if !strings.Contains(msg, "etoc") || !strings.Contains(msg, "axatoc") {
			t.Fatalf("error %q must mention both etoc and axatoc", msg)
		}
	})
}
