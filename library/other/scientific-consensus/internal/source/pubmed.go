package source

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/mvanhorn/printing-press-library/library/other/scientific-consensus/internal/cliutil"
)

const eutils = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils"

type pubmedSource struct {
	limiter *cliutil.AdaptiveLimiter
}

func init() {
	// Keyless PubMed allows ~3 req/s; NCBI_API_KEY raises to ~10.
	rate := 3.0
	if os.Getenv("NCBI_API_KEY") != "" {
		rate = 9.0
	}
	Register(&pubmedSource{limiter: cliutil.NewAdaptiveLimiter(rate)})
}

func (p *pubmedSource) Name() string { return "pubmed" }
func (p *pubmedSource) Description() string {
	return "PubMed E-utilities — biomedical literature + authoritative MeSH publication types"
}
func (p *pubmedSource) AuthRequired() bool     { return false }
func (p *pubmedSource) OptionalKeyEnv() string { return "NCBI_API_KEY" }

func apiKeyParam() string {
	if k := os.Getenv("NCBI_API_KEY"); k != "" {
		return "&api_key=" + url.QueryEscape(k)
	}
	return ""
}

func (p *pubmedSource) Sync(ctx context.Context, opts SyncOptions) ([]Work, error) {
	if opts.Limit <= 0 {
		opts.Limit = 25
	}
	ids, err := pubmedSearch(ctx, p.limiter, opts.Query, opts.Limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	summaries, err := pubmedSummaries(ctx, p.limiter, ids)
	if err != nil {
		return nil, err
	}
	out := make([]Work, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, Work{
			Source:   "pubmed",
			SourceID: s.UID,
			Title:    s.Title,
			Year:     s.Year(),
			DOI:      s.DOI(),
			URL:      "https://pubmed.ncbi.nlm.nih.gov/" + s.UID + "/",
		})
	}
	return out, nil
}

func pubmedSearch(ctx context.Context, lim *cliutil.AdaptiveLimiter, query string, retmax int) ([]string, error) {
	u := fmt.Sprintf("%s/esearch.fcgi?db=pubmed&retmode=json&retmax=%d&term=%s%s",
		eutils, retmax, url.QueryEscape(query), apiKeyParam())
	var resp struct {
		ESearchResult struct {
			IDList []string `json:"idlist"`
		} `json:"esearchresult"`
	}
	if err := getJSON(ctx, lim, u, &resp); err != nil {
		return nil, err
	}
	return resp.ESearchResult.IDList, nil
}

type pubmedSummary struct {
	UID        string   `json:"uid"`
	Title      string   `json:"title"`
	PubDate    string   `json:"pubdate"`
	PubType    []string `json:"pubtype"`
	ArticleIDs []struct {
		IDType string `json:"idtype"`
		Value  string `json:"value"`
	} `json:"articleids"`
}

func (s pubmedSummary) Year() int {
	f := strings.Fields(s.PubDate)
	if len(f) == 0 {
		return 0
	}
	var y int
	fmt.Sscanf(f[0], "%d", &y)
	return y
}

func (s pubmedSummary) DOI() string {
	for _, a := range s.ArticleIDs {
		if a.IDType == "doi" {
			return a.Value
		}
	}
	return ""
}

func pubmedSummaries(ctx context.Context, lim *cliutil.AdaptiveLimiter, ids []string) ([]pubmedSummary, error) {
	u := fmt.Sprintf("%s/esummary.fcgi?db=pubmed&retmode=json&id=%s%s",
		eutils, strings.Join(ids, ","), apiKeyParam())
	var raw struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := getJSON(ctx, lim, u, &raw); err != nil {
		return nil, err
	}
	uidsRaw, ok := raw.Result["uids"]
	if !ok {
		return nil, nil
	}
	var uids []string
	_ = json.Unmarshal(uidsRaw, &uids)
	out := make([]pubmedSummary, 0, len(uids))
	for _, uid := range uids {
		if r, ok := raw.Result[uid]; ok {
			var s pubmedSummary
			if json.Unmarshal(r, &s) == nil {
				out = append(out, s)
			}
		}
	}
	return out, nil
}

// PubMedPubTypes returns authoritative MeSH publication types keyed by PMID.
// Best-effort: returns whatever it can resolve. Uses the registered pubmed
// source's limiter when available.
func PubMedPubTypes(ctx context.Context, pmids []string) (map[string][]string, error) {
	if len(pmids) == 0 {
		return nil, nil
	}
	var lim *cliutil.AdaptiveLimiter
	if s, ok := Lookup("pubmed"); ok {
		if ps, ok := s.(*pubmedSource); ok {
			lim = ps.limiter
		}
	}
	summaries, err := pubmedSummaries(ctx, lim, pmids)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]string, len(summaries))
	for _, s := range summaries {
		out[s.UID] = s.PubType
	}
	return out, nil
}

// pubmedLimiter returns the registered pubmed source's limiter, or nil when the
// source is not registered. Every AdaptiveLimiter method is nil-safe, so a nil
// limiter means "unpaced", not "broken".
func pubmedLimiter() *cliutil.AdaptiveLimiter {
	if s, ok := Lookup("pubmed"); ok {
		if ps, ok := s.(*pubmedSource); ok {
			return ps.limiter
		}
	}
	return nil
}

// pubmedFetchBatch is how many PMIDs go into one efetch call. E-utilities
// accepts several hundred; 100 keeps a single response comfortably inside the
// 8 MiB read cap in getXML even for long structured abstracts.
const pubmedFetchBatch = 100

// pubmedFetchSet is the efetch `retmode=xml` envelope.
//
// The nesting in these struct tags is load-bearing, not decoration. A
// PubmedArticle carries ArticleId and AbstractText elements in places OTHER
// than the article's own record: every <ReferenceList> entry has its own
// <ArticleIdList>, and translated abstracts live under <OtherAbstract>.
// Measured 2026-08-02 against the live API on 41 records, a parser that
// collected ArticleId from anywhere in the subtree mapped 8 of them to a CITED
// paper's DOI instead of their own — which would have attached a completely
// unrelated abstract to 20% of the works it backfilled. Path-scoped tags make
// that failure unrepresentable rather than merely unlikely.
type pubmedFetchSet struct {
	Articles []pubmedFetchArticle `xml:"PubmedArticle"`
}

type pubmedFetchArticle struct {
	PMID string `xml:"MedlineCitation>PMID"`
	// The article's own abstract only — deliberately NOT OtherAbstract.
	Abstract []pubmedAbstractPart `xml:"MedlineCitation>Article>Abstract>AbstractText"`
}

// pubmedAbstractPart is one section of a possibly structured abstract. Label is
// "BACKGROUND", "METHODS", … on structured abstracts and empty on flat ones.
//
// Inner is captured as innerxml rather than chardata because AbstractText may
// contain inline markup: measured 2026-08-02, 5 of 59 AbstractText elements in
// the backfill probe carried <sup>/<sub>. Go's `,chardata` collects only an
// element's DIRECT character data, so those records would silently lose the
// nested text ("CO<sub>2</sub>" becomes "CO"). flattenAbstractXML re-decodes the
// fragment instead, which also resolves entities such as &#xae;.
type pubmedAbstractPart struct {
	Label string `xml:"Label,attr"`
	Inner string `xml:",innerxml"`
}

// Text renders one section as plain text, prefixing the section label when the
// abstract is structured so "BACKGROUND: …" survives into the classifiers.
func (p pubmedAbstractPart) Text() string {
	body := flattenAbstractXML(p.Inner)
	if body == "" {
		return ""
	}
	if label := strings.TrimSpace(p.Label); label != "" {
		return label + ": " + body
	}
	return body
}

// flattenAbstractXML turns an AbstractText fragment into plain text, keeping the
// text of nested inline elements and resolving character entities. Whitespace is
// collapsed so the result reads the same whether the source was pretty-printed
// or not.
//
// Malformed input yields whatever decoded before the error rather than an error:
// this is best-effort enrichment, and a partial abstract still serves the
// relevance and PICO gates better than no abstract at all.
func flattenAbstractXML(inner string) string {
	if inner == "" {
		return ""
	}
	dec := xml.NewDecoder(strings.NewReader("<pp-abstract>" + inner + "</pp-abstract>"))
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
	return strings.Join(strings.Fields(b.String()), " ")
}

// PubMedAbstractsByDOI resolves DOIs to PubMed abstract text, keyed by the
// NORMALIZED DOI (see NormDOI) so callers can join on it directly.
//
// Best-effort by contract. A DOI that PubMed does not index, that resolves
// ambiguously, or that has no abstract on file is simply absent from the map; it
// is never an error, and one unresolvable DOI never voids the rest of the batch.
// The returned error reports the FIRST transport-level failure so a caller may
// surface it, but the map is always usable regardless of the error — callers
// that treat enrichment as optional can ignore it entirely.
//
// Why one esearch per DOI instead of one OR-ed query per batch: measured
// 2026-08-02 against the live API on the 51 abstract-less works in
// scengine/testdata/corpora_full, per-DOI queries resolve 41 (80.4%) while
// OR-ing 20 `<doi>[doi]` terms into a single query resolves only 20 (39.2%).
// Batching is ~20x cheaper in requests and silently loses half the yield, so the
// per-DOI cost buys the result rather than paying for nothing. One lookup costs
// ~631 ms at the keyless ~3 req/s pace, so CALLERS MUST BOUND len(dois) — this
// function deliberately has no internal cap, because only the caller knows which
// DOIs are worth the budget.
func PubMedAbstractsByDOI(ctx context.Context, dois []string) (map[string]string, error) {
	if len(dois) == 0 {
		return nil, nil
	}
	lim := pubmedLimiter()

	doiForPMID := make(map[string]string, len(dois))
	pmids := make([]string, 0, len(dois))
	seen := make(map[string]bool, len(dois))
	var firstErr error

	for _, raw := range dois {
		if ctx.Err() != nil {
			break
		}
		d := normDOI(raw)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		pmid, err := pubmedPMIDForDOI(ctx, lim, d)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Two DOIs resolving to one PMID would leave the abstract's owner
		// ambiguous, so the first claim wins and the second is dropped.
		if pmid == "" || doiForPMID[pmid] != "" {
			continue
		}
		doiForPMID[pmid] = d
		pmids = append(pmids, pmid)
	}

	out := make(map[string]string, len(pmids))
	for i := 0; i < len(pmids); i += pubmedFetchBatch {
		if ctx.Err() != nil {
			break
		}
		end := i + pubmedFetchBatch
		if end > len(pmids) {
			end = len(pmids)
		}
		byPMID, err := pubmedAbstracts(ctx, lim, pmids[i:end])
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for pmid, abs := range byPMID {
			if d := doiForPMID[pmid]; d != "" {
				out[d] = abs
			}
		}
	}
	return out, firstErr
}

// pubmedPMIDForDOI resolves ONE normalized DOI to a PMID through esearch's [doi]
// field. Returns "" when PubMed holds no record — and equally when it holds more
// than one, because an ambiguous match cannot be attributed and attaching the
// wrong abstract to a work is worse than leaving that work without one.
func pubmedPMIDForDOI(ctx context.Context, lim *cliutil.AdaptiveLimiter, doi string) (string, error) {
	ids, err := pubmedSearch(ctx, lim, doi+"[doi]", 2)
	if err != nil {
		return "", err
	}
	if len(ids) != 1 {
		return "", nil
	}
	return ids[0], nil
}

// pubmedAbstracts fetches abstracts for a batch of PMIDs, keyed by PMID. A
// record that carries no abstract is absent from the map rather than mapped to
// the empty string, so callers cannot confuse "no abstract" with "not fetched".
func pubmedAbstracts(ctx context.Context, lim *cliutil.AdaptiveLimiter, pmids []string) (map[string]string, error) {
	if len(pmids) == 0 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/efetch.fcgi?db=pubmed&retmode=xml&id=%s%s",
		eutils, url.QueryEscape(strings.Join(pmids, ",")), apiKeyParam())
	var set pubmedFetchSet
	if err := getXML(ctx, lim, u, &set); err != nil {
		return nil, err
	}
	return abstractsFromFetchSet(set), nil
}

// abstractsFromFetchSet renders a parsed efetch payload into PMID -> abstract.
// Split out of pubmedAbstracts so the XML contract above can be tested against
// fixed fixtures without touching the network.
func abstractsFromFetchSet(set pubmedFetchSet) map[string]string {
	out := make(map[string]string, len(set.Articles))
	for _, art := range set.Articles {
		pmid := strings.TrimSpace(art.PMID)
		if pmid == "" {
			continue
		}
		parts := make([]string, 0, len(art.Abstract))
		for _, p := range art.Abstract {
			if txt := p.Text(); txt != "" {
				parts = append(parts, txt)
			}
		}
		if len(parts) == 0 {
			continue
		}
		out[pmid] = strings.Join(parts, " ")
	}
	return out
}

// getXML performs a paced GET and decodes an XML body into v. It mirrors
// getJSON — including the 429 -> *cliutil.RateLimitError contract, so a throttle
// is never mistaken for "this work has no abstract" — and lives here rather than
// in source.go because PubMed's efetch is the only XML endpoint in this package.
// // pp:client-call
func getXML(ctx context.Context, limiter *cliutil.AdaptiveLimiter, url string, v any) error {
	limiter.Wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/xml")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		limiter.OnRateLimit()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return &cliutil.RateLimitError{URL: redactURL(url), Body: string(body)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("GET %s: HTTP %d: %s", redactURL(url), resp.StatusCode, string(body))
	}
	limiter.OnSuccess()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if v == nil {
		return nil
	}
	return xml.Unmarshal(data, v)
}
