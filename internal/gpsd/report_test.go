package gpsd_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/source"
)

func TestTPVFromFix_CellFix(t *testing.T) {
	f := source.Fix{
		Time: time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
		Mode: 2, Lat: 48.7698, Lon: 9.1676, EPH: 1548, EPX: 1548, EPY: 1548,
		Source: "cell", Radio: "LTE", MCC: 262, MNC: 3, CID: 0x1684B3E, TAC: 0xE8E5,
	}
	tpv := gpsd.TPVFromFix(f, "cell0")
	if tpv.Mode != 2 || tpv.Lat == nil || *tpv.Lat != 48.7698 {
		t.Fatalf("bad tpv: %+v", tpv)
	}
	if tpv.EPH == nil || *tpv.EPH != 1548 {
		t.Fatal("eph must equal range")
	}
	if tpv.Time != "2026-06-28T12:00:00.000Z" {
		t.Fatalf("time=%q", tpv.Time)
	}
	if tpv.CellFix == nil || tpv.CellFix.CID != 0x1684B3E {
		t.Fatal("cellfix extension missing")
	}

	// Honesty: must NOT contain alt/speed/track keys.
	b, _ := json.Marshal(tpv)
	for _, k := range []string{`"alt"`, `"altHAE"`, `"speed"`, `"track"`, `"climb"`} {
		if bytes.Contains(b, []byte(k)) {
			t.Fatalf("TPV must not contain %s: %s", k, b)
		}
	}
}

func TestTPVFromFix_NoFixOmitsCoords(t *testing.T) {
	tpv := gpsd.TPVFromFix(source.Fix{Mode: 0}, "cell0")
	if tpv.Mode != 0 {
		t.Fatal("want mode 0")
	}
	b, _ := json.Marshal(tpv)
	for _, k := range []string{`"lat"`, `"lon"`, `"eph"`, `"time"`, `"cellfix"`} {
		if bytes.Contains(b, []byte(k)) {
			t.Fatalf("no-fix TPV must omit %s: %s", k, b)
		}
	}
}

func TestSKYEmptyHasNoFakeDOP(t *testing.T) {
	b, _ := json.Marshal(gpsd.SKYEmpty("cell0"))
	if !bytes.Contains(b, []byte(`"satellites":[]`)) {
		t.Fatalf("want empty satellites: %s", b)
	}
	for _, k := range []string{`"hdop"`, `"pdop"`, `"vdop"`} {
		if bytes.Contains(b, []byte(k)) {
			t.Fatalf("must not fabricate %s: %s", k, b)
		}
	}
}

func TestFixFromTPV_NoFix(t *testing.T) {
	got, _ := gpsd.FixFromTPV(gpsd.TPV{Class: "TPV", Mode: 0})
	if got.HasFix() || got.Source != "" {
		t.Fatalf("no-fix TPV should yield empty fix: %+v", got)
	}
}

func TestMarshalLineFraming(t *testing.T) {
	b, err := gpsd.MarshalLine(gpsd.Version{Class: "VERSION", Release: "celloc-test"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(b, []byte("\r\n")) {
		t.Fatalf("want CRLF framing: %q", b)
	}
}

func TestTPVFromWifiFix(t *testing.T) {
	f := source.Fix{
		Time: time.Unix(1700000000, 0).UTC(), Mode: 2,
		Lat: 48.7701, Lon: 9.169, EPH: 35, EPX: 35, EPY: 35,
		Source: "wifi", APCount: 7,
	}
	tpv := gpsd.TPVFromFix(f, "cell0")
	if tpv.WifiFix == nil || tpv.WifiFix.APCount != 7 {
		t.Fatalf("wifix missing/wrong: %+v", tpv.WifiFix)
	}
	if tpv.CellFix != nil {
		t.Fatalf("wifi fix must not carry cellfix")
	}
	b, _ := gpsd.MarshalLine(tpv)
	for _, k := range []string{`"alt"`, `"speed"`, `"track"`, `"cellfix"`} {
		if bytes.Contains(b, []byte(k)) {
			t.Fatalf("wifi TPV must not contain %s: %s", k, b)
		}
	}
	back, _ := gpsd.FixFromTPV(tpv)
	if back.Source != "wifi" || back.APCount != 7 {
		t.Fatalf("FixFromTPV lost wifi info: %+v", back)
	}
}

func TestFixFromTPV_ModeWithoutCoordsIsNoFix(t *testing.T) {
	lat := 48.7
	tests := []struct {
		name string
		tpv  gpsd.TPV
	}{
		{"mode 2, no lat/lon", gpsd.TPV{Class: "TPV", Mode: 2}},
		{"mode 3, lat only", gpsd.TPV{Class: "TPV", Mode: 3, Lat: &lat}},
		{"mode 2, lon only", gpsd.TPV{Class: "TPV", Mode: 2, Lon: &lat}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := gpsd.FixFromTPV(tc.tpv); got.HasFix() {
				t.Fatalf("TPV without coordinates must not be a fix (bogus 0,0): %+v", got)
			}
		})
	}
}

func TestFixFromTPV_KeepsTime(t *testing.T) {
	lat, lon := 48.7, 9.1
	got, err := gpsd.FixFromTPV(gpsd.TPV{Class: "TPV", Mode: 2, Lat: &lat, Lon: &lon, Time: "2026-06-28T12:00:00.000Z"})
	if err != nil || !got.HasFix() || !got.Time.Equal(time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("want fix with parsed time: %+v", got)
	}
}

func TestFixFromTPV_BadTimeIsReported(t *testing.T) {
	lat, lon := 48.7, 9.1
	got, err := gpsd.FixFromTPV(gpsd.TPV{Class: "TPV", Mode: 2, Lat: &lat, Lon: &lon, Time: "yesterday-ish"})
	if err == nil || !strings.Contains(err.Error(), "yesterday-ish") {
		t.Fatalf("want error naming the bad time, got %v", err)
	}
	if !got.HasFix() || !got.Time.IsZero() {
		t.Fatalf("position must survive with a zero time: %+v", got)
	}
}
