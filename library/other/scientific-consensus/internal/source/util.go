package source

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/cliutil"
)

// normDOI lowercases and strips any URL/scheme prefix from a DOI for stable
// cross-source joins.
func normDOI(doi string) string {
	d := strings.ToLower(strings.TrimSpace(doi))
	d = strings.TrimPrefix(d, "https://doi.org/")
	d = strings.TrimPrefix(d, "http://doi.org/")
	d = strings.TrimPrefix(d, "doi:")
	return d
}

// NormDOI is the exported form for cross-package joins.
func NormDOI(doi string) string { return normDOI(doi) }

// getJSONWithKey is getJSON plus the Semantic Scholar API key header when
// SEMANTIC_SCHOLAR_API_KEY is set. // pp:client-call
func getJSONWithKey(ctx context.Context, limiter *cliutil.AdaptiveLimiter, url string, v any) error {
	limiter.Wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if k := os.Getenv("SEMANTIC_SCHOLAR_API_KEY"); k != "" {
		req.Header.Set("x-api-key", k)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		limiter.OnRateLimit()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &cliutil.RateLimitError{URL: url, Body: string(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: HTTP %d: %s", url, resp.StatusCode, string(body))
	}
	limiter.OnSuccess()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	return json.Unmarshal(data, v)
}
