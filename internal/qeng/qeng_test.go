package qeng_test

import (
	"errors"
	"testing"

	"github.com/ckeller42/celloc/internal/qeng"
)

// Real samples captured live from the GL-E5800.
const (
	lteLine    = `+QENG: "LTE","FDD",262,03,1684B3E,204,3350,7,5,5,E8E5,-83,-14,-47,17,13,100,-`
	nsaLine    = `+QENG: "NR5G-NSA",262,03,451,-78,26,-10,638304,78,9,1`
	withEcho   = "AT+QENG=\"servingcell\"\r\n" + lteLine + "\r\n\r\nOK\r\n"
	nsaThenLte = nsaLine + "\r\n" + lteLine + "\r\n"
)

func TestParseLTE(t *testing.T) {
	cells, err := qeng.ParseServingCell(lteLine)
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) != 1 {
		t.Fatalf("want 1 cell, got %d", len(cells))
	}
	c := cells[0]
	if c.Radio != qeng.RadioLTE || c.MCC != 262 || c.MNC != 3 {
		t.Fatalf("bad MCC/MNC/radio: %+v", c)
	}
	if c.CID != 0x1684B3E || c.TAC != 0xE8E5 || !c.HasID {
		t.Fatalf("bad CID/TAC: CID=%#x TAC=%#x hasID=%v", c.CID, c.TAC, c.HasID)
	}
}

func TestLeadingZeroMNCIsDecimal(t *testing.T) {
	// "03" must be 3, not octal — a real foot-gun the shell version had to dodge.
	cells, _ := qeng.ParseServingCell(lteLine)
	if cells[0].MNC != 3 {
		t.Fatalf("MNC=%d, want 3", cells[0].MNC)
	}
}

func TestParseStripsEchoAndOK(t *testing.T) {
	cells, err := qeng.ParseServingCell(withEcho)
	if err != nil || len(cells) != 1 || cells[0].MCC != 262 {
		t.Fatalf("echo/OK not handled: %+v err=%v", cells, err)
	}
}

func TestNR5GNSAHasNoIDs(t *testing.T) {
	cells, err := qeng.ParseServingCell(nsaLine)
	if err != nil {
		t.Fatal(err)
	}
	c := cells[0]
	if c.Radio != qeng.RadioNR5GNSA || c.MCC != 262 || c.MNC != 3 {
		t.Fatalf("bad NSA parse: %+v", c)
	}
	if c.HasID {
		t.Fatalf("NSA line must not be geolocatable: %+v", c)
	}
}

func TestSelectGeolocatablePrefersLTEAnchor(t *testing.T) {
	cells, err := qeng.ParseServingCell(nsaThenLte)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := qeng.SelectGeolocatable(cells)
	if !ok || got.Radio != qeng.RadioLTE || got.CID != 0x1684B3E {
		t.Fatalf("want LTE anchor, got %+v ok=%v", got, ok)
	}
}

func TestSelectGeolocatableNoneWhenNSAOnly(t *testing.T) {
	cells, _ := qeng.ParseServingCell(nsaLine)
	if _, ok := qeng.SelectGeolocatable(cells); ok {
		t.Fatal("NSA-only must not be geolocatable in v1")
	}
}

func TestEmptyAndDetached(t *testing.T) {
	for _, in := range []string{"", "\r\n", "OK\r\n", "+QENG: "} {
		if _, err := qeng.ParseServingCell(in); err != qeng.ErrNoCells {
			t.Fatalf("input %q: want ErrNoCells, got %v", in, err)
		}
	}
}

func TestGarbageDoesNotPanic(t *testing.T) {
	for _, in := range []string{
		`+QENG: "LTE","FDD",262`,       // too few fields
		`+QENG: "LTE","FDD",x,y,z,...`, // non-numeric mcc
		`random noise`,
	} {
		t.Run(in, func(*testing.T) {
			// Must not panic; partial decode or ErrNoCells are both acceptable.
			_, _ = qeng.ParseServingCell(in)
		})
	}
}

func TestNR5GSADecodesIDs(t *testing.T) {
	line := `+QENG: "NR5G-SA","TDD",262,03,12345ABC,500,E8E5,627264,78`
	cells, err := qeng.ParseServingCell(line)
	if err != nil {
		t.Fatal(err)
	}
	c := cells[0]
	if c.Radio != qeng.RadioNR5GSA || c.MCC != 262 || c.MNC != 3 {
		t.Fatalf("bad NR5G-SA parse: %+v", c)
	}
	if c.CID != 0x12345ABC || c.TAC != 0xE8E5 || !c.HasID {
		t.Fatalf("NR5G-SA IDs not decoded: %+v", c)
	}
	// SA has no LTE anchor, so its own IDs are the only cell to geolocate.
	got, ok := qeng.SelectGeolocatable(cells)
	if !ok || got.Radio != qeng.RadioNR5GSA || got.CID != 0x12345ABC {
		t.Fatalf("NR5G-SA with IDs must be selectable, got %+v ok=%v", got, ok)
	}
}

func TestSelectGeolocatablePrefersLTEOverNR5GSA(t *testing.T) {
	in := `+QENG: "NR5G-SA","TDD",262,03,12345ABC,500,E8E5,627264,78` + "\r\n" + lteLine + "\r\n"
	cells, err := qeng.ParseServingCell(in)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := qeng.SelectGeolocatable(cells)
	if !ok || got.Radio != qeng.RadioLTE {
		t.Fatalf("want LTE preferred, got %+v ok=%v", got, ok)
	}
}

// The standard Quectel form prefixes "servingcell",<state> on the same line.
func TestParsePrefixedServingCell(t *testing.T) {
	const prefixedLTE = `+QENG: "servingcell","NOCONN","LTE","FDD",262,03,1684B3E,204,3350,7,5,5,E8E5,-83,-14,-47,17,13,100,-`
	tests := []struct {
		name        string
		in          string
		wantOK      bool
		wantLTE     bool
		wantNoCells bool // parser must report ErrNoCells
	}{
		{name: "prefixed LTE", in: prefixedLTE, wantOK: true, wantLTE: true},
		{name: "prefixed LTE CRLF", in: prefixedLTE + "\r\n\r\nOK\r\n", wantOK: true, wantLTE: true},
		{name: "prefixed LTE with echo", in: "AT+QENG=\"servingcell\"\r\n" + prefixedLTE + "\r\nOK\r\n", wantOK: true, wantLTE: true},
		{name: "search state only", in: `+QENG: "servingcell","SEARCH"`, wantNoCells: true},
		{name: "limited service only", in: `+QENG: "servingcell","LIMSRV"` + "\r\n", wantNoCells: true},
		{name: "prefix only", in: `+QENG: "servingcell"`, wantNoCells: true},
		{name: "truncated prefixed LTE", in: `+QENG: "servingcell","NOCONN","LTE","FDD",262`, wantNoCells: true},
		{name: "prefixed NR5G-SA", in: `+QENG: "servingcell","NOCONN","NR5G-SA","TDD",262,03,12345ABC,500,E8E5,627264,78`, wantOK: true},
		{name: "state line then unprefixed LTE", in: `+QENG: "servingcell","CONNECT"` + "\r\n" + lteLine + "\r\n", wantOK: true, wantLTE: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cells, err := qeng.ParseServingCell(tc.in)
			if tc.wantNoCells != errors.Is(err, qeng.ErrNoCells) {
				t.Fatalf("err=%v, wantNoCellsls=%v", err, tc.wantNoCells)
			}
			got, ok := qeng.SelectGeolocatable(cells)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v (cells=%+v err=%v)", ok, tc.wantOK, cells, err)
			}
			if !ok {
				return
			}
			if tc.wantLTE {
				if got.Radio != qeng.RadioLTE || got.MCC != 262 || got.MNC != 3 ||
					got.CID != 0x1684B3E || got.TAC != 0xE8E5 || got.Signal != -83 {
					t.Fatalf("bad prefixed LTE decode: %+v", got)
				}
				return
			}
			if got.Radio != qeng.RadioNR5GSA || got.CID != 0x12345ABC || got.TAC != 0xE8E5 {
				t.Fatalf("bad prefixed NR5G-SA decode: %+v", got)
			}
		})
	}
}

func TestUnknownRadioSkipped(t *testing.T) {
	if _, err := qeng.ParseServingCell(`+QENG: "WCDMA","x",262,03`); err != qeng.ErrNoCells {
		t.Fatalf("unknown radio should yield no cells, got %v", err)
	}
}

func TestBadHexCIDStillParsesCellWithoutID(t *testing.T) {
	line := `+QENG: "LTE","FDD",262,03,ZZZZ,204,3350,7,5,5,E8E5,-83`
	cells, err := qeng.ParseServingCell(line)
	if err != nil {
		t.Fatal(err)
	}
	if cells[0].HasID {
		t.Fatal("bad hex CID must leave HasID=false")
	}
	if cells[0].MCC != 262 {
		t.Fatal("MCC should still decode")
	}
}
