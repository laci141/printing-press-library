// Copyright 2026 laci141 and contributors. Licensed under Apache-2.0. See LICENSE.

package source

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func rawMap(t *testing.T, js string) map[string]json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(js), &m); err != nil {
		t.Fatalf("bad test json: %v", err)
	}
	return m
}

func TestCtisExtractCount(t *testing.T) {
	cases := []struct {
		name string
		js   string
		want int
	}{
		{"top-level totalSize", `{"totalSize":42}`, 42},
		{"top-level total", `{"total":7}`, 7},
		{"nested pagination", `{"pagination":{"totalRecords":13}}`, 13},
		{"missing", `{"foo":"bar"}`, 0},
		{"first match wins", `{"totalSize":5,"total":99}`, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ctisExtractCount(rawMap(t, tc.js)); got != tc.want {
				t.Errorf("ctisExtractCount(%s) = %d, want %d", tc.js, got, tc.want)
			}
		})
	}
}

func TestCtisString(t *testing.T) {
	m := rawMap(t, `{"a":"first","b":{"value":"nested"},"empty":""}`)
	if got := ctisString(m, "a"); got != "first" {
		t.Errorf("plain string: got %q", got)
	}
	if got := ctisString(m, "b"); got != "nested" {
		t.Errorf("nested value: got %q", got)
	}
	// empty string should be skipped in favor of the next present key
	if got := ctisString(m, "empty", "a"); got != "first" {
		t.Errorf("empty fallthrough: got %q", got)
	}
	if got := ctisString(m, "missing"); got != "" {
		t.Errorf("missing key: got %q", got)
	}
}

func TestCtisCountries(t *testing.T) {
	m := rawMap(t, `{"trialCountries":["France:5","Germany:2","France:1","  ",""]}`)
	got := ctisCountries(m, "trialCountries")
	want := []string{"France", "Germany"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q, want %q", i, got[i], want[i])
		}
	}
	if ctisCountries(rawMap(t, `{}`), "trialCountries") != nil {
		t.Errorf("missing key should yield nil")
	}
}

func TestCtisExtractItems(t *testing.T) {
	js := `{"data":[{"ctNumber":"2024-001","ctTitle":"Trial A","trialCountries":["Spain:3"]},
	                 {"id":"2024-002","title":"Trial B"}]}`
	items := ctisExtractItems(rawMap(t, js), 10)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}
	if items[0].CTNumber != "2024-001" || items[0].Title != "Trial A" {
		t.Errorf("item0 = %+v", items[0])
	}
	if items[1].CTNumber != "2024-002" {
		t.Errorf("item1 ctNumber = %q", items[1].CTNumber)
	}
	// limit is honored
	if got := ctisExtractItems(rawMap(t, js), 1); len(got) != 1 {
		t.Errorf("limit=1 yielded %d items", len(got))
	}
}

func TestTruncate429(t *testing.T) {
	if got := truncate429([]byte("short")); got != "short" {
		t.Errorf("short passthrough: %q", got)
	}
	long := strings.Repeat("x", 400)
	got := truncate429([]byte(long))
	if len(got) != 303 || !strings.HasSuffix(got, "...") {
		t.Errorf("long truncate: len=%d suffix ok=%v", len(got), strings.HasSuffix(got, "..."))
	}
}

func TestIsNotFound(t *testing.T) {
	if !isNotFound(errors.New("request failed: HTTP 404 not found")) {
		t.Errorf("404 error should be not-found")
	}
	if isNotFound(errors.New("HTTP 500")) {
		t.Errorf("500 should not be not-found")
	}
	if isNotFound(nil) {
		t.Errorf("nil should not be not-found")
	}
}
