package sources

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
	"time"
)

// Grants.gov Search2 API — open federal funding opportunities.
// Keyless. Dates in the results use MM/DD/YYYY.

const (
	grantsSearchURL = "https://api.grants.gov/v1/api/search2"
	grantsFetchURL  = "https://api.grants.gov/v1/api/fetchOpportunity"
)

// grantsSortNewestFirst orders results by posting date, newest first.
//
// "posted" means the record was published and never closed, not that the
// opportunity is still accepting applications. Grants.gov leaves stale records
// in that state indefinitely: measured on 2026-08-11 for the keyword
// "climate", the 66 posted results included a NOAA call opened in July 2011
// for fiscal year 2012, and 8 of the 66 predated 2022. Unsorted, those records
// surface on the first page ahead of live ones.
//
// Sorting rather than filtering is deliberate. A date filter would discard
// them: the API's dateRange parameter cuts the same 66 results to 27 at one
// year and to 7 at 30 days, and some genuinely rolling programs opened years
// ago and still accept applications. Sorting keeps every result reachable
// while putting the live ones first — measured, the top ten then all opened
// within the current year and all carried real deadlines.
//
// Verified to take effect rather than being silently ignored: "openDate|asc"
// returns the 2011 record first, "openDate|desc" returns one opened the
// previous day. By contrast openDateFrom is accepted and has no effect (the
// hit count is unchanged), so it must not be used.
const grantsSortNewestFirst = "openDate|desc"

// grantsStaleAfter is how old a posting may be before it is worth flagging.
//
// Two years is past every annual funding cycle, so a still-posted record older
// than that is more likely forgotten than rolling. This drives a caveat in the
// output, never a filter.
const grantsStaleAfter = 2 * 365 * 24 * time.Hour

// grantsDateLayout is the MM/DD/YYYY format the API returns.
const grantsDateLayout = "01/02/2006"

// normalizeText collapses interior whitespace runs to a single space and trims
// the ends.
//
// Grants.gov ships raw agency-entered text. Measured on 2026-09-03 for the
// keyword "climate" (48 results): three titles carried doubled interior
// spaces — "Egypt  Annual Program Statement" (DFOP0018819), "Science &
// Technology  Broad Agency Announcement" (HM047623BAA0001) and "Notice of
// Intent:  Program to End Modern Slavery" (SFOP0008547) — and two agency
// names carried a trailing space: "National Geospatial-Intelligence Agency "
// (HM047623BAA0001) and "DOT Federal Highway Administration "
// (693JJ323NF00014).
//
// strings.Fields rather than a regexp, for two reasons. This module has no
// external dependencies and stdlib keeps it that way; and Fields splits on
// unicode.IsSpace, so it also folds the non-breaking space that
// html.UnescapeString produces from &nbsp;. Go's regexp \s class is ASCII-only
// and would leave U+00A0 in place.
func normalizeText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

type Opportunity struct {
	ID         json.Number `json:"id"`
	Number     string      `json:"number"`
	Title      string      `json:"title"`
	AgencyCode string      `json:"agencyCode"`
	Agency     string      `json:"agency"`
	OpenDate   string      `json:"openDate"`
	CloseDate  string      `json:"closeDate"`
	Status     string      `json:"oppStatus"`

	// Populated from fetchOpportunity, only for --details/--min-award/--eligibility.
	Details *OppDetails `json:"details,omitempty"`
}

// Stale reports whether this opportunity was posted long enough ago that its
// "posted" status is questionable. Unparseable or missing dates are not
// treated as stale: absence of evidence is not evidence.
func (o Opportunity) Stale() bool {
	t, err := time.Parse(grantsDateLayout, o.OpenDate)
	if err != nil {
		return false
	}
	return time.Since(t) > grantsStaleAfter
}

// HasDeadline reports whether the record carries a real closing date.
//
// Two shapes mean "none stated": an empty string (19 of 66 measured results)
// and the placeholder 01/01/2099 (2 of 66). Neither confirms that applications
// are still accepted — the 2011 record has an empty close date — so callers
// should describe these as unstated rather than as rolling.
func (o Opportunity) HasDeadline() bool {
	if o.CloseDate == "" {
		return false
	}
	t, err := time.Parse(grantsDateLayout, o.CloseDate)
	if err != nil {
		return false
	}
	return t.Year() < 2099
}

type OppDetails struct {
	AwardCeiling     int64    `json:"awardCeiling"`
	AwardFloor       int64    `json:"awardFloor"`
	EstimatedFunding int64    `json:"estimatedFunding,omitempty"`
	ResponseDate     string   `json:"responseDate,omitempty"`
	ApplicantTypes   []string `json:"applicantTypes,omitempty"`
}

// AwardCap returns the best available amount for filtering and display: the
// award ceiling when present, otherwise the estimated total funding.
func (d OppDetails) AwardCap() int64 {
	if d.AwardCeiling > 0 {
		return d.AwardCeiling
	}
	return d.EstimatedFunding
}

type grantsSearchResp struct {
	ErrorCode int    `json:"errorcode"`
	Msg       string `json:"msg"`
	Data      struct {
		HitCount int           `json:"hitCount"`
		OppHits  []Opportunity `json:"oppHits"`
	} `json:"data"`
}

// SearchOpportunities lists posted (open) opportunities matching keyword,
// newest posting first.
func SearchOpportunities(keyword, agencyCode string, rows int) ([]Opportunity, int, error) {
	payload := map[string]any{
		"keyword":        keyword,
		"oppStatuses":    "posted",
		"rows":           rows,
		"startRecordNum": 0,
		"sortBy":         grantsSortNewestFirst,
	}
	if agencyCode != "" {
		payload["agencies"] = agencyCode
	}
	var resp grantsSearchResp
	if err := postJSON(grantsSearchURL, payload, &resp); err != nil {
		return nil, 0, fmt.Errorf("grants.gov search: %w", err)
	}
	if resp.ErrorCode != 0 {
		return nil, 0, fmt.Errorf("grants.gov search: errorcode %d: %s", resp.ErrorCode, resp.Msg)
	}
	opps := resp.Data.OppHits
	for i := range opps {
		// Unescape first, normalize second. The order matters: unescaping can
		// itself produce whitespace (&nbsp; becomes U+00A0, &#32; a plain
		// space), so normalizing first would leave that behind.
		opps[i].Title = normalizeText(html.UnescapeString(opps[i].Title)) // titles contain entities such as &ndash;
		opps[i].Agency = normalizeText(html.UnescapeString(opps[i].Agency))
	}
	return opps, resp.Data.HitCount, nil
}

// CountStale returns how many of these opportunities were posted long enough
// ago to be worth a caveat.
func CountStale(opps []Opportunity) int {
	n := 0
	for _, o := range opps {
		if o.Stale() {
			n++
		}
	}
	return n
}

type grantsFetchResp struct {
	ErrorCode int    `json:"errorcode"`
	Msg       string `json:"msg"`
	Data      struct {
		Synopsis struct {
			AwardCeiling     any    `json:"awardCeiling"`
			AwardFloor       any    `json:"awardFloor"`
			EstimatedFunding any    `json:"estimatedFunding"`
			ResponseDate     string `json:"responseDate"`
			ApplicantTypes   []struct {
				Description string `json:"description"`
			} `json:"applicantTypes"`
		} `json:"synopsis"`
	} `json:"data"`
}

// FetchDetails loads award ceiling/floor + eligible applicant types for one opportunity.
func FetchDetails(id json.Number) (*OppDetails, error) {
	var resp grantsFetchResp
	if err := postJSON(grantsFetchURL, map[string]any{"opportunityId": id.String()}, &resp); err != nil {
		return nil, fmt.Errorf("grants.gov fetchOpportunity %s: %w", id, err)
	}
	if resp.ErrorCode != 0 {
		return nil, fmt.Errorf("grants.gov fetchOpportunity %s: errorcode %d: %s", id, resp.ErrorCode, resp.Msg)
	}
	d := &OppDetails{
		AwardCeiling:     ParseMoney(resp.Data.Synopsis.AwardCeiling),
		AwardFloor:       ParseMoney(resp.Data.Synopsis.AwardFloor),
		EstimatedFunding: ParseMoney(resp.Data.Synopsis.EstimatedFunding),
		ResponseDate:     resp.Data.Synopsis.ResponseDate,
	}
	for _, t := range resp.Data.Synopsis.ApplicantTypes {
		if t.Description != "" {
			d.ApplicantTypes = append(d.ApplicantTypes, t.Description)
		}
	}
	return d, nil
}
