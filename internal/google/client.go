package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/wifiscan"
)

// DefaultBaseURL is the Google APIs root.
const DefaultBaseURL = "https://www.googleapis.com"

// Doer is the subset of *http.Client used here; injected for tests.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client resolves positions via the Google Geolocation API.
type Client struct {
	Key     string
	HTTP    Doer
	BaseURL string // overrides DefaultBaseURL (tests)
}

type wifiAP struct {
	MAC    string `json:"macAddress"`
	Signal int    `json:"signalStrength"`
}

type request struct {
	ConsiderIP       bool     `json:"considerIp"`
	WifiAccessPoints []wifiAP `json:"wifiAccessPoints"`
}

// Resolve implements the wifi.Resolver contract.
func (c *Client) Resolve(ctx context.Context, aps []wifiscan.AP) (geoloc.Location, error) {
	r := request{ConsiderIP: false}
	for _, ap := range aps {
		r.WifiAccessPoints = append(r.WifiAccessPoints, wifiAP{MAC: ap.BSSID, Signal: ap.Signal})
	}
	body, err := json.Marshal(r)
	if err != nil {
		return geoloc.Location{}, err
	}
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := base + "/geolocation/v1/geolocate?key=" + url.QueryEscape(c.Key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return geoloc.Location{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		// *url.Error.Error() includes the full URL (with the API key in the query);
		// unwrap to its Op/Err so the key never reaches logs.
		var ue *url.Error
		if errors.As(err, &ue) {
			return geoloc.Location{}, fmt.Errorf("google: %s: %w", ue.Op, ue.Err)
		}
		return geoloc.Location{}, fmt.Errorf("google: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return geoloc.Location{}, fmt.Errorf("google: read body: %w", err)
	}
	return ParseResponse(resp.StatusCode, respBody)
}
