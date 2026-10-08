package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/developer-tools/thelancet/internal/config"
)

const fakeOpenAlexKey = "test-key-0000000000ab"

// setOpenAlexKey points OPENALEX_API_KEY at key for the test; an empty key
// leaves the variable truly unset. t.Setenv registers the restore of the
// caller's value before os.Unsetenv clears it.
func setOpenAlexKey(t *testing.T, key string) {
	t.Helper()
	if key == "" {
		t.Setenv("OPENALEX_API_KEY", "")
		os.Unsetenv("OPENALEX_API_KEY")
		return
	}
	t.Setenv("OPENALEX_API_KEY", key)
}

// newKeyTestClientAt builds a client for baseURL with OPENALEX_API_KEY set to
// key (unset when empty) and no on-disk config.
func newKeyTestClientAt(t *testing.T, baseURL, key string) *Client {
	t.Helper()
	t.Setenv("THELANCET_CONFIG", filepath.Join(t.TempDir(), "absent.toml"))
	t.Setenv("THELANCET_BASE_URL", baseURL)
	setOpenAlexKey(t, key)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	return c
}

// newKeyTestClient builds a client against the plain-http test server srv.
func newKeyTestClient(t *testing.T, srv *httptest.Server, key string) *Client {
	t.Helper()
	return newKeyTestClientAt(t, srv.URL, key)
}

// routeTransport delivers every request to target (an httptest server) and
// records the scheme://host the client addressed, so tests can use a real
// https://api.openalex.org base URL without any network access.
type routeTransport struct {
	target    *url.URL
	addressed *string
}

func (rt routeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.addressed != nil {
		*rt.addressed = req.URL.Scheme + "://" + req.URL.Host
	}
	out := req.Clone(req.Context())
	out.URL.Scheme, out.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(out)
}

// newRoutedKeyTestClient builds a client for baseURL whose requests are
// delivered to an httptest server; *auth receives the Authorization header
// that server saw and *addressed the scheme://host the client targeted.
func newRoutedKeyTestClient(t *testing.T, baseURL, key string, auth, addressed *string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*auth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	target, _ := url.Parse(srv.URL)
	c := newKeyTestClientAt(t, baseURL, key)
	c.HTTPClient.Transport = routeTransport{target, addressed}
	return c
}

// T1: with the variable set, a request to https://api.openalex.org carries the
// key as a Bearer header and never in the URL.
func TestOpenAlexKeySentAsBearer(t *testing.T) {
	var gotAuth, gotQuery, addressed string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotQuery = r.Header.Get("Authorization"), r.URL.RawQuery
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = routeTransport{target, &addressed}
	if _, err := c.GetNoCache(context.Background(), "/works", map[string]string{"per-page": "1"}); err != nil {
		t.Fatal(err)
	}
	if addressed != "https://api.openalex.org" {
		t.Fatalf("client addressed %q", addressed)
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
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Error("dry run must not send a request")
		return nil, io.EOF
	})
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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

// T4: an HTTP error never carries the full key, even when the server echoes it.
func TestOpenAlexErrorMasksKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request, auth was "+r.Header.Get("Authorization"), http.StatusBadRequest)
	}))
	defer srv.Close()
	target, _ := url.Parse(srv.URL)
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = routeTransport{target, nil}
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

// T6: a custom http base URL never receives the env key.
func TestOpenAlexKeyNotSentToCustomServer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newKeyTestClient(t, srv, fakeOpenAlexKey)
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "" {
		t.Errorf("custom server received Authorization = %q, want none", gotAuth)
	}
}

// T7: https://api.openalex.org (host compared case-insensitively) gets the key.
func TestOpenAlexKeySentToOpenAlexHost(t *testing.T) {
	for _, base := range []string{"https://api.openalex.org", "https://API.OpenAlex.org"} {
		var auth, addressed string
		c := newRoutedKeyTestClient(t, base, fakeOpenAlexKey, &auth, &addressed)
		if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
			t.Fatal(err)
		}
		if auth != "Bearer "+fakeOpenAlexKey {
			t.Errorf("%s: Authorization = %q, want Bearer + key", base, auth)
		}
	}
}

// T8: look-alike hosts and plain http to the real host get no key.
func TestOpenAlexKeyNotSentToLookAlikeHosts(t *testing.T) {
	for _, base := range []string{
		"https://api.openalex.org.evil.test",
		"https://evil.test/api.openalex.org",
		"https://notapi.openalex.org",
		"https://api.openalex.org.",
		"http://api.openalex.org",
	} {
		var auth, addressed string
		c := newRoutedKeyTestClient(t, base, fakeOpenAlexKey, &auth, &addressed)
		if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
			t.Fatalf("%s: %v", base, err)
		}
		if addressed == "" {
			t.Fatalf("%s: request never reached the transport", base)
		}
		if auth != "" {
			t.Errorf("%s: Authorization = %q, want none", base, auth)
		}
	}
}

// T10: a key the user put in the config file keeps being sent to any host.
func TestConfigFileAuthHeaderStillSentToCustomServer(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("auth_header = \"Bearer file-key-0000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THELANCET_CONFIG", cfgPath)
	t.Setenv("THELANCET_BASE_URL", srv.URL)
	setOpenAlexKey(t, "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer file-key-0000" {
		t.Errorf("Authorization = %q, want the config-file value", gotAuth)
	}
}

// T9: a test that clears OPENALEX_API_KEY restores the caller's value.
func TestOpenAlexClearingTestRestoresEnv(t *testing.T) {
	const parent = "parent-value-000000"
	t.Setenv("OPENALEX_API_KEY", parent)
	t.Run("clears", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer srv.Close()
		newKeyTestClient(t, srv, "")
		if _, ok := os.LookupEnv("OPENALEX_API_KEY"); ok {
			t.Error("variable should be unset inside the clearing test")
		}
	})
	if got := os.Getenv("OPENALEX_API_KEY"); got != parent {
		t.Errorf("parent value after subtest = %q, want %q", got, parent)
	}
}

// redirectRecorder answers the first request with a 302 to location and
// records the headers of every later request.
func redirectRecorder(location string, second *http.Header) roundTripFunc {
	n := 0
	return func(req *http.Request) (*http.Response, error) {
		n++
		if n == 1 {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{location}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}
		*second = req.Header.Clone()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Request:    req,
		}, nil
	}
}

// T11: a same-host https -> http redirect must not carry the env key.
func TestOpenAlexKeyDroppedOnHTTPDowngradeRedirect(t *testing.T) {
	var second http.Header
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = redirectRecorder("http://api.openalex.org/works?per-page=1", &second)
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if second == nil {
		t.Fatal("redirect was not followed")
	}
	if got := second.Get("Authorization"); got != "" {
		t.Errorf("redirected http request carried Authorization = %q, want none", got)
	}
}

// T12: a same-host https -> https redirect keeps the env key.
func TestOpenAlexKeyKeptOnSameHostHTTPSRedirect(t *testing.T) {
	var second http.Header
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = redirectRecorder("https://api.openalex.org/other", &second)
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if got := second.Get("Authorization"); got != "Bearer "+fakeOpenAlexKey {
		t.Errorf("redirected https request Authorization = %q, want Bearer + key", got)
	}
}

// T13: a config-file key is not gated by the env-key rule, so the same
// downgrade redirect keeps it (existing behaviour, unchanged by the fix).
func TestConfigFileKeyUnchangedOnHTTPDowngradeRedirect(t *testing.T) {
	var second http.Header
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("auth_header = \"Bearer file-key-0000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THELANCET_CONFIG", cfgPath)
	t.Setenv("THELANCET_BASE_URL", "https://api.openalex.org")
	setOpenAlexKey(t, "")
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	c.HTTPClient.Transport = redirectRecorder("http://api.openalex.org/works", &second)
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if got := second.Get("Authorization"); got != "Bearer file-key-0000" {
		t.Errorf("config-file Authorization after redirect = %q, want unchanged Bearer file-key-0000", got)
	}
}

// newHeadersConfigClient builds a client for baseURL with OPENALEX_API_KEY set
// and an explicit [headers] Authorization in the config file.
func newHeadersConfigClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(cfgPath, []byte("[headers]\nAuthorization = \"Bearer custom-0000\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("THELANCET_CONFIG", cfgPath)
	t.Setenv("THELANCET_BASE_URL", baseURL)
	setOpenAlexKey(t, fakeOpenAlexKey)
	cfg, err := config.Load("")
	if err != nil {
		t.Fatal(err)
	}
	c := New(cfg, 5*time.Second, 0)
	c.cacheDir = t.TempDir()
	return c
}

// recordAll answers the first request with a 302 to location and records the
// headers of every request, first included.
func recordAll(location string, seen *[]http.Header) roundTripFunc {
	inner := redirectRecorder(location, new(http.Header))
	return func(req *http.Request) (*http.Response, error) {
		*seen = append(*seen, req.Header.Clone())
		return inner(req)
	}
}

// T14: an explicit [headers] Authorization on a custom server (env key set)
// survives a same-host redirect.
func TestExplicitHeadersAuthKeptOnSameHostRedirect(t *testing.T) {
	var seen []http.Header
	c := newHeadersConfigClient(t, "http://proxy.test")
	c.HTTPClient.Transport = recordAll("http://proxy.test/next", &seen)
	if _, err := c.GetNoCache(context.Background(), "/works", nil); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want 2", len(seen))
	}
	for i, h := range seen {
		if got := h.Get("Authorization"); got != "Bearer custom-0000" {
			t.Errorf("request %d Authorization = %q, want Bearer custom-0000", i+1, got)
		}
	}
}

// T15: https -> http downgrade on api.openalex.org with a per-call
// Authorization override keeps the override and never sends the env key.
func TestPerCallAuthOverrideKeptOnDowngradeRedirect(t *testing.T) {
	var seen []http.Header
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = recordAll("http://api.openalex.org/works", &seen)
	hdr := map[string]string{"Authorization": "Bearer custom-0000"}
	if _, err := c.GetWithHeadersNoCache(context.Background(), "/works", nil, hdr); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want 2", len(seen))
	}
	if got := seen[0].Get("Authorization"); got != "Bearer custom-0000" {
		t.Errorf("first request Authorization = %q, want the override", got)
	}
	if got := seen[1].Get("Authorization"); got != "Bearer custom-0000" {
		t.Errorf("http hop Authorization = %q, want the override kept", got)
	}
	for i, h := range seen {
		if strings.Contains(h.Get("Authorization"), fakeOpenAlexKey) {
			t.Errorf("request %d carried the env key", i+1)
		}
	}
}

// T16: an override that merely contains the env credential is not the env
// credential, so the downgrade redirect keeps it.
func TestAuthOverrideContainingEnvKeyKeptOnDowngradeRedirect(t *testing.T) {
	var seen []http.Header
	c := newKeyTestClientAt(t, "https://api.openalex.org", fakeOpenAlexKey)
	c.HTTPClient.Transport = recordAll("http://api.openalex.org/works", &seen)
	custom := "Bearer " + fakeOpenAlexKey + "-extra"
	if _, err := c.GetWithHeadersNoCache(context.Background(), "/works", nil, map[string]string{"Authorization": custom}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 {
		t.Fatalf("saw %d requests, want 2", len(seen))
	}
	if got := seen[1].Get("Authorization"); got != custom {
		t.Errorf("http hop Authorization = %q, want %q kept", got, custom)
	}
}
