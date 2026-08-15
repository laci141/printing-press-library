package sources

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"
)

// grantsSearchStub stands in for the live API. It records the keyword of every
// search it receives and answers with the hit count the test dictates; a word
// mapped to a negative count is answered with an HTTP error instead.
type grantsSearchStub struct {
	mu       sync.Mutex
	keywords []string
	hits     map[string]int
}

func (s *grantsSearchStub) seen() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	got := append([]string(nil), s.keywords...)
	sort.Strings(got) // the requests run concurrently, so arrival order is not fixed
	return got
}

// startGrantsSearchStub points grantsSearchURL at the stub for one test.
func startGrantsSearchStub(t *testing.T, hits map[string]int) *grantsSearchStub {
	t.Helper()
	stub := &grantsSearchStub{hits: hits}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Keyword string `json:"keyword"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request body", http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		stub.keywords = append(stub.keywords, req.Keyword)
		stub.mu.Unlock()

		n, ok := stub.hits[req.Keyword]
		if ok && n < 0 {
			// 4xx rather than 5xx: do() retries 5xx with a one-second pause.
			http.Error(w, "upstream refused", http.StatusBadRequest)
			return
		}
		var resp grantsSearchResp
		resp.Data.HitCount = n
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	prev := grantsSearchURL
	grantsSearchURL = srv.URL
	t.Cleanup(func() { grantsSearchURL = prev })
	return stub
}

// A single word has nothing to be compared against: the breakdown would only
// restate the total the user already sees.
func TestKeywordHitsSingleWordReturnsNil(t *testing.T) {
	stub := startGrantsSearchStub(t, map[string]int{"climate": 61})

	if got := KeywordHits("climate", ""); got != nil {
		t.Errorf("KeywordHits(%q) = %v, want nil", "climate", got)
	}
	if seen := stub.seen(); len(seen) != 0 {
		t.Errorf("stub received %v, want no requests at all", seen)
	}
}

// "and" is not a word Grants.gov was asked about, so it must not be reported as
// one that matched nothing.
func TestKeywordHitsDropsStopWords(t *testing.T) {
	stub := startGrantsSearchStub(t, map[string]int{"climate": 61, "water": 12})

	got := KeywordHits("climate and water", "")
	want := []KeywordHit{{Word: "climate", Hits: 61}, {Word: "water", Hits: 12}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KeywordHits = %v, want %v", got, want)
	}
	if seen, wantSeen := stub.seen(), []string{"climate", "water"}; !reflect.DeepEqual(seen, wantSeen) {
		t.Errorf("stub was asked about %v, want %v", seen, wantSeen)
	}
}

// The zero-hit word is the whole point of the breakdown: it is the one that
// contributed nothing to the results the user is looking at.
func TestKeywordHitsReportsZeroHitWord(t *testing.T) {
	startGrantsSearchStub(t, map[string]int{"climate": 61, "zzzqqqxxx": 0})

	got := KeywordHits("climate zzzqqqxxx", "")
	want := []KeywordHit{{Word: "climate", Hits: 61}, {Word: "zzzqqqxxx", Hits: 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("KeywordHits = %v, want %v", got, want)
	}
}

// A breakdown missing one word would read as "that word matched nothing", which
// is a different and wrong statement. Better to say nothing.
func TestKeywordHitsFailureReturnsNilNotPartial(t *testing.T) {
	startGrantsSearchStub(t, map[string]int{"climate": 61, "water": -1})

	if got := KeywordHits("climate water", ""); got != nil {
		t.Errorf("KeywordHits = %v, want nil when a request failed", got)
	}
}
