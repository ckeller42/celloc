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
	"strings"

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

type cellTower struct {
	CellID            int64 `json:"cellId"`
	LocationAreaCode  int   `json:"locationAreaCode"`
	MobileCountryCode int   `json:"mobileCountryCode"`
	MobileNetworkCode int   `json:"mobileNetworkCode"`
	SignalStrength    int   `json:"signalStrength,omitempty"`
}

type request struct {
	ConsiderIP       bool        `json:"considerIp"`
	RadioType        string      `json:"radioType,omitempty"`
	WifiAccessPoints []wifiAP    `json:"wifiAccessPoints,omitempty"`
	CellTowers       []cellTower `json:"cellTowers,omitempty"`
}

// radioType maps a modem access-technology string to a Google radioType.
func radioType(radio string) string {
	if strings.Contains(strings.ToUpper(radio), "NR") {
		return "nr"
	}
	return "lte"
}

// Resolve implements the wifi.Resolver contract; when cell is non-nil it is
// blended into the request as a cellTower so Google fuses WiFi + cell.
func (c *Client) Resolve(ctx context.Context, aps []wifiscan.AP, cell *geoloc.CellTower) (geoloc.Location, error) {
	r := request{ConsiderIP: false}
	for _, ap := range aps {
		r.WifiAccessPoints = append(r.WifiAccessPoints, wifiAP{MAC: ap.BSSID, Signal: ap.Signal})
	}
	if cell != nil {
		r.RadioType = radioType(cell.Radio)
		ct := cellTower{
			CellID:            cell.CID,
			LocationAreaCode:  cell.TAC,
			MobileCountryCode: cell.MCC,
			MobileNetworkCode: cell.MNC,
			SignalStrength:    cell.Signal,
		}
		r.CellTowers = append(r.CellTowers, ct)
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
