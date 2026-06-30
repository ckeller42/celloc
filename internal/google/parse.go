// Package google resolves a position from nearby WiFi APs via the Google
// Geolocation API. parse.go is pure (HTTP status + body -> geoloc.Location or a
// classified error); client.go does the I/O behind an injected Doer.
package google

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/ckeller42/celloc/internal/geoloc"
)

type apiResponse struct {
	Location *struct {
		Lat float64 `json:"lat"`
		Lng float64 `json:"lng"`
	} `json:"location"`
	Accuracy float64 `json:"accuracy"`
	Error    *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Reason string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// ParseResponse maps an HTTP status + body to a geoloc.Location, or a classified
// error ("google: not found" / "...rate limited" / "...auth" / "...server").
func ParseResponse(httpStatus int, body []byte) (geoloc.Location, error) {
	var r apiResponse
	_ = json.Unmarshal(body, &r) // best-effort; status drives classification

	if httpStatus == 200 {
		if r.Location != nil {
			return geoloc.Location{Lat: r.Location.Lat, Lon: r.Location.Lng, Accuracy: r.Accuracy}, nil
		}
		return geoloc.Location{}, errors.New("google: 200 without location")
	}

	reason := ""
	if r.Error != nil && len(r.Error.Errors) > 0 {
		reason = r.Error.Errors[0].Reason
	}
	switch {
	case httpStatus == 404:
		return geoloc.Location{}, errors.New("google: not found")
	case httpStatus == 429 || strings.Contains(reason, "imitExceeded"):
		return geoloc.Location{}, fmt.Errorf("google: rate limited (%s)", reason)
	case httpStatus == 400 || httpStatus == 403:
		return geoloc.Location{}, fmt.Errorf("google: auth/bad request (%d %s)", httpStatus, reason)
	default:
		return geoloc.Location{}, fmt.Errorf("google: server (%d)", httpStatus)
	}
}
