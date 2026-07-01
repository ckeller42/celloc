package cell

import (
	"context"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/qeng"
)

// ServingCellReader reads the modem's serving cell (AT+QENG) and returns it as a
// provider-neutral geoloc.CellTower, for blending into a WiFi geolocation request.
// It performs no OpenCelliD lookup — resolution is the provider's job.
type ServingCellReader struct {
	Runner atrun.Runner
}

// NewServingCellReader builds a reader over the given AT runner.
func NewServingCellReader(r atrun.Runner) *ServingCellReader {
	return &ServingCellReader{Runner: r}
}

// ServingCell implements wifi.CellReader. ok is false when the modem read fails
// or no geolocatable serving cell is present.
func (r *ServingCellReader) ServingCell(ctx context.Context) (*geoloc.CellTower, bool) {
	out, err := r.Runner.Run(ctx, servingCellCmd)
	if err != nil {
		return nil, false
	}
	cells, err := qeng.ParseServingCell(out)
	if err != nil {
		return nil, false
	}
	c, ok := qeng.SelectGeolocatable(cells)
	if !ok {
		return nil, false
	}
	return &geoloc.CellTower{
		Radio: string(c.Radio),
		MCC:   c.MCC,
		MNC:   c.MNC,
		CID:   c.CID,
		TAC:   c.TAC,
	}, true
}
