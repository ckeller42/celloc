package cell_test

import (
	"context"
	"errors"
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
