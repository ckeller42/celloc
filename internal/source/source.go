// Package source defines the pluggable positioning source interface and the
// Fix value type shared across celloc. Keeping the interface and Fix here (with
// no I/O) lets the gpsd, influx and selection logic stay pure and unit-testable.
package source

import (
	"context"
	"errors"
	"time"
)

// ErrNoFix is returned by a Source that currently has no usable position.
var ErrNoFix = errors.New("source: no fix")

// Fix is a single position estimate from some source. A cell-tower fix is 2D
// (Mode 2) with an isotropic horizontal error radius (EPH=EPX=EPY) taken from
// the geolocation provider's range. It deliberately carries no altitude/speed —
// those are unknown for a cell fix and must not be faked.
type Fix struct {
	Time     time.Time
	Mode     int     // gpsd TPV mode: 0/1 = no fix, 2 = 2D
	Lat, Lon float64 // WGS84 degrees
	EPH      float64 // horizontal position error, meters (1-sigma-ish)
	EPX, EPY float64 // longitude/latitude error, meters (== EPH for a cell fix)

	Source string // e.g. "cell"
	Radio  string // e.g. "LTE"
	MCC    int
	MNC    int
	CID    int64
	TAC    int
}

// HasFix reports whether the fix carries a usable position.
func (f Fix) HasFix() bool { return f.Mode >= 2 }

// Source produces the current best position estimate. Implementations must be
// safe for concurrent use by the daemon's server and poll loop.
type Source interface {
	// Name identifies the source (e.g. "cell", "gnss") for logging.
	Name() string
	// Fix returns the current best fix, or ErrNoFix when none is available.
	Fix(ctx context.Context) (Fix, error)
}

// Select returns the first source (in priority order) that yields a fix. This
// is how GNSS can later outrank cell without changing the daemon: pass it as
// the earlier source. Returns ErrNoFix if every source has none.
func Select(ctx context.Context, sources ...Source) (Fix, error) {
	for _, s := range sources {
		if s == nil {
			continue
		}
		f, err := s.Fix(ctx)
		if err == nil && f.HasFix() {
			return f, nil
		}
	}
	return Fix{}, ErrNoFix
}
