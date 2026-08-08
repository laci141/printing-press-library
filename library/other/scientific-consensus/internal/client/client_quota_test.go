// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.

package client

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

// captureQuotaStderr runs fn with os.Stderr redirected and returns what it wrote.
func captureQuotaStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	os.Stderr = orig
	w.Close()
	out := <-done
	r.Close()
	return out
}

func quotaHeader(remaining, reset string) http.Header {
	h := http.Header{}
	if remaining != "" {
		h.Set("X-RateLimit-Remaining", remaining)
	}
	if reset != "" {
		h.Set("X-RateLimit-Reset", reset)
	}
	return h
}

func TestWarnLowQuota_NoHeaders(t *testing.T) {
	c := &Client{}
	// PubMed, Crossref and Europe PMC responses look like this.
	out := captureQuotaStderr(t, func() { c.warnLowQuota(http.Header{}) })
	if out != "" {
		t.Fatalf("expected silence without quota headers, got %q", out)
	}
	if c.quotaWarned {
		t.Fatal("quotaWarned set without any quota header")
	}
}

func TestWarnLowQuota_AboveThreshold(t *testing.T) {
	c := &Client{}
	out := captureQuotaStderr(t, func() { c.warnLowQuota(quotaHeader("949", "26641")) })
	if out != "" {
		t.Fatalf("expected silence at 949 remaining, got %q", out)
	}
}

func TestWarnLowQuota_BelowThresholdWarnsOnce(t *testing.T) {
	c := &Client{}
	first := captureQuotaStderr(t, func() { c.warnLowQuota(quotaHeader("47", "26641")) })
	want := "quota nearly exhausted, 47 requests remaining (shared per IP, resets in 7.4h)\n"
	if first != want {
		t.Fatalf("warning mismatch:\n got %q\nwant %q", first, want)
	}
	second := captureQuotaStderr(t, func() { c.warnLowQuota(quotaHeader("46", "26580")) })
	if second != "" {
		t.Fatalf("expected only one warning per client, second was %q", second)
	}
}

func TestWarnLowQuota_MalformedValues(t *testing.T) {
	cases := []struct {
		name      string
		remaining string
		reset     string
		want      string
	}{
		{"non-numeric remaining", "many", "26641", ""},
		{"empty remaining", "", "26641", ""},
		{"negative remaining", "-5", "26641", ""},
		{"non-numeric reset", "12", "soon", "quota nearly exhausted, 12 requests remaining (shared per IP, resets in unknown)\n"},
		{"missing reset", "12", "", "quota nearly exhausted, 12 requests remaining (shared per IP, resets in unknown)\n"},
		{"sub-hour reset", "12", "900", "quota nearly exhausted, 12 requests remaining (shared per IP, resets in 15m)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &Client{}
			out := captureQuotaStderr(t, func() { c.warnLowQuota(quotaHeader(tc.remaining, tc.reset)) })
			if out != tc.want {
				t.Fatalf("got %q, want %q", out, tc.want)
			}
			if tc.want == "" && strings.Contains(out, "quota") {
				t.Fatalf("unexpected quota warning: %q", out)
			}
		})
	}
}
