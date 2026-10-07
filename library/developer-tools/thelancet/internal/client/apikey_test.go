package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/thelancet/internal/config"
)

const fakeOpenAlexKey = "test-key-0000000000ab"

// newKeyTestClient builds a client against srv with OPENALEX_API_KEY set to
// key (unset when empty) and no on-disk config.
func newKeyTestClient(t *testing.T, srv *httptest.Server, key string) *Client {
	t.Helper()
	t.Setenv("THELANCET_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv("THELANCET_BASE_URL", srv.URL)
	if key == "" {
		os.Unsetenv("OPENALEX_API_KEY")
	} else {
		t.Setenv("OPENALEX_API_KEY", key)
	}
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	return c
}

// T1: with the variable set, a request carries the key as a Bearer header and
// never in the URL.
func TestOpenAlexKeySentAsBearer(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newKeyTestClient(t, srv, fakeOpenAlexKey)
	if _, err := c.GetNoCache(context.Background(), "/works", map[string]string{"per-page": "1"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+fakeOpenAlexKey {
		t.Errorf("Authorization header = %q, want Bearer + key", gotAuth)
	}
	if strings.Contains(gotQuery, fakeOpenAlexKey) || strings.Contains(gotQuery, "api_key") {
		t.Errorf("key leaked into query: %q", gotQuery)
	}
}

// T2: without the variable nothing key-shaped is sent.
func TestOpenAlexNoKeySendsNothing(t *testing.T) {
	var gotAuth, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newKeyTestClient(t, srv, "")
	if _, err := c.GetNoCache(context.Background(), "/works", map[string]string{"per-page": "1"}); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty", gotAuth)
	}
	if strings.Contains(gotQuery, "api_key") {
		t.Errorf("api_key param sent: %q", gotQuery)
	}
}

// T3: --dry-run shows only the masked key.
func TestOpenAlexDryRunMasksKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("dry run must not send a request")
	}))
	defer srv.Close()
	c := newKeyTestClient(t, srv, fakeOpenAlexKey)
	c.DryRun = true

	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	_, _, err := c.do(context.Background(), "GET", "/works", map[string]string{"per-page": "1"}, nil, nil)
	w.Close()
	os.Stderr = old
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), fakeOpenAlexKey) {
		t.Errorf("dry-run output contains the full key:\n%s", out)
	}
	if !strings.Contains(string(out), "Authorization: ****00ab") {
		t.Errorf("dry-run output lacks masked key:\n%s", out)
	}
}

// T4: an HTTP error never carries the full key, even when the server echoes it.
func TestOpenAlexErrorMasksKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request, auth was "+r.Header.Get("Authorization"), http.StatusBadRequest)
	}))
	defer srv.Close()
	c := newKeyTestClient(t, srv, fakeOpenAlexKey)
	_, err := c.GetNoCache(context.Background(), "/works", nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), fakeOpenAlexKey) {
		t.Errorf("error contains the full key: %v", err)
	}
	if !strings.Contains(err.Error(), "****00ab") {
		t.Errorf("error lacks masked key (server echo not exercised?): %v", err)
	}
}

// T5: keyed and keyless responses never share a cache entry.
func TestOpenAlexCacheKeyDiffersWithKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	with := newKeyTestClient(t, srv, fakeOpenAlexKey)
	without := newKeyTestClient(t, srv, "")
	without.Config.Path = with.Config.Path // isolate the key as the only difference
	p := map[string]string{"per-page": "1"}
	if with.cacheKey("/works", p) == without.cacheKey("/works", p) {
		t.Error("cache key identical with and without an API key")
	}
}
