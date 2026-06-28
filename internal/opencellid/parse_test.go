package opencellid_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/opencellid"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   opencellid.Status
		loc    opencellid.Location
	}{
		{
			"ok", 200, `{"lat":48.7698,"lon":9.1676,"range":1548}`, opencellid.StatusOK,
			opencellid.Location{Lat: 48.7698, Lon: 9.1676, Range: 1548},
		},
		{"unknown zero", 200, `{"lat":0,"lon":0,"range":0}`, opencellid.StatusUnknownCell, opencellid.Location{}},
		{"unknown empty", 200, `{}`, opencellid.StatusUnknownCell, opencellid.Location{}},
		{"unknown error field", 200, `{"error":"cell not found"}`, opencellid.StatusUnknownCell, opencellid.Location{}},
		{"rate limited", 429, ``, opencellid.StatusRateLimited, opencellid.Location{}},
		{"auth 401", 401, ``, opencellid.StatusAuth, opencellid.Location{}},
		{"auth 403", 403, ``, opencellid.StatusAuth, opencellid.Location{}},
		{"server 503", 503, ``, opencellid.StatusServer, opencellid.Location{}},
		{"not found 404", 404, ``, opencellid.StatusUnknownCell, opencellid.Location{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, st, err := opencellid.ParseResponse(tc.status, []byte(tc.body))
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if st != tc.want {
				t.Fatalf("status = %v, want %v", st, tc.want)
			}
			if loc != tc.loc {
				t.Fatalf("loc = %+v, want %+v", loc, tc.loc)
			}
		})
	}
}

func TestParseResponseBadJSON(t *testing.T) {
	if _, _, err := opencellid.ParseResponse(200, []byte("not json")); err == nil {
		t.Fatal("want error for malformed json")
	}
}
