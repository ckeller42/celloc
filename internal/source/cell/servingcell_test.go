package cell_test

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"

	"github.com/ckeller42/celloc/internal/source/cell"
)

type scRunner func(context.Context, string) (string, error)

func (f scRunner) Run(ctx context.Context, cmd string) (string, error) { return f(ctx, cmd) }

func TestServingCellReaderDecodesLTE(t *testing.T) {
	const lte = `+QENG: "LTE","FDD",262,03,1684B3E,204,3350,7,5,5,E8E5,-83`
	r := cell.NewServingCellReader(scRunner(func(context.Context, string) (string, error) { return lte, nil }))
	c, ok := r.ServingCell(context.Background())
	if !ok {
		t.Fatal("want a serving cell")
	}
	if c.Radio != "LTE" || c.MCC != 262 || c.MNC != 3 || c.CID != 0x1684B3E || c.TAC != 0xE8E5 {
		t.Fatalf("bad cell: %+v", c)
	}
	if c.Signal != -83 { // RSRP from the trailing field
		t.Fatalf("RSRP not captured: %+v", c)
	}
}

func TestServingCellReaderNoCellOnError(t *testing.T) {
	r := cell.NewServingCellReader(scRunner(func(context.Context, string) (string, error) {
		return "", errors.New("modem busy")
	}))
	if _, ok := r.ServingCell(context.Background()); ok {
		t.Fatal("want ok=false on runner error")
	}
}

func TestServingCellReaderNoCellWhenNoLTE(t *testing.T) {
	r := cell.NewServingCellReader(scRunner(func(context.Context, string) (string, error) {
		return `+QENG: "servingcell","SEARCH"`, nil
	}))
	if _, ok := r.ServingCell(context.Background()); ok {
		t.Fatal("want ok=false when no geolocatable cell")
	}
}

func TestServingCellReaderDecodesPrefixedLTE(t *testing.T) {
	const out = "+QENG: \"servingcell\",\"NOCONN\",\"LTE\",\"FDD\",262,03,1684B3E,204,3350,7,5,5,E8E5,-83,-14,-47,17,13,100,-\r\n\r\nOK\r\n"
	r := cell.NewServingCellReader(scRunner(func(context.Context, string) (string, error) { return out, nil }))
	c, ok := r.ServingCell(context.Background())
	if !ok || c.Radio != "LTE" || c.CID != 0x1684B3E || c.TAC != 0xE8E5 {
		t.Fatalf("prefixed serving cell not decoded: %+v ok=%v", c, ok)
	}
}

func TestServingCellReaderLogsFailures(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
		want string
	}{
		{"runner error", "", errors.New("modem busy"), "modem busy"},
		{"no qeng lines", "ERROR\r\n", nil, "no serving-cell lines"},
		{"no geolocatable cell", `+QENG: "NR5G-NSA",262,03,451,-78,26,-10,638304,78,9,1`, nil, "no geolocatable"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			prev := log.Writer()
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(prev) })

			r := cell.NewServingCellReader(scRunner(func(context.Context, string) (string, error) { return tc.out, tc.err }))
			for i := 0; i < 3; i++ {
				if _, ok := r.ServingCell(context.Background()); ok {
					t.Fatal("want ok=false")
				}
			}
			got := buf.String()
			if !strings.Contains(got, tc.want) {
				t.Fatalf("log missing %q:\n%s", tc.want, got)
			}
			if n := strings.Count(got, "\n"); n != 1 {
				t.Fatalf("want 1 rate-limited log line, got %d:\n%s", n, got)
			}
		})
	}
}
