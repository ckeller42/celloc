package unwiredlabs_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/unwiredlabs"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   unwiredlabs.Status
		lat    float64
	}{
		{"ok", 200, `{"status":"ok","balance":98,"lat":48.7701,"lon":9.169,"accuracy":35}`, unwiredlabs.StatusOK, 48.7701},
		{"no match", 200, `{"status":"error","message":"No matches found"}`, unwiredlabs.StatusNotFound, 0},
		{"bad token body", 200, `{"status":"error","message":"Invalid token"}`, unwiredlabs.StatusAuth, 0},
		{"quota body", 200, `{"status":"error","message":"Insufficient credits / balance"}`, unwiredlabs.StatusRateLimited, 0},
		{"http 401", 401, ``, unwiredlabs.StatusAuth, 0},
		{"http 429", 429, ``, unwiredlabs.StatusRateLimited, 0},
		{"http 503", 503, ``, unwiredlabs.StatusServer, 0},
		{"bad json", 200, `{`, unwiredlabs.StatusServer, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, st, _ := unwiredlabs.ParseResponse(tc.status, []byte(tc.body))
			if st != tc.want {
				t.Fatalf("status=%v want %v", st, tc.want)
			}
			if tc.want == unwiredlabs.StatusOK && loc.Lat != tc.lat {
				t.Fatalf("lat=%v want %v", loc.Lat, tc.lat)
			}
		})
	}
}
