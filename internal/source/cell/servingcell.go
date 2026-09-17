// Package cell reads the modem's serving cell for blending into a WiFi
// geolocation request. Resolution is the provider's job.
package cell

import (
	"context"
	"fmt"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/qeng"
	"github.com/ckeller42/celloc/internal/ratelog"
)

// servingCellCmd is the AT command for the serving cell.
const servingCellCmd = `AT+QENG="servingcell"`

// ServingCellReader reads the modem's serving cell (AT+QENG) and returns it as a
// provider-neutral geoloc.CellTower, for blending into a WiFi geolocation request.
type ServingCellReader struct {
	Runner atrun.Runner

	logs ratelog.Limiter
}

// NewServingCellReader builds a reader over the given AT runner.
func NewServingCellReader(r atrun.Runner) *ServingCellReader {
	return &ServingCellReader{Runner: r}
}

// ServingCell implements wifi.CellReader. ok is false when the modem read fails
// or no geolocatable serving cell is present; each such case is logged
// (rate-limited) so a missing cell anchor is visible.
func (r *ServingCellReader) ServingCell(ctx context.Context) (*geoloc.CellTower, bool) {
	out, err := r.Runner.Run(ctx, servingCellCmd)
	if err != nil {
		r.logs.Printf("cell: serving-cell read failed: %v", err)
		return nil, false
	}
	cells, err := qeng.ParseServingCell(out)
	if err != nil {
		r.logs.Printf("cell: %v in modem reply %s", err, truncate(out))
		return nil, false
	}
	c, ok := qeng.SelectGeolocatable(cells)
	if !ok {
		r.logs.Printf("cell: no geolocatable serving cell among %d decoded (%s)", len(cells), radios(cells))
		return nil, false
	}
	return &geoloc.CellTower{
		Radio:  string(c.Radio),
		MCC:    c.MCC,
		MNC:    c.MNC,
		CID:    c.CID,
		TAC:    c.TAC,
		Signal: c.Signal,
	}, true
}

// truncate quotes s, capped so a verbose modem reply can't flood the log.
func truncate(s string) string {
	const maxLen = 120
	if len(s) > maxLen {
		return fmt.Sprintf("%q...", s[:maxLen])
	}
	return fmt.Sprintf("%q", s)
}

func radios(cells []qeng.Cell) string {
	rs := make([]string, 0, len(cells))
	for _, c := range cells {
		rs = append(rs, string(c.Radio))
	}
	return fmt.Sprint(rs)
}
