package unwiredlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Doer is the subset of *http.Client used here; injected for tests.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client resolves positions via the Unwired Labs LocationAPI.
type Client struct {
	Token    string
	Endpoint string // region subdomain, e.g. "eu1"
	HTTP     Doer
	BaseURL  string // overrides https://<Endpoint>.unwiredlabs.com (tests)
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	ep := c.Endpoint
	if ep == "" {
		ep = "eu1"
	}
	return "https://" + ep + ".unwiredlabs.com"
}

// LookupWifi resolves a position from the given APs.
func (c *Client) LookupWifi(ctx context.Context, aps []WifiAP) (Location, Status, error) {
	return c.do(ctx, Request{Token: c.Token, Wifi: aps, Address: 0})
}

func (c *Client) do(ctx context.Context, r Request) (Location, Status, error) {
	r.Token = c.Token
	body, err := json.Marshal(r)
	if err != nil {
		return Location{}, StatusServer, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL()+"/v2/process.php", bytes.NewReader(body))
	if err != nil {
		return Location{}, StatusServer, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: read body: %w", err)
	}
	return ParseResponse(resp.StatusCode, respBody)
}
