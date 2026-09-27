package influx_test

import (
	"errors"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/influx"
	"github.com/ckeller42/celloc/internal/source"
)

func TestFixLine(t *testing.T) {
	ts := time.Date(2026, 9, 17, 10, 0, 0, 123456789, time.FixedZone("CEST", 2*3600))
	tests := []struct {
		name string
		f    source.Fix
		want string
	}{
		{
			"cell matches seed schema",
			source.Fix{Lat: 48.7698, Lon: 9.1676, EPH: 1548, Source: "cell", Radio: "LTE", MCC: 262, MNC: 3, CID: 0x1684B3E, TAC: 0xE8E5},
			"geo,source=cell,radio=LTE lat=48.7698,lon=9.1676,range_m=1548i,mcc=262i,mnc=3i,cid=23612222i,tac=59621i",
		},
		{
			"cell negative coords",
			source.Fix{Lat: -33.8688, Lon: 151.2093, EPH: 500, Source: "cell", Radio: "LTE"},
			"geo,source=cell,radio=LTE lat=-33.8688,lon=151.2093,range_m=500i,mcc=0i,mnc=0i,cid=0i,tac=0i",
		},
		{
			"cell without radio omits the empty tag",
			source.Fix{Mode: 2, Lat: 48.7698, Lon: 9.1676, EPH: 1548, Source: "cell", MCC: 262, MNC: 3, CID: 1, TAC: 2},
			"geo,source=cell lat=48.7698,lon=9.1676,range_m=1548i,mcc=262i,mnc=3i,cid=1i,tac=2i",
		},
		{
			"radio tag special characters are escaped",
			source.Fix{Mode: 2, Lat: 1, Lon: 2, EPH: 3, Source: "cell", Radio: "NR 5G,SA=x"},
			`geo,source=cell,radio=NR\ 5G\,SA\=x lat=1,lon=2,range_m=3i,mcc=0i,mnc=0i,cid=0i,tac=0i`,
		},
		{
			"wifi",
			source.Fix{Mode: 2, Lat: 48.7701, Lon: 9.1690, EPH: 35, Source: "wifi", APCount: 7},
			"geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i",
		},
		{
			"wifi ignores a stray radio",
			source.Fix{Mode: 2, Lat: 48.7701, Lon: 9.169, EPH: 35, Source: "wifi", Radio: "LTE", APCount: 7},
			"geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i",
		},
		{
			"cell with fix timestamp",
			source.Fix{Time: ts, Mode: 2, Lat: 48.7698, Lon: 9.1676, EPH: 1548, Source: "cell", Radio: "LTE", MCC: 262, MNC: 3, CID: 1, TAC: 2},
			"geo,source=cell,radio=LTE lat=48.7698,lon=9.1676,range_m=1548i,mcc=262i,mnc=3i,cid=1i,tac=2i 1789632000123456789",
		},
		{
			"wifi with fix timestamp",
			source.Fix{Time: ts, Mode: 2, Lat: 48.7701, Lon: 9.169, EPH: 35, Source: "wifi", APCount: 7},
			"geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i 1789632000123456789",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := influx.FixLine(tc.f)
			if err != nil {
				t.Fatalf("FixLine: %v", err)
			}
			if got != tc.want {
				t.Fatalf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestFixLineRefusesUnattributedFix(t *testing.T) {
	ts := time.Unix(1789632000, 0)
	tests := []struct {
		name string
		f    source.Fix
	}{
		{"empty source", source.Fix{Time: ts, Mode: 2, Lat: 48.7, Lon: 9.1, EPH: 35}},
		{"empty source with cell ids", source.Fix{Mode: 2, Lat: 48.7, Lon: 9.1, EPH: 35, Radio: "LTE", MCC: 262, CID: 1}},
		{"empty source with ap count", source.Fix{Mode: 2, Lat: 48.7, Lon: 9.1, EPH: 35, APCount: 3}},
		{"unknown source", source.Fix{Mode: 2, Lat: 48.7, Lon: 9.1, EPH: 5, Source: "gnss"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := influx.FixLine(tc.f)
			if !errors.Is(err, influx.ErrUnattributedFix) {
				t.Fatalf("want ErrUnattributedFix, got err=%v line=%q", err, got)
			}
			if got != "" {
				t.Fatalf("no line may be rendered for an unattributed fix, got %q", got)
			}
		})
	}
}

func TestStatusLine(t *testing.T) {
	now := time.Unix(1789632000, 500_000_000).UTC()
	tests := []struct {
		name      string
		f         source.Fix
		connected bool
		want      string
	}{
		{
			"fresh fix",
			source.Fix{Mode: 2, Time: now.Add(-12500 * time.Millisecond)},
			true,
			"geo_status mode=2i,fix_age_s=12.5,connected=true 1789632000500000000",
		},
		{
			"connected, no fix (no time)",
			source.Fix{Mode: 0},
			true,
			"geo_status mode=0i,fix_age_s=-1,connected=true 1789632000500000000",
		},
		{
			"disconnected",
			source.Fix{},
			false,
			"geo_status mode=0i,fix_age_s=-1,connected=false 1789632000500000000",
		},
		{
			"stale cached fix, sub-ms rounded",
			source.Fix{Mode: 2, Time: now.Add(-(150*time.Second + 1234567))},
			true,
			"geo_status mode=2i,fix_age_s=150.001,connected=true 1789632000500000000",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := influx.StatusLine(tc.f, tc.connected, now); got != tc.want {
				t.Fatalf("\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}
