// Package cell implements source.Source via cell-tower geolocation: read the
// serving cell over AT, resolve it with OpenCelliD, and serve the result. It
// keeps the last good fix and serves it until StaleAfter so a single failed
// poll doesn't blank the position.
package cell

import (
	"context"
	"sync"
	"time"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/opencellid"
	"github.com/ckeller42/celloc/internal/qeng"
	"github.com/ckeller42/celloc/internal/source"
)

// servingCellCmd is the AT command for the serving cell.
const servingCellCmd = `AT+QENG="servingcell"`

// Resolver resolves a cell query to a location (satisfied by *opencellid.Client).
type Resolver interface {
	Lookup(ctx context.Context, q opencellid.Query) (opencellid.Location, opencellid.Status, error)
}

// Source is a cell-tower positioning source.
type Source struct {
	Runner     atrun.Runner
	Resolver   Resolver
	Radio      string        // OpenCelliD radio param (v1: "LTE")
	StaleAfter time.Duration // serve cached fix up to this age after a failed poll
	Now        func() time.Time

	mu     sync.Mutex
	last   source.Fix
	lastAt time.Time
}

// New builds a cell Source with a real clock.
func New(r atrun.Runner, res Resolver, radio string, staleAfter time.Duration) *Source {
	return &Source{Runner: r, Resolver: res, Radio: radio, StaleAfter: staleAfter, Now: time.Now}
}

// Name implements source.Source.
func (s *Source) Name() string { return "cell" }

// Fix implements source.Source: try a fresh resolve; on any failure fall back to
// a cached fix while it is younger than StaleAfter, else ErrNoFix.
func (s *Source) Fix(ctx context.Context) (source.Fix, error) {
	if f, ok := s.resolve(ctx); ok {
		s.store(f)
		return f, nil
	}
	return s.cached()
}

func (s *Source) resolve(ctx context.Context) (source.Fix, bool) {
	out, err := s.Runner.Run(ctx, servingCellCmd)
	if err != nil {
		return source.Fix{}, false
	}
	cells, err := qeng.ParseServingCell(out)
	if err != nil {
		return source.Fix{}, false
	}
	c, ok := qeng.SelectGeolocatable(cells)
	if !ok {
		return source.Fix{}, false
	}
	loc, st, err := s.Resolver.Lookup(ctx, opencellid.Query{
		MCC: c.MCC, MNC: c.MNC, LAC: c.TAC, CellID: c.CID, Radio: s.Radio,
	})
	if err != nil || st != opencellid.StatusOK {
		return source.Fix{}, false
	}
	r := float64(loc.Range)
	return source.Fix{
		Time: s.now(), Mode: 2,
		Lat: loc.Lat, Lon: loc.Lon, EPH: r, EPX: r, EPY: r,
		Source: "cell", Radio: string(c.Radio),
		MCC: c.MCC, MNC: c.MNC, CID: c.CID, TAC: c.TAC,
	}, true
}

func (s *Source) store(f source.Fix) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last, s.lastAt = f, s.now()
}

func (s *Source) cached() (source.Fix, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastAt.IsZero() || s.now().Sub(s.lastAt) >= s.StaleAfter {
		return source.Fix{}, source.ErrNoFix
	}
	return s.last, nil
}

func (s *Source) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
