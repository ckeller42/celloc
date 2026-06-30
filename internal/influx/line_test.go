package influx_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/influx"
	"github.com/ckeller42/celloc/internal/source"
)

func TestFixLineMatchesSeedSchema(t *testing.T) {
	f := source.Fix{
		Lat: 48.7698, Lon: 9.1676, EPH: 1548,
		Source: "cell", Radio: "LTE", MCC: 262, MNC: 3, CID: 0x1684B3E, TAC: 0xE8E5,
	}
	got := influx.FixLine(f)
	want := "geo,source=cell,radio=LTE lat=48.7698,lon=9.1676,range_m=1548i,mcc=262i,mnc=3i,cid=23612222i,tac=59621i"
	if got != want {
		t.Fatalf("\n got: %s\nwant: %s", got, want)
	}
}

func TestFixLineNegativeCoords(t *testing.T) {
	f := source.Fix{Lat: -33.8688, Lon: 151.2093, EPH: 500, Source: "cell", Radio: "LTE"}
	got := influx.FixLine(f)
	want := "geo,source=cell,radio=LTE lat=-33.8688,lon=151.2093,range_m=500i,mcc=0i,mnc=0i,cid=0i,tac=0i"
	if got != want {
		t.Fatalf("\n got: %s\nwant: %s", got, want)
	}
}

func TestFixLineWifi(t *testing.T) {
	f := source.Fix{
		Mode: 2, Lat: 48.7701, Lon: 9.1690, EPH: 35,
		Source: "wifi", APCount: 7,
	}
	got := influx.FixLine(f)
	want := "geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i"
	if got != want {
		t.Fatalf("FixLine(wifi)\n got=%q\nwant=%q", got, want)
	}
}
