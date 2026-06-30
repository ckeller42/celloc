package google_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/google"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr bool
		lat     float64
		acc     float64
	}{
		{"ok", 200, `{"location":{"lat":48.7701,"lng":9.169},"accuracy":28}`, false, 48.7701, 28},
		{"200 no location", 200, `{}`, true, 0, 0},
		{"not found", 404, `{"error":{"code":404,"errors":[{"reason":"notFound"}],"message":"Not Found"}}`, true, 0, 0},
		{"key invalid", 400, `{"error":{"code":400,"errors":[{"reason":"keyInvalid"}],"message":"bad key"}}`, true, 0, 0},
		{"key invalid 403", 403, `{"error":{"code":403,"errors":[{"reason":"keyInvalid"}],"message":"bad key"}}`, true, 0, 0},
		{"daily limit", 403, `{"error":{"code":403,"errors":[{"reason":"dailyLimitExceeded"}],"message":"quota"}}`, true, 0, 0},
		{"rate 429", 429, `{"error":{"code":429,"message":"slow down"}}`, true, 0, 0},
		{"server", 503, ``, true, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, err := google.ParseResponse(tc.status, []byte(tc.body))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr && (loc.Lat != tc.lat || loc.Accuracy != tc.acc) {
				t.Fatalf("loc=%+v", loc)
			}
		})
	}
}
