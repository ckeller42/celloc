package google_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ckeller42/celloc/internal/google"
	"github.com/ckeller42/celloc/internal/wifiscan"
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

func TestResolveBuildsRequest(t *testing.T) {
	rt := &roundTrip{code: 200, resp: `{"location":{"lat":48.77,"lng":9.17},"accuracy":25}`}
	c := &google.Client{Key: "g.key", HTTP: rt}
	loc, err := c.Resolve(context.Background(), []wifiscan.AP{
		{BSSID: "00:11:22:33:44:55", Signal: -50},
		{BSSID: "aa:bb:cc:dd:ee:ff", Signal: -60},
	})
	if err != nil || loc.Accuracy != 25 {
		t.Fatalf("loc=%+v err=%v", loc, err)
	}
	if !strings.HasPrefix(rt.gotURL, "https://www.googleapis.com/geolocation/v1/geolocate?key=") {
		t.Fatalf("url=%q", rt.gotURL)
	}
	var sent struct {
		ConsiderIP bool `json:"considerIp"`
		Wifi       []struct {
			MAC    string `json:"macAddress"`
			Signal int    `json:"signalStrength"`
		} `json:"wifiAccessPoints"`
	}
	if err := json.Unmarshal(rt.gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.ConsiderIP || len(sent.Wifi) != 2 || sent.Wifi[0].MAC != "00:11:22:33:44:55" || sent.Wifi[0].Signal != -50 {
		t.Fatalf("body=%s", rt.gotBody)
	}
}

func TestResolveClassifiesError(t *testing.T) {
	rt := &roundTrip{code: 404, resp: `{"error":{"code":404,"errors":[{"reason":"notFound"}]}}`}
	c := &google.Client{Key: "g.key", HTTP: rt}
	if _, err := c.Resolve(context.Background(), []wifiscan.AP{{BSSID: "x"}}); err == nil {
		t.Fatal("want error on 404")
	}
}
