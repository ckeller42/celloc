package unwiredlabs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
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

func TestResolveMapsAPsAndClassifies(t *testing.T) {
	ok := &roundTrip{code: 200, resp: `{"status":"ok","lat":48.77,"lon":9.17,"accuracy":30}`}
	c := &unwiredlabs.Client{Token: "pk.test", Endpoint: "eu1", HTTP: ok}
	loc, err := c.Resolve(context.Background(), []wifiscan.AP{{BSSID: "aa:bb", Signal: -50}}, nil)
	if err != nil || loc.Accuracy != 30 {
		t.Fatalf("ok: loc=%+v err=%v", loc, err)
	}
	bad := &roundTrip{code: 200, resp: `{"status":"error","message":"Invalid token"}`}
	c2 := &unwiredlabs.Client{Token: "pk.bad", Endpoint: "eu1", HTTP: bad}
	if _, err := c2.Resolve(context.Background(), []wifiscan.AP{{BSSID: "aa:bb"}}, nil); err == nil {
		t.Fatal("want error on auth status")
	}
}

func TestResolveBlendsCell(t *testing.T) {
	rt := &roundTrip{code: 200, resp: `{"status":"ok","lat":48.77,"lon":9.17,"accuracy":40}`}
	c := &unwiredlabs.Client{Token: "pk.test", Endpoint: "eu1", HTTP: rt}
	cell := &geoloc.CellTower{Radio: "LTE", MCC: 262, MNC: 3, CID: 23612222, TAC: 59621, Signal: -84}
	if _, err := c.Resolve(context.Background(), []wifiscan.AP{{BSSID: "aa:bb", Signal: -50}}, cell); err != nil {
		t.Fatal(err)
	}
	var sent unwiredlabs.Request
	if err := json.Unmarshal(rt.gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if len(sent.Cells) != 1 || sent.Cells[0].CID != 23612222 || sent.Cells[0].LAC != 59621 || sent.MCC != 262 {
		t.Fatalf("cell not blended: %s", rt.gotBody)
	}
	if len(sent.Wifi) != 1 {
		t.Fatalf("wifi should still be present: %s", rt.gotBody)
	}
	if sent.Radio != "lte" {
		t.Fatalf("radio=%q want lte", sent.Radio)
	}
}

func TestResolveRadioMapsNR(t *testing.T) {
	rt := &roundTrip{code: 200, resp: `{"status":"ok","lat":48.77,"lon":9.17,"accuracy":50}`}
	c := &unwiredlabs.Client{Token: "pk.test", Endpoint: "eu1", HTTP: rt}
	cell := &geoloc.CellTower{Radio: "NR5G-SA", MCC: 262, MNC: 3, CID: 1, TAC: 2}
	if _, err := c.Resolve(context.Background(), nil, cell); err != nil {
		t.Fatal(err)
	}
	var sent unwiredlabs.Request
	if err := json.Unmarshal(rt.gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Radio != "nr" {
		t.Fatalf("radio=%q want nr", sent.Radio)
	}
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
