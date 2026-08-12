package gaclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mvanhorn/printing-press-library/library/productivity/giustizia-amministrativa/internal/cliutil"
)

const (
	// BaseURL is the portal host serving the search form/action.
	BaseURL = "https://www.giustizia-amministrativa.it"
	// formPath is the "Decisioni e Pareri" search page (the handshake target).
	formPath = "/web/guest/dcsnprr"
	// portletPrefix is the Liferay namespace of the search portlet instance.
	portletPrefix = "_decisioni_pareri_web_DecisioniPareriWebPortlet_INSTANCE_XKc17mrB8J10"
	// portletID is the matching p_p_id value.
	portletID = "decisioni_pareri_web_DecisioniPareriWebPortlet_INSTANCE_XKc17mrB8J10"

	defaultUA       = "giustizia-amministrativa-pp-cli/0.1.0 (+https://github.com/aborruso)"
	defaultPageSize = 10
	// politeRate keeps requests gentle against a public institutional site.
	politeRate = 2.0
)

var rePAuth = regexp.MustCompile(`p_auth=([A-Za-z0-9]+)`)

// Client talks to the giustizia-amministrativa public search over plain HTTP,
// managing the Liferay session handshake (p_auth + affinity cookies).
type Client struct {
	http    *http.Client
	limiter *cliutil.AdaptiveLimiter
	ua      string

	mu    sync.Mutex
	pAuth string
}

// New returns a ready Client with a cookie jar and a polite adaptive limiter.
func New() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		http:    &http.Client{Jar: jar, Timeout: 30 * time.Second},
		limiter: cliutil.NewAdaptiveLimiter(politeRate),
		ua:      defaultUA,
	}
}

// SearchOptions describes a provvedimenti query.
type SearchOptions struct {
	Testo    string // simple full-text
	All      string // advanced: all of these words
	Any      string // advanced: any of these words
	Not      string // advanced: none of these words
	Phrase   string // advanced: exact phrase
	Tipo     string // sentenza|ordinanza|decreto|parere|plenaria|generale
	Sede     string // roma|milano|consiglio-di-stato|...
	Anno     int
	AnnoFrom int // year-range sweep: first year (inclusive)
	AnnoTo   int // year-range sweep: last year (inclusive)
	Numero   int
	Nrg      int
	AnnoNrg  int
	Limit    int // max results to return (per year when sweeping a year range)
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, int, error) {
	// Retry on transient throttling (429): the public institutional site rate-
	// limits bursts; back off and retry a few times before surfacing the error.
	const maxAttempts = 4
	var body []byte
	var status int
	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if c.limiter != nil {
			c.limiter.Wait()
		}
		body, status, err = c.doGet(ctx, rawURL)
		if err != nil {
			return nil, 0, err
		}
		if status == http.StatusTooManyRequests {
			if c.limiter != nil {
				c.limiter.OnRateLimit()
			}
			if attempt == maxAttempts {
				return body, status, &cliutil.RateLimitError{URL: rawURL, Body: "giustizia-amministrativa"}
			}
			if waitErr := sleepCtx(ctx, time.Duration(attempt)*2*time.Second); waitErr != nil {
				return nil, 0, waitErr
			}
			continue
		}
		if c.limiter != nil {
			c.limiter.OnSuccess()
		}
		return body, status, nil
	}
	return body, status, err
}

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", c.ua)
	req.Header.Set("Accept-Language", "it-IT,it;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

// sleepCtx waits for d or until ctx is cancelled.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// handshake fetches the form page to obtain the p_auth token and affinity
// cookies (stored in the jar). Safe to call repeatedly; refreshes the token.
func (c *Client) handshake(ctx context.Context) error {
	body, status, err := c.get(ctx, BaseURL+formPath)
	if err != nil {
		return fmt.Errorf("handshake: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("handshake: HTTP %d", status)
	}
	m := rePAuth.FindSubmatch(body)
	if m == nil {
		return fmt.Errorf("handshake: token p_auth non trovato nella pagina del form")
	}
	c.mu.Lock()
	c.pAuth = string(m[1])
	c.mu.Unlock()
	return nil
}

func (c *Client) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pAuth
}

// buildSearchURL constructs a paginated search action URL for page cur (1-based).
func (c *Client) buildSearchURL(opts SearchOptions, cur int) string {
	v := url.Values{}
	v.Set("p_p_id", portletID)
	v.Set("p_p_lifecycle", "1")
	v.Set("p_p_state", "normal")
	v.Set("p_p_mode", "view")
	v.Set(portletPrefix+"_javax.portlet.action", "search")
	v.Set("p_auth", c.token())

	p := func(name, val string) { v.Set(portletPrefix+"_"+name, val) }

	advanced := opts.All != "" || opts.Any != "" || opts.Not != "" || opts.Phrase != ""
	if advanced {
		p("isAdvancedSearch", "true")
		p("searchAllWords", opts.All)
		p("searchAnyWords", opts.Any)
		p("searchNotWords", opts.Not)
		p("searchPhrase", opts.Phrase)
	} else {
		p("isAdvancedSearch", "false")
		p("searchtextProvvedimenti", opts.Testo)
	}
	if t := mapTipo(opts.Tipo); t != "" {
		p("TipoProvvedimentoItem", t)
	}
	if s := mapSede(opts.Sede); s != "" {
		p("sedeProvvedimenti", s)
	}
	if opts.Anno != 0 {
		p("DataYearItem", strconv.Itoa(opts.Anno))
	}
	if opts.Numero != 0 {
		p("numeroProvvedimenti", strconv.Itoa(opts.Numero))
	}
	if opts.Nrg != 0 {
		p("numeroNrg", strconv.Itoa(opts.Nrg))
		p("asSearchMode", "nrg")
	} else {
		p("asSearchMode", "provv")
	}
	if opts.AnnoNrg != 0 {
		p("DataNrgItem", strconv.Itoa(opts.AnnoNrg))
	}
	p("pageSize", strconv.Itoa(defaultPageSize))
	p("changePage", "true")
	p("cur", strconv.Itoa(cur))
	return BaseURL + formPath + "?" + v.Encode()
}

// SearchResult bundles the rows of a search with the reported total. Warnings
// carries non-fatal notices (a year skipped during a sweep) for the caller to
// surface on stderr; results are still usable.
type SearchResult struct {
	Items    []Provvedimento
	Total    int
	Warnings []string
}

// Search runs a query and returns up to Limit results. It performs the session
// handshake on first use. When a year range (AnnoFrom/AnnoTo) is set it sweeps
// the years one by one — the portal has no relevance sort and only a single-year
// filter, so historical coverage requires iterating the year filter — applying
// Limit per year and de-duplicating the union by id.
func (c *Client) Search(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = defaultPageSize
	}
	if opts.Anno != 0 && (opts.AnnoFrom != 0 || opts.AnnoTo != 0) {
		return nil, fmt.Errorf("usa --anno oppure --anno-from/--anno-to, non entrambi")
	}
	if c.token() == "" {
		if err := c.handshake(ctx); err != nil {
			return nil, err
		}
	}
	if opts.AnnoFrom != 0 || opts.AnnoTo != 0 {
		return c.searchSweep(ctx, opts)
	}
	return c.searchOnce(ctx, opts)
}

// yearRange normalizes an inclusive year span: a missing bound mirrors the
// other, and a reversed span is swapped so from <= to.
func yearRange(from, to int) (int, int) {
	if from == 0 {
		from = to
	}
	if to == 0 {
		to = from
	}
	if from > to {
		from, to = to, from
	}
	return from, to
}

// dedupKey identifies a provvedimento for de-duplication: ECLI, else idprovv,
// else the document coordinates (schema|nrg|nome_file). It never returns "" for
// a real result, so items lacking an ECLI/idprovv still de-duplicate instead of
// being appended once per swept year.
func dedupKey(p Provvedimento) string {
	if p.Ecli != "" {
		return p.Ecli
	}
	if p.Idprovv != "" {
		return p.Idprovv
	}
	return p.Schema + "|" + p.Nrg + "|" + p.NomeFile
}

// fatalSweepError reports whether an error must abort a whole sweep instead of
// skipping the failing year: rate limiting (further years would only hit more
// of it) and a cancelled/expired context.
func fatalSweepError(ctx context.Context, err error) bool {
	var rle *cliutil.RateLimitError
	if errors.As(err, &rle) {
		return true
	}
	return ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// searchSweep iterates the year filter from AnnoFrom to AnnoTo (inclusive),
// running searchOnce per year and de-duplicating the union by dedupKey. Limit
// applies per year. Total is the sum of per-year totals.
//
// A transient failure on one year (timeout, network) does not discard the years
// already collected: that year is skipped and reported in Warnings. Rate limits
// and a cancelled context abort the sweep, and an error is returned only when no
// year succeeded at all.
func (c *Client) searchSweep(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	from, to := yearRange(opts.AnnoFrom, opts.AnnoTo)
	yearOpts := opts
	yearOpts.AnnoFrom, yearOpts.AnnoTo = 0, 0
	return sweepYears(ctx, from, to, func(y int) (*SearchResult, error) {
		yearOpts.Anno = y
		return c.searchOnce(ctx, yearOpts)
	})
}

// appendSkippedWarning records the years dropped by a transient failure, so a
// gap in the swept range is never silent — including when the sweep later
// aborts for a different reason.
func appendSkippedWarning(warnings, skipped []string, lastErr error) []string {
	if len(skipped) == 0 {
		return warnings
	}
	return append(warnings, fmt.Sprintf("anni non recuperati: %s (ultimo errore: %v)", strings.Join(skipped, ", "), lastErr))
}

// sweepYears merges the per-year results of fetch over an inclusive year span,
// applying the skip/abort policy described on searchSweep.
func sweepYears(ctx context.Context, from, to int, fetch func(year int) (*SearchResult, error)) (*SearchResult, error) {
	res := &SearchResult{}
	seen := map[string]bool{}
	var skipped []string
	var lastErr error
	for y := from; y <= to; y++ {
		part, err := fetch(y)
		if err != nil {
			if fatalSweepError(ctx, err) {
				if len(res.Items) == 0 {
					return nil, err
				}
				res.Warnings = append(res.Warnings, fmt.Sprintf("sweep interrotto all'anno %d: %v; risultati parziali dagli anni %d-%d", y, err, from, y-1))
				res.Warnings = appendSkippedWarning(res.Warnings, skipped, lastErr)
				return res, nil
			}
			lastErr = err
			skipped = append(skipped, strconv.Itoa(y))
			continue
		}
		res.Total += part.Total
		for _, p := range part.Items {
			key := dedupKey(p)
			if seen[key] {
				continue
			}
			seen[key] = true
			res.Items = append(res.Items, p)
		}
	}
	if len(skipped) > 0 && len(res.Items) == 0 {
		return nil, lastErr
	}
	res.Warnings = appendSkippedWarning(res.Warnings, skipped, lastErr)
	return res, nil
}

// searchOnce paginates a single query until Limit results are collected,
// retrying once on a 403 (expired token).
func (c *Client) searchOnce(ctx context.Context, opts SearchOptions) (*SearchResult, error) {
	res := &SearchResult{}
	maxPages := (opts.Limit + defaultPageSize - 1) / defaultPageSize
	for page := 1; page <= maxPages; page++ {
		body, status, err := c.get(ctx, c.buildSearchURL(opts, page))
		if err != nil {
			if rle, ok := err.(*cliutil.RateLimitError); ok {
				return nil, rle
			}
			return nil, err
		}
		if status == http.StatusForbidden {
			// Expired/stale token: refresh the handshake and retry this page once.
			if herr := c.handshake(ctx); herr != nil {
				return nil, herr
			}
			body, status, err = c.get(ctx, c.buildSearchURL(opts, page))
			if err != nil {
				return nil, err
			}
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("ricerca: HTTP %d", status)
		}
		text := string(body)
		if page == 1 {
			res.Total = ParseTotal(text)
		}
		items := ParseResults(text)
		if len(items) == 0 {
			break
		}
		res.Items = append(res.Items, items...)
		if len(res.Items) >= opts.Limit {
			res.Items = res.Items[:opts.Limit]
			break
		}
	}
	return res, nil
}

// FullText fetches the raw HTML of a single provvedimento document. It uses
// p.URL when present, otherwise rebuilds it from schema/nrg/nome_file.
func (c *Client) FullText(ctx context.Context, p Provvedimento) (string, error) {
	docURL := p.URL
	if docURL == "" {
		if p.Schema == "" || p.Nrg == "" || p.NomeFile == "" {
			return "", fmt.Errorf("dati insufficienti per costruire l'URL del documento (servono schema, nrg, nome_file)")
		}
		docURL = DocURL(p.Schema, p.Nrg, p.NomeFile)
	}
	body, status, err := c.get(ctx, docURL)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("testo integrale: HTTP %d", status)
	}
	return string(body), nil
}

// DocURL builds the public full-text URL for a provvedimento.
func DocURL(schema, nrg, nomeFile string) string {
	v := url.Values{}
	v.Set("nodeRef", "")
	v.Set("schema", schema)
	v.Set("nrg", nrg)
	v.Set("nomeFile", nomeFile)
	v.Set("subDir", "Provvedimenti")
	return "https://mdp.giustizia-amministrativa.it/visualizzah2/?" + v.Encode()
}

// mapTipo translates a CLI-friendly tipo into the portal option value.
func mapTipo(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "", "tutti", "all":
		return ""
	case "sentenza", "sentenze":
		return "Sentenza"
	case "ordinanza", "ordinanze":
		return "Ordinanza"
	case "decreto", "decreti":
		return "Decreto"
	case "parere", "pareri":
		return "Parere"
	case "plenaria", "adunanza-plenaria":
		return "P"
	case "generale", "adunanza-generale":
		return "C"
	default:
		return ""
	}
}

// sedeMap maps CLI-friendly sede slugs to portal option values.
var sedeMap = map[string]string{
	"consiglio-di-stato": "Consiglio di Stato",
	"cds":                "Consiglio di Stato",
	"cgars":              "C.G.A.R.S",
	"ancona":             "Ancona", "aosta": "Aosta", "bari": "Bari", "bologna": "Bologna",
	"bolzano": "Bolzano", "brescia": "Brescia", "cagliari": "Cagliari", "campobasso": "Campobasso",
	"catania": "Catania", "catanzaro": "Catanzaro", "firenze": "Firenze", "genova": "Genova",
	"laquila": "L'Aquila", "l-aquila": "L'Aquila", "latina": "Latina", "lecce": "Lecce",
	"milano": "Milano", "napoli": "Napoli", "palermo": "Palermo", "parma": "Parma",
	"perugia": "Perugia", "pescara": "Pescara", "potenza": "Potenza",
	"reggio-calabria": "Reggio Calabria", "roma": "Roma", "salerno": "Salerno",
	"torino": "Torino", "trento": "Trento", "trieste": "Trieste", "venezia": "Venezia",
}

func mapSede(s string) string {
	key := strings.ToLower(strings.TrimSpace(s))
	if key == "" {
		return ""
	}
	if v, ok := sedeMap[key]; ok {
		return v
	}
	// Accept an already-correct portal value as-is.
	return strings.TrimSpace(s)
}
