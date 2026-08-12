package icaroclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/printing-press-library/library/other/ars-sicilia/internal/cliutil"
)

// DefaultBaseURL points at the production data portal.
const DefaultBaseURL = "https://dati.ars.sicilia.it"

// DefaultUserAgent is sent with every request so the portal team can identify
// the CLI in their logs.
const DefaultUserAgent = "ars-sicilia-pp-cli/0.1.0 (+https://github.com/aborruso/ars-trasparente)"

// HTTPRateLimitError is returned by the icaroclient when the portal
// responds with HTTP 429 Too Many Requests. Callers can check for this
// type to surface a rate-limit-specific exit code (7) instead of a
// generic error exit (1).
type HTTPRateLimitError struct {
	URL string
}

func (e *HTTPRateLimitError) Error() string {
	return fmt.Sprintf("rate limited (HTTP 429) from ARS portal: %s", e.URL)
}

// DefaultRateLimit is the per-session request rate applied to the Icaro
// portal unless the caller disables pacing (rateLimit <= 0). The ARS
// portal is a legacy JSP application with no documented rate-limit policy;
// 2 req/s matches the top-level CLI default and is conservative enough
// to avoid session throttling.
const DefaultRateLimit = 2.0

// Client wraps the multi-step Icaro session flow:
//
//  1. GET /icaro/default.jsp?icaDB=NNN&icaQuery=<expr> establishes the session
//     and assigns icaQueryId=1 server-side.
//  2. GET /icaro/shortList.jsp[?setPage=N] returns paginated rows.
//  3. GET /icaro/doc<NNN>-1.jsp?icaQueryId=1&icaDocId=M returns the full doc.
//
// Cookies (JSESSIONID) are kept in a jar bound to this Client instance.
type Client struct {
	BaseURL    string
	UserAgent  string
	HTTPClient *http.Client
	limiter    *cliutil.AdaptiveLimiter
}

// Record is one short-list row. Fields are positional + free text — the
// archive's Columns slice names them in display order.
type Record struct {
	DocID   int               `json:"doc_id"`
	Fields  map[string]string `json:"fields"`
	Title   string            `json:"title"`
	Excerpt string            `json:"excerpt,omitempty"`
	URL     string            `json:"url,omitempty"`
}

// Doc is the parsed body of a single document page.
//
// DocID e URL non identificano il documento: `icaDocId` è la posizione nella
// short list della sessione corrente, quindi lo stesso valore punta a un
// documento diverso appena cambia la query, e fuori sessione l'URL risponde
// 302. Il portale un identificatore stabile ce l'ha — è il `docno(N)` con cui
// costruisce il proprio permalink — e sta in DocNo/Permalink: quelli si
// possono citare, salvare e riaprire.
type Doc struct {
	DocID int `json:"doc_id"`
	// DocNo è il numero di documento interno del portale, stabile nel tempo.
	DocNo int `json:"docno,omitempty"`
	// Permalink riapre il documento in una sessione nuova. È l'unico URL di
	// questo struct che ha senso conservare.
	Permalink string            `json:"permalink,omitempty"`
	Title     string            `json:"title"`
	Fields    map[string]string `json:"fields"`
	Body      string            `json:"body"`
	URL       string            `json:"url"`
}

// SearchOptions tunes a single search run.
type SearchOptions struct {
	// Params is the friendly flag map (legisl/anno/firmatario/...).
	Params map[string]string
	// ISISRaw bypasses BuildQuery and ships the expression verbatim.
	ISISRaw string
	// MaxPages caps the number of shortList pages to fetch. 0 means "fetch
	// the first page only" — typical interactive case.
	MaxPages int
	// Limit is a max-records ceiling honored after collecting pages.
	Limit int
	// Truncated, when non-nil, is set by Search to report whether the archive
	// held more matching records than are being returned — whether because
	// Limit cut the set short or because MaxPages left later pages unread.
	// Callers that render len(records) as a de facto total (deputato profilo,
	// commissione dossier) use this to flag undercounts instead of silently
	// presenting a capped count as complete.
	Truncated *bool
	// ForceIcaro pins the search to the legacy Icaro engine even for archives
	// migrated to /bd/. The `get` path needs it: /bd/ rows carry no Icaro DocID
	// and the /bd/ per-document detail is not implemented, so GetDoc must run on
	// the Icaro DocID. On /bd/ archives this only finds records still present in
	// Icaro's (frozen) index; recent records return not-found, which is correct.
	ForceIcaro bool
	// StopWhen, when non-nil, is consulted after each page with everything
	// collected so far: returning true ends pagination. It exists because Limit
	// counts ROWS, and in the leggi archive (indexed per article) the unit the
	// caller wants is the law — a count knowable only after collapsing the rows
	// fetched so far, never in advance. Estimating "10 laws ≈ 100 rows" up front
	// is what made `leggi cerca --anno 2025` answer 4 laws out of 31: the year's
	// first laws are the budget ones, ~25 article-rows each, and they ate the
	// window. With the predicate the window stops on the unit that was asked for,
	// so it also ends EARLIER than the estimate when the laws are short.
	//
	// Only the Icaro loop below honors it: /bd/ archives return from searchBD
	// before it, and none of them is indexed per article.
	StopWhen func([]Record) bool
}

// New constructs a Client with a fresh cookie jar and a 30 s default timeout.
// Pass nil to use http.DefaultClient parameters with a jar. The client paces
// outbound requests at DefaultRateLimit req/s using an adaptive limiter.
func New(httpClient *http.Client) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	} else {
		clone := *httpClient
		httpClient = &clone
	}
	httpClient.Jar = jar
	return &Client{
		BaseURL:    DefaultBaseURL,
		UserAgent:  DefaultUserAgent,
		HTTPClient: httpClient,
		limiter:    cliutil.NewAdaptiveLimiter(DefaultRateLimit),
	}, nil
}

// Search runs the bootstrap + shortList loop and returns the parsed records.
// Cancellation propagates via ctx.
func (c *Client) Search(ctx context.Context, arc Archive, opts SearchOptions) ([]Record, error) {
	if c == nil {
		return nil, fmt.Errorf("nil icaroclient.Client")
	}
	// Gli archivi delle sedute migrati al backend /bd/ (sommari, resoconti,
	// convocazioni) hanno l'indice Icaro congelato: instradiamo qui, dove i dati
	// sono correnti. Gli altri archivi restano sul flusso Icaro sotto. Il path
	// `get` passa ForceIcaro perché il dettaglio per-documento su /bd/ non esiste
	// e serve il DocID Icaro (vedi SearchOptions.ForceIcaro).
	if IsBDArchive(arc.Slug) && !opts.ForceIcaro {
		return c.searchBD(ctx, arc, opts)
	}
	expr := BuildQuery(arc, opts.Params, opts.ISISRaw)
	if err := c.bootstrapSession(ctx, arc.ID, expr); err != nil {
		return nil, err
	}
	maxPages := opts.MaxPages
	if maxPages <= 0 {
		maxPages = 1
	}
	var all []Record
	lastPage, lastTotalPages := 0, 0
	// droppedByLimit records whether Limit actually cut records off the set we
	// fetched. It is tracked separately from pagination because the two hide
	// different halves of the same question: stopping early leaves whole pages
	// unread (lastPage < lastTotalPages), while Limit can also slice away rows
	// of the LAST page — in which case no page is left unread and pagination
	// alone reports nothing was dropped.
	droppedByLimit := false
	for page := 1; page <= maxPages; page++ {
		rows, totalPages, err := c.fetchPage(ctx, arc, page)
		if err != nil {
			return all, err
		}
		all = append(all, rows...)
		lastPage, lastTotalPages = page, totalPages
		if opts.Limit > 0 && len(all) >= opts.Limit {
			droppedByLimit = len(all) > opts.Limit
			all = all[:opts.Limit]
			break
		}
		// Dopo il taglio per Limit, non prima: il predicato deve giudicare le
		// righe che il chiamante riceverà davvero.
		if opts.StopWhen != nil && opts.StopWhen(all) {
			break
		}
		if page >= totalPages {
			break
		}
	}
	if opts.Limit > 0 && len(all) > opts.Limit {
		droppedByLimit = true
		all = all[:opts.Limit]
	}
	if opts.Truncated != nil {
		*opts.Truncated = droppedByLimit || lastPage < lastTotalPages
	}
	return all, nil
}

// GetDoc fetches and parses the document body for a previously-searched item.
// The session established by Search MUST still be valid when GetDoc runs;
// callers that just want one record should call Search with a query that
// narrows to that record, then GetDoc on its DocID.
func (c *Client) GetDoc(ctx context.Context, arc Archive, docID int) (Doc, error) {
	docURL := fmt.Sprintf("%s/icaro/doc%s-1.jsp?icaQueryId=1&icaDocId=%d&_=%d",
		c.BaseURL, arc.ID, docID, time.Now().UnixMilli())
	body, err := c.get(ctx, docURL)
	if err != nil {
		return Doc{}, fmt.Errorf("fetching document %d (%s): %w", docID, arc.Slug, err)
	}
	doc, err := ParseDoc(body, arc, docID)
	if err != nil {
		return Doc{}, err
	}
	doc.URL = docURL
	if doc.DocNo > 0 {
		doc.Permalink = PermalinkURL(c.BaseURL, arc.ID, doc.DocNo)
	}
	return doc, nil
}

// PermalinkURL costruisce il link stabile a un documento: è la stessa query
// `docno(N)` che il portale mette dietro il proprio bottone «Link diretto al
// documento», e riapre quel documento in una sessione nuova.
func PermalinkURL(baseURL, archiveID string, docNo int) string {
	q := url.Values{}
	q.Set("icaDB", archiveID)
	q.Set("icaQuery", fmt.Sprintf("docno(%d)", docNo))
	return baseURL + "/icaro/default.jsp?" + q.Encode()
}

// bootstrapSession establishes a fresh server-side query state. icaQueryId is
// always 1 after this call.
func (c *Client) bootstrapSession(ctx context.Context, archiveID, queryExpr string) error {
	q := url.Values{}
	q.Set("icaDB", archiveID)
	q.Set("icaQuery", queryExpr)
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	bootURL := c.BaseURL + "/icaro/default.jsp?" + q.Encode()
	_, err := c.get(ctx, bootURL)
	if err != nil {
		return fmt.Errorf("bootstrap session (archive %s): %w", archiveID, err)
	}
	return nil
}

// fetchPage requests one shortList page and parses its rows; also extracts
// the total page count from the pagination block.
func (c *Client) fetchPage(ctx context.Context, arc Archive, page int) ([]Record, int, error) {
	q := url.Values{}
	if page > 1 {
		q.Set("setPage", strconv.Itoa(page))
	}
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	pageURL := c.BaseURL + "/icaro/shortList.jsp?" + q.Encode()
	body, err := c.get(ctx, pageURL)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch shortList page %d: %w", page, err)
	}
	rows, totalPages, err := ParseShortList(body, arc, c.BaseURL)
	if err != nil {
		return nil, totalPages, err
	}
	return rows, totalPages, nil
}

// get issues a GET against the URL using the client's session jar.
// The adaptive limiter paces requests and backs off on 429 responses.
func (c *Client) get(ctx context.Context, rawURL string) (string, error) {
	c.limiter.Wait()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	req.Header.Set("Accept-Language", "it-IT,it;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 429 {
		c.limiter.OnRateLimit()
		return "", &HTTPRateLimitError{URL: rawURL}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("unexpected status %d for %s", resp.StatusCode, rawURL)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	c.limiter.OnSuccess()
	return string(raw), nil
}

// Source returns the URL that would have been used as the entry point in a
// browser, useful for `url` fields on records (so users can click through).
func (c *Client) Source(archiveID, queryExpr string) string {
	q := url.Values{}
	q.Set("icaDB", archiveID)
	q.Set("icaQuery", queryExpr)
	return c.BaseURL + "/icaro/default.jsp?" + q.Encode()
}

// JoinFields is a tiny helper to make a CSV string out of selected record
// fields, useful when building human-friendly summaries.
func JoinFields(r Record, keys ...string) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if v, ok := r.Fields[k]; ok {
			out = append(out, v)
		}
	}
	return strings.Join(out, " · ")
}
