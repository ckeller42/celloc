package qeng_test

import (
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
		if _, err := qeng.ParseServingCell(in); err == nil {
			// some may decode partially; just must not panic
			_ = err
		}
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
	// v1 still won't select it (LTE-only).
	if _, ok := qeng.SelectGeolocatable(cells); ok {
		t.Fatal("NR5G-SA must not be selected in v1")
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
