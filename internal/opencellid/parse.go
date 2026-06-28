// Package opencellid resolves a serving cell to a location via the OpenCelliD
// API. parse.go is pure (HTTP status + body -> Location/Status); client.go does
// the I/O behind an injectable Doer.
package opencellid

import (
	"encoding/json"
	"fmt"
)

// Location is a resolved cell position. Range is the provider's error radius in
// meters (used as the gpsd EPH for the resulting fix).
type Location struct {
	Lat   float64
	Lon   float64
	Range int
}

// Status classifies a lookup outcome so callers can decide retry vs give-up.
type Status int

const (
	StatusOK          Status = iota // usable Location
	StatusUnknownCell               // cell not in the DB (no/zero coordinates)
	StatusRateLimited               // quota exceeded — back off
	StatusAuth                      // bad/expired API key
	StatusServer                    // 5xx — transient
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusUnknownCell:
		return "unknown-cell"
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

type apiResponse struct {
	Lat   *float64 `json:"lat"`
	Lon   *float64 `json:"lon"`
	Range int      `json:"range"`
	Error string   `json:"error"`
}

// ParseResponse maps an HTTP status + body to a (Location, Status). It returns a
// non-nil error only for unexpected/malformed payloads (a 200 with no usable
// coordinates is StatusUnknownCell, not an error).
func ParseResponse(httpStatus int, body []byte) (Location, Status, error) {
	switch {
	case httpStatus == 401 || httpStatus == 403:
		return Location{}, StatusAuth, nil
	case httpStatus == 429:
		return Location{}, StatusRateLimited, nil
	case httpStatus >= 500:
		return Location{}, StatusServer, nil
	case httpStatus != 200:
		// 400/404 etc. — treat as unknown cell (mirrors the script's guard).
		return Location{}, StatusUnknownCell, nil
	}

	var r apiResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Location{}, StatusServer, fmt.Errorf("opencellid: bad json: %w", err)
	}
	// Unknown cell: explicit error, or missing/zero coordinates (the seed guard).
	if r.Error != "" || r.Lat == nil || r.Lon == nil || (*r.Lat == 0 && *r.Lon == 0) {
		return Location{}, StatusUnknownCell, nil
	}
	return Location{Lat: *r.Lat, Lon: *r.Lon, Range: r.Range}, StatusOK, nil
}
