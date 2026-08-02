package cli

// zz_live_backfill_probe_test.go is a TEMPORARY measurement harness, not a test
// of anything. It exists to answer one question with live data before any
// abstract-backfill feature is written:
//
//	returners - losers > 0 ?
//
// where "returners" are works the gates currently exclude that a PubMed-supplied
// abstract would let back in, and "losers" are works the gates currently KEEP
// that a PubMed-supplied abstract would push out (an abstract-less work needs
// only relevanceMinTokensNoAbstract stems; giving it an abstract raises the bar
// to relevanceMinTokens).
//
// It replays the REAL gates — relevantToClaim, scengine.PICOTokens,
// scengine.IsPICORelevant — never a second implementation of them, because two
// tokenizers that drift apart would make the measurement describe the harness
// instead of the product.
//
// Skipped unless SC_PROBE_LIVE=1, so `go test ./...` neither hits the network
// nor depends on NCBI availability.

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/scengine"
)

const (
	probeOpenAlexBase = "https://api.openalex.org"
	probeEutils       = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"
	// probeLimit is the consensus command's own --limit default, so the
	// measurement describes production conditions rather than a widened pool.
	probeLimit = 40
	probePace  = 350 * time.Millisecond
)

// liveOAGetter is the apiGetter the real fetchWorks needs, backed by the live
// OpenAlex API instead of the generated client (which would pull in config and
// caching this measurement does not want).
type liveOAGetter struct{ t *testing.T }

func (g liveOAGetter) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	q := url.Values{}
	for k, v := range params {
		q.Set(k, v)
	}
	u := probeOpenAlexBase + path + "?" + q.Encode()
	time.Sleep(probePace)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "scientific-consensus-backfill-probe (mailto:"+defaultMailto+")")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openalex GET %s: HTTP %d: %s", path, resp.StatusCode, truncProbe(string(body), 200))
	}
	return body, nil
}

func truncProbe(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// probeGates reports the two gate verdicts for one work, using the shipped gate
// code. Order matters only for reporting: production runs relevance first, so a
// work failing relevance never reaches PICO.
func probeGates(claim string, w scWork, iv, out []string) (relOK, picoOK bool) {
	relOK = relevantToClaim(claim, w)
	picoOK = scengine.IsPICORelevant(w.Abstract, w.Title, iv, out)
	return relOK, picoOK
}

func probePasses(claim string, w scWork, iv, out []string) bool {
	rel, pico := probeGates(claim, w, iv, out)
	return rel && pico
}

// ---- PubMed side of the probe -------------------------------------------
//
// Inlined here rather than calling a shipped helper: the point of this run is to
// decide whether that helper should exist. The parsing is the strict-path shape
// validated on 2026-08-02 (article-owned DOI and abstract only; a PubmedArticle
// also carries ArticleId/AbstractText elements under ReferenceList and
// OtherAbstract, and collecting those mapped 8 of 41 records to a cited paper).

type probeFetchSet struct {
	Articles []probeFetchArticle `xml:"PubmedArticle"`
}

type probeFetchArticle struct {
	PMID     string             `xml:"MedlineCitation>PMID"`
	Abstract []probeAbstractSec `xml:"MedlineCitation>Article>Abstract>AbstractText"`
}

type probeAbstractSec struct {
	Label string `xml:"Label,attr"`
	Inner string `xml:",innerxml"`
}

func (s probeAbstractSec) text() string {
	dec := xml.NewDecoder(strings.NewReader("<r>" + s.Inner + "</r>"))
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		if cd, ok := tok.(xml.CharData); ok {
			b.Write(cd)
		}
	}
	body := strings.Join(strings.Fields(b.String()), " ")
	if body == "" {
		return ""
	}
	if l := strings.TrimSpace(s.Label); l != "" {
		return l + ": " + body
	}
	return body
}

func probeGet(ctx context.Context, u string) ([]byte, error) {
	time.Sleep(probePace)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "scientific-consensus-backfill-probe")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncProbe(string(body), 160))
	}
	return body, nil
}

// probePubMedAbstracts resolves DOIs to abstracts one esearch per DOI, which
// measured 41/51 against 20/51 for OR-batched queries on 2026-08-02.
func probePubMedAbstracts(ctx context.Context, t *testing.T, dois []string) map[string]string {
	t.Helper()
	doiForPMID := map[string]string{}
	var pmids []string
	for _, d := range dois {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" {
			continue
		}
		u := fmt.Sprintf("%s/esearch.fcgi?db=pubmed&retmode=json&retmax=2&term=%s",
			probeEutils, url.QueryEscape(d+"[doi]"))
		raw, err := probeGet(ctx, u)
		if err != nil {
			t.Logf("  esearch %s: %v", d, err)
			continue
		}
		var r struct {
			ESearchResult struct {
				IDList []string `json:"idlist"`
			} `json:"esearchresult"`
		}
		if json.Unmarshal(raw, &r) != nil || len(r.ESearchResult.IDList) != 1 {
			continue
		}
		pmid := r.ESearchResult.IDList[0]
		if doiForPMID[pmid] != "" {
			continue
		}
		doiForPMID[pmid] = d
		pmids = append(pmids, pmid)
	}
	out := map[string]string{}
	for i := 0; i < len(pmids); i += 100 {
		end := i + 100
		if end > len(pmids) {
			end = len(pmids)
		}
		u := fmt.Sprintf("%s/efetch.fcgi?db=pubmed&retmode=xml&id=%s",
			probeEutils, url.QueryEscape(strings.Join(pmids[i:end], ",")))
		raw, err := probeGet(ctx, u)
		if err != nil {
			t.Logf("  efetch: %v", err)
			continue
		}
		var set probeFetchSet
		if err := xml.Unmarshal(raw, &set); err != nil {
			t.Logf("  efetch parse: %v", err)
			continue
		}
		for _, a := range set.Articles {
			var parts []string
			for _, s := range a.Abstract {
				if txt := s.text(); txt != "" {
					parts = append(parts, txt)
				}
			}
			if len(parts) == 0 {
				continue
			}
			if d := doiForPMID[strings.TrimSpace(a.PMID)]; d != "" {
				out[d] = strings.Join(parts, " ")
			}
		}
	}
	return out
}

// ---- the measurement ----------------------------------------------------

type probeTally struct {
	claim string
	// pool
	fetched int
	// current verdicts
	kept, excluded int
	// benefit side
	exclNoAbs, exclNoAbsNoDOI, exclBackfilled, returners int
	// risk side
	keptNoAbs, keptNoAbsNoDOI, keptBackfilled, losers int
	returnerTitles                                    []string
	loserTitles                                       []string
}

func TestLiveBackfillBenefitVsRisk(t *testing.T) {
	if os.Getenv("SC_PROBE_LIVE") != "1" {
		t.Skip("live probe: set SC_PROBE_LIVE=1 to run")
	}
	claims := []string{
		"omega-3 improves cardiovascular health",
		"vitamin C prevents the common cold",
		"meditation reduces anxiety",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	g := liveOAGetter{t: t}

	var all []probeTally
	for _, claim := range claims {
		tally := probeTally{claim: claim}
		works, _, err := fetchWorks(ctx, g, claim, "", "relevance_score:desc", probeLimit)
		if err != nil {
			t.Fatalf("fetchWorks(%q): %v", claim, err)
		}
		tally.fetched = len(works)
		iv, out := scengine.PICOTokens(claim)
		t.Logf("=== %s  (fetched %d, PICO iv=%v out=%v)", claim, len(works), iv, out)

		var exclIdx, keptIdx []int
		for i, w := range works {
			if probePasses(claim, w, iv, out) {
				keptIdx = append(keptIdx, i)
			} else {
				exclIdx = append(exclIdx, i)
			}
		}
		tally.kept, tally.excluded = len(keptIdx), len(exclIdx)

		// Candidate sets: only abstract-less works can be changed by a backfill.
		need := map[string][]int{} // normalized doi -> work indexes
		addCandidate := func(i int, isKept bool) {
			w := works[i]
			if strings.TrimSpace(w.Abstract) != "" {
				return
			}
			doi := strings.ToLower(strings.TrimSpace(w.DOI))
			if isKept {
				tally.keptNoAbs++
			} else {
				tally.exclNoAbs++
			}
			if doi == "" {
				if isKept {
					tally.keptNoAbsNoDOI++
				} else {
					tally.exclNoAbsNoDOI++
				}
				return
			}
			need[doi] = append(need[doi], i)
		}
		for _, i := range exclIdx {
			addCandidate(i, false)
		}
		for _, i := range keptIdx {
			addCandidate(i, true)
		}

		dois := make([]string, 0, len(need))
		for d := range need {
			dois = append(dois, d)
		}
		sort.Strings(dois)
		t.Logf("  abstract-less: excluded %d (%d w/o DOI), kept %d (%d w/o DOI); resolving %d DOI(s)",
			tally.exclNoAbs, tally.exclNoAbsNoDOI, tally.keptNoAbs, tally.keptNoAbsNoDOI, len(dois))

		filled := probePubMedAbstracts(ctx, t, dois)

		keptSet := map[int]bool{}
		for _, i := range keptIdx {
			keptSet[i] = true
		}
		for doi, idxs := range need {
			abs, ok := filled[doi]
			if !ok {
				continue
			}
			for _, i := range idxs {
				if keptSet[i] {
					tally.keptBackfilled++
				} else {
					tally.exclBackfilled++
				}
				w := works[i]
				w.Abstract = abs
				now := probePasses(claim, w, iv, out)
				switch {
				case !keptSet[i] && now:
					tally.returners++
					tally.returnerTitles = append(tally.returnerTitles, w.Title)
				case keptSet[i] && !now:
					tally.losers++
					tally.loserTitles = append(tally.loserTitles, w.Title)
				}
			}
		}
		t.Logf("  backfilled: %d of %d excluded, %d of %d kept", tally.exclBackfilled, tally.exclNoAbs, tally.keptBackfilled, tally.keptNoAbs)
		t.Logf("  RETURNERS %d   LOSERS %d   NET %+d", tally.returners, tally.losers, tally.returners-tally.losers)
		for _, s := range tally.returnerTitles {
			t.Logf("    + %s", truncProbe(s, 90))
		}
		for _, s := range tally.loserTitles {
			t.Logf("    - %s", truncProbe(s, 90))
		}
		all = append(all, tally)
	}

	var F, K, X, ena, kna, eb, kb, ret, los int
	for _, a := range all {
		F += a.fetched
		K += a.kept
		X += a.excluded
		ena += a.exclNoAbs
		kna += a.keptNoAbs
		eb += a.exclBackfilled
		kb += a.keptBackfilled
		ret += a.returners
		los += a.losers
	}
	t.Logf("")
	t.Logf("======== TOTAL over %d claims, --limit %d ========", len(all), probeLimit)
	t.Logf("fetched %d | currently kept %d | currently excluded %d", F, K, X)
	t.Logf("abstract-less among excluded %d -> backfilled %d", ena, eb)
	t.Logf("abstract-less among kept     %d -> backfilled %d", kna, kb)
	t.Logf("RETURNERS %d   LOSERS %d   NET %+d", ret, los, ret-los)
}

// TestLiveBackfillCapSizing measures the two quantities that set the per-run
// backfill cap: how many abstract-less candidates a MAXIMUM-size fetch actually
// produces, and what one DOI lookup costs in wall time at PubMed's keyless pace.
//
// A cap is only meaningful against both. Sized against candidate count alone it
// silently truncates real work; sized against latency alone it is either
// unreachable or pointlessly small.
func TestLiveBackfillCapSizing(t *testing.T) {
	if os.Getenv("SC_PROBE_LIVE") != "1" {
		t.Skip("live probe: set SC_PROBE_LIVE=1 to run")
	}
	claims := []string{
		"omega-3 improves cardiovascular health",
		"vitamin C prevents the common cold",
		"meditation reduces anxiety",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	g := liveOAGetter{t: t}

	const maxFetch = 200
	var sample []string
	totFetched, totCand, totExcl := 0, 0, 0
	for _, claim := range claims {
		works, _, err := fetchWorks(ctx, g, claim, "", "relevance_score:desc", maxFetch)
		if err != nil {
			t.Fatalf("fetchWorks(%q): %v", claim, err)
		}
		iv, out := scengine.PICOTokens(claim)
		cand, excl := 0, 0
		for _, w := range works {
			if strings.TrimSpace(w.Abstract) != "" || strings.TrimSpace(w.DOI) == "" {
				continue
			}
			cand++
			if !probePasses(claim, w, iv, out) {
				excl++
			}
			if len(sample) < 15 {
				sample = append(sample, strings.ToLower(strings.TrimSpace(w.DOI)))
			}
		}
		t.Logf("%-42s fetched %3d  backfill candidates %3d (%.1f%%)  of which gate-excluded %3d",
			claim, len(works), cand, 100*float64(cand)/float64(len(works)), excl)
		totFetched += len(works)
		totCand += cand
		totExcl += excl
	}
	t.Logf("TOTAL at --limit %d: fetched %d, candidates %d (%.1f%%), gate-excluded among them %d",
		maxFetch, totFetched, totCand, 100*float64(totCand)/float64(totFetched), totExcl)

	// Cost of one DOI -> PMID lookup at the keyless ~3 req/s pace the shipped
	// limiter enforces. probeGet already sleeps probePace before each call, so
	// this includes the pacing, which is what a cap has to budget for.
	start := time.Now()
	resolved := 0
	for _, d := range sample {
		u := fmt.Sprintf("%s/esearch.fcgi?db=pubmed&retmode=json&retmax=2&term=%s",
			probeEutils, url.QueryEscape(d+"[doi]"))
		raw, err := probeGet(ctx, u)
		if err != nil {
			continue
		}
		var r struct {
			ESearchResult struct {
				IDList []string `json:"idlist"`
			} `json:"esearchresult"`
		}
		if json.Unmarshal(raw, &r) == nil && len(r.ESearchResult.IDList) == 1 {
			resolved++
		}
	}
	per := time.Duration(0)
	if len(sample) > 0 {
		per = time.Since(start) / time.Duration(len(sample))
	}
	t.Logf("per-DOI esearch cost: %v over %d lookups (%d resolved)", per.Round(time.Millisecond), len(sample), resolved)
	for _, cap := range []int{25, 50, 75, 100} {
		t.Logf("  cap %3d -> worst-case backfill wall time ~%v", cap, (per * time.Duration(cap)).Round(time.Second))
	}
}
