package unwiredlabs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ckeller42/celloc/internal/unwiredlabs"
)

type roundTrip struct {
	gotURL  string
	gotBody []byte
	resp    string
	code    int
}

func (r *roundTrip) Do(req *http.Request) (*http.Response, error) {
	r.gotURL = req.URL.String()
	r.gotBody, _ = io.ReadAll(req.Body)
	return &http.Response{
		StatusCode: r.code,
		Body:       io.NopCloser(bytes.NewBufferString(r.resp)),
		Header:     make(http.Header),
	}, nil
}

func TestLookupWifiBuildsRequest(t *testing.T) {
	rt := &roundTrip{code: 200, resp: `{"status":"ok","lat":48.77,"lon":9.17,"accuracy":30}`}
	c := &unwiredlabs.Client{Token: "pk.test", Endpoint: "eu1", HTTP: rt}
	loc, st, err := c.LookupWifi(context.Background(), []unwiredlabs.WifiAP{
		{BSSID: "00:11:22:33:44:55", Signal: -50},
	})
	if err != nil || st != unwiredlabs.StatusOK || loc.Accuracy != 30 {
		t.Fatalf("loc=%+v st=%v err=%v", loc, st, err)
	}
	if rt.gotURL != "https://eu1.unwiredlabs.com/v2/process.php" {
		t.Fatalf("url=%q", rt.gotURL)
	}
	var sent unwiredlabs.Request
	if err := json.Unmarshal(rt.gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Token != "pk.test" || len(sent.Wifi) != 1 || sent.Wifi[0].BSSID != "00:11:22:33:44:55" {
		t.Fatalf("body=%s", rt.gotBody)
	}
}
