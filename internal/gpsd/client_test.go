package gpsd_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/influx"
	"github.com/ckeller42/celloc/internal/source"
)

func TestClientReadsTPVFromServer(t *testing.T) {
	fix := source.Fix{
		Time: time.Now().UTC(), Mode: 2, Lat: 48.7698, Lon: 9.1676,
		EPH: 1548, EPX: 1548, EPY: 1548, Source: "cell", Radio: "LTE",
		MCC: 262, MNC: 3, CID: 0x1684B3E, TAC: 0xE8E5,
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &gpsd.Server{Device: "cell0", Provider: func() source.Fix { return fix }, Interval: 20 * time.Millisecond}
	go func() { _ = srv.Serve(ctx, ln) }()

	c, err := gpsd.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Watch(); err != nil {
		t.Fatal(err)
	}
	tpv, err := c.ReadTPV()
	if err != nil {
		t.Fatal(err)
	}
	if tpv.Mode != 2 || tpv.Lat == nil || *tpv.Lat != 48.7698 {
		t.Fatalf("bad TPV: %+v", tpv)
	}
	if tpv.CellFix == nil || tpv.CellFix.Radio != "LTE" || tpv.CellFix.CID != 0x1684B3E {
		t.Fatalf("cellfix not round-tripped: %+v", tpv.CellFix)
	}

	// And the inverse conversion reconstructs a usable Fix.
	got := gpsd.FixFromTPV(tpv)
	if got.Mode != 2 || got.Lat != 48.7698 || got.Radio != "LTE" || got.CID != 0x1684B3E || got.Source != "cell" {
		t.Fatalf("FixFromTPV wrong: %+v", got)
	}
}

func TestWifiRoundTripToInfluxLine(t *testing.T) {
	fix := source.Fix{
		Time: time.Unix(1700000000, 0).UTC(), Mode: 2,
		Lat: 48.7701, Lon: 9.169, EPH: 35, EPX: 35, EPY: 35,
		Source: "wifi", APCount: 7,
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	srv := &gpsd.Server{Device: "wifi0", Provider: func() source.Fix { return fix }, Interval: 20 * time.Millisecond}
	go func() { _ = srv.Serve(ctx, ln) }()

	c, err := gpsd.Dial(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Watch(); err != nil {
		t.Fatal(err)
	}
	tpv, err := c.ReadTPV()
	if err != nil {
		t.Fatal(err)
	}

	back := gpsd.FixFromTPV(tpv)
	const want = "geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i"
	if got := influx.FixLine(back); got != want {
		t.Fatalf("round-trip influx line = %q, want %q", got, want)
	}
}
