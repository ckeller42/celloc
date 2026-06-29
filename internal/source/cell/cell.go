// Package cell implements source.Source via cell-tower geolocation: read the
// serving cell over AT, resolve it with OpenCelliD, and serve the result. It
// keeps the last good fix and serves it until StaleAfter so a single failed
// poll doesn't blank the position.
package cell

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/opencellid"
	"github.com/ckeller42/celloc/internal/qeng"
	"github.com/ckeller42/celloc/internal/source"
)

// servingCellCmd is the AT command for the serving cell.
const servingCellCmd = `AT+QENG="servingcell"`

// resolveLogThrottle bounds how often an unchanged resolve failure is logged, so
// a permanent fault (e.g. a bad key -> StatusAuth, retried every poll) is visible
// in logread without flooding it.
const resolveLogThrottle = time.Minute

// errNoServingCell means QENG returned no geolocatable cell (e.g. NR5G-only site
// with no LTE anchor). It is expected on some networks, distinct from a lookup or
// modem error, so callers can choose to log it less loudly.
var errNoServingCell = errors.New("no geolocatable serving cell")

// Resolver resolves a cell query to a location (satisfied by *opencellid.Client).
type Resolver interface {
	Lookup(ctx context.Context, q opencellid.Query) (opencellid.Location, opencellid.Status, error)
}

// Source is a cell-tower positioning source.
type Source struct {
	Runner     atrun.Runner
	Resolver   Resolver
	Radio      string                           // OpenCelliD radio param (v1: "LTE")
	StaleAfter time.Duration                    // serve cached fix up to this age after a failed poll
	Now        func() time.Time                 // injectable clock (defaults to time.Now)
	Logf       func(format string, args ...any) // resolve-failure log sink (nil = silent)

	mu         sync.Mutex
	last       source.Fix
	lastAt     time.Time
	lastLogMsg string
	lastLogAt  time.Time
}

// New builds a cell Source with a real clock and log.Printf as the log sink.
func New(r atrun.Runner, res Resolver, radio string, staleAfter time.Duration) *Source {
	return &Source{
		Runner: r, Resolver: res, Radio: radio, StaleAfter: staleAfter,
		Now: time.Now, Logf: log.Printf,
	}
}

// Name implements source.Source.
func (s *Source) Name() string { return "cell" }

// Fix implements source.Source: try a fresh resolve; on any failure log it
// (throttled) and fall back to a cached fix while it is younger than StaleAfter,
// else ErrNoFix.
func (s *Source) Fix(ctx context.Context) (source.Fix, error) {
	f, err := s.resolve(ctx)
	if err == nil {
		s.store(f)
		return f, nil
	}
	s.logResolve(err)
	return s.cached()
}

// resolve performs one fresh lookup. A nil error means f is a usable fix; a
// non-nil error is classified (modem / parse / no-cell / lookup / status) so the
// caller can log a diagnosable reason.
func (s *Source) resolve(ctx context.Context) (source.Fix, error) {
	out, err := s.Runner.Run(ctx, servingCellCmd)
	if err != nil {
		return source.Fix{}, fmt.Errorf("modem AT read: %w", err)
	}
	cells, err := qeng.ParseServingCell(out)
	if err != nil {
		return source.Fix{}, fmt.Errorf("parse QENG: %w", err)
	}
	c, ok := qeng.SelectGeolocatable(cells)
	if !ok {
		return source.Fix{}, errNoServingCell
	}
	loc, st, err := s.Resolver.Lookup(ctx, opencellid.Query{
		MCC: c.MCC, MNC: c.MNC, LAC: c.TAC, CellID: c.CID, Radio: s.Radio,
	})
	if err != nil {
		return source.Fix{}, fmt.Errorf("opencellid lookup: %w", err)
	}
	if st != opencellid.StatusOK {
		return source.Fix{}, fmt.Errorf("opencellid: %s", st)
	}
	r := float64(loc.Range)
	return source.Fix{
		Time: s.now(), Mode: 2,
		Lat: loc.Lat, Lon: loc.Lon, EPH: r, EPX: r, EPY: r,
		Source: "cell", Radio: string(c.Radio),
		MCC: c.MCC, MNC: c.MNC, CID: c.CID, TAC: c.TAC,
	}, nil
}

// logResolve logs a resolve failure via Logf, suppressing repeats of the same
// message within resolveLogThrottle so a steady-state fault is visible but not
// spammed.
func (s *Source) logResolve(err error) {
	if s.Logf == nil {
		return
	}
	msg := err.Error()
	s.mu.Lock()
	now := s.now()
	if msg == s.lastLogMsg && now.Sub(s.lastLogAt) < resolveLogThrottle {
		s.mu.Unlock()
		return
	}
	s.lastLogMsg, s.lastLogAt = msg, now
	s.mu.Unlock()
	s.Logf("cell: resolve failed: %s (serving cached fix until stale)", msg)
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
