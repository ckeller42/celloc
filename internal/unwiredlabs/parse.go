// Package unwiredlabs resolves a position from nearby WiFi APs (and optionally
// cells) via the Unwired Labs LocationAPI (process.php). parse.go is pure
// (HTTP status + body -> Location/Status); client.go does the I/O behind a Doer.
package unwiredlabs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Location is a resolved position. Accuracy is the provider's error radius in
// meters (used as the gpsd EPH for the resulting fix).
type Location struct {
	Lat      float64
	Lon      float64
	Accuracy float64
}

// Status classifies a lookup outcome so callers can decide retry vs give-up.
type Status int

// Lookup outcome classifications.
const (
	StatusOK          Status = iota // usable Location
	StatusNotFound                  // no match for the given APs/cells
	StatusRateLimited               // quota/credits exhausted — back off
	StatusAuth                      // bad/expired token
	StatusServer                    // 5xx / malformed — transient
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusNotFound:
		return "not-found"
	case StatusRateLimited:
		return "rate-limited"
	case StatusAuth:
		return "auth"
	case StatusServer:
		return "server"
	default:
		return "invalid"
	}
}

// WifiAP is one access point in a LocationAPI request.
type WifiAP struct {
	BSSID  string `json:"bssid"`
	Signal int    `json:"signal,omitempty"`
}

// CellTower is one cell in a LocationAPI request (optional fallback anchor).
type CellTower struct {
	LAC    int   `json:"lac"`
	CID    int64 `json:"cid"`
	MCC    int   `json:"mcc"`
	MNC    int   `json:"mnc"`
	Signal int   `json:"signal,omitempty"`
}

// Request is the process.php JSON body.
type Request struct {
	Token   string      `json:"token"`
	Radio   string      `json:"radio,omitempty"`
	MCC     int         `json:"mcc,omitempty"`
	MNC     int         `json:"mnc,omitempty"`
	Cells   []CellTower `json:"cells,omitempty"`
	Wifi    []WifiAP    `json:"wifi,omitempty"`
	Address int         `json:"address"`
}

type apiResponse struct {
	Status   string   `json:"status"`
	Message  string   `json:"message"`
	Lat      *float64 `json:"lat"`
	Lon      *float64 `json:"lon"`
	Accuracy float64  `json:"accuracy"`
}

// ParseResponse maps an HTTP status + body to a (Location, Status). It returns a
// non-nil error only for malformed payloads.
func ParseResponse(httpStatus int, body []byte) (Location, Status, error) {
	switch {
	case httpStatus == 401 || httpStatus == 403:
		return Location{}, StatusAuth, nil
	case httpStatus == 429:
		return Location{}, StatusRateLimited, nil
	case httpStatus >= 500:
		return Location{}, StatusServer, nil
	case httpStatus != 200:
		return Location{}, StatusNotFound, nil
	}

	var r apiResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: bad json: %w", err)
	}
	if r.Status == "ok" && r.Lat != nil && r.Lon != nil {
		return Location{Lat: *r.Lat, Lon: *r.Lon, Accuracy: r.Accuracy}, StatusOK, nil
	}
	if r.Status == "error" {
		return Location{}, classify(r.Message), nil
	}
	return Location{}, StatusServer, nil
}

// classify maps an error message to a Status (best-effort; the API returns 200
// with a status:"error" body for most failures).
func classify(msg string) Status {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "token"), strings.Contains(m, "key"),
		strings.Contains(m, "access"), strings.Contains(m, "invalid request"):
		return StatusAuth
	case strings.Contains(m, "credit"), strings.Contains(m, "balance"),
		strings.Contains(m, "limit"), strings.Contains(m, "quota"):
		return StatusRateLimited
	default:
		return StatusNotFound
	}
}
