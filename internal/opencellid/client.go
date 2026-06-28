package opencellid

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// DefaultBaseURL is the public OpenCelliD API root.
const DefaultBaseURL = "https://opencellid.org"

// Doer is the subset of *http.Client used here; injected for tests.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Query identifies a cell to look up.
type Query struct {
	MCC    int
	MNC    int
	LAC    int   // = TAC for LTE/NR
	CellID int64 // = CID/NCI
	Radio  string
}

// Client resolves cells via the OpenCelliD HTTP API.
type Client struct {
	Key     string
	HTTP    Doer
	BaseURL string // defaults to DefaultBaseURL
}

// Lookup resolves q to a Location. Network/HTTP errors are returned as errors;
// API-level outcomes (unknown cell, rate limit, auth) come back via Status.
func (c *Client) Lookup(ctx context.Context, q Query) (Location, Status, error) {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	v := url.Values{}
	v.Set("key", c.Key)
	v.Set("mcc", strconv.Itoa(q.MCC))
	v.Set("mnc", strconv.Itoa(q.MNC))
	v.Set("lac", strconv.Itoa(q.LAC))
	v.Set("cellid", strconv.FormatInt(q.CellID, 10))
	if q.Radio != "" {
		v.Set("radio", q.Radio)
	}
	v.Set("format", "json")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/cell/get?"+v.Encode(), nil)
	if err != nil {
		return Location{}, StatusServer, err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("opencellid: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("opencellid: read body: %w", err)
	}
	return ParseResponse(resp.StatusCode, body)
}
