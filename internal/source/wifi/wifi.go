// Package wifi implements source.Source via WiFi-AP geolocation: scan nearby
// access points, resolve them via a configured Resolver (e.g. Google or Unwired
// Labs), and serve the result. It keeps the last good fix until StaleAfter so a
// single failed scan or lookup doesn't blank the position. Mirrors
// internal/source/cell.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/wifiscan"
)

// resolveLogThrottle bounds how often an unchanged failure is logged.
const resolveLogThrottle = time.Minute

// errFewAPs means the scan returned fewer than MinAPs usable access points.
var errFewAPs = errors.New("wifi: too few access points")

// Scanner produces nearby access points (satisfied by wifiscan.Scanner).
type Scanner interface {
	Scan(ctx context.Context) ([]wifiscan.AP, error)
}

// CellReader returns the current serving cell to blend into the resolver request
// (satisfied by *cell.ServingCellReader). ok is false when no cell is available.
type CellReader interface {
	ServingCell(ctx context.Context) (*geoloc.CellTower, bool)
}

// Resolver resolves scanned APs to a location (satisfied by *unwiredlabs.Client
// and *google.Client). A nil error means the Location is usable; a non-nil error
// is a classified provider failure (e.g. "unwiredlabs: auth").
type Resolver interface {
	Resolve(ctx context.Context, aps []wifiscan.AP, cell *geoloc.CellTower) (geoloc.Location, error)
}

// Source is a WiFi-AP positioning source.
type Source struct {
	Scanner    Scanner
	Resolver   Resolver
	Cell       CellReader // optional: blends the serving cell into the request
	MinAPs     int
	StaleAfter time.Duration
	Now        func() time.Time
	Logf       func(format string, args ...any)

	mu         sync.Mutex
	last       source.Fix
	lastAt     time.Time
	lastLogMsg string
	lastLogAt  time.Time
}

// New builds a wifi Source with a real clock and log.Printf as the log sink.
func New(sc Scanner, res Resolver, minAPs int, staleAfter time.Duration) *Source {
	return &Source{
		Scanner: sc, Resolver: res, MinAPs: minAPs, StaleAfter: staleAfter,
		Now: time.Now, Logf: log.Printf,
	}
}

// Name implements source.Source.
func (s *Source) Name() string { return "wifi" }

// Fix implements source.Source: try a fresh scan+resolve; on any failure log it
// (throttled) and fall back to a cached fix younger than StaleAfter, else ErrNoFix.
func (s *Source) Fix(ctx context.Context) (source.Fix, error) {
	f, err := s.resolve(ctx)
	if err == nil {
		s.store(f)
		return f, nil
	}
	s.logResolve(err)
	return s.cached()
}

func (s *Source) resolve(ctx context.Context) (source.Fix, error) {
	aps, scanErr := s.Scanner.Scan(ctx)
	haveAPs := scanErr == nil && len(aps) >= s.MinAPs
	if !haveAPs {
		aps = nil // never send a below-threshold AP set
	}

	// Blend the serving cell when available; it anchors the fix (or resolves it
	// alone when WiFi is too sparse).
	var cell *geoloc.CellTower
	if s.Cell != nil {
		if c, ok := s.Cell.ServingCell(ctx); ok {
			cell = c
		}
	}

	if !haveAPs && cell == nil {
		if scanErr != nil {
			return source.Fix{}, fmt.Errorf("wifi scan: %w", scanErr)
		}
		return source.Fix{}, errFewAPs
	}

	loc, err := s.Resolver.Resolve(ctx, aps, cell)
	if err != nil {
		return source.Fix{}, err
	}
	r := loc.Accuracy
	f := source.Fix{
		Time: s.now(), Mode: 2,
		Lat: loc.Lat, Lon: loc.Lon, EPH: r, EPX: r, EPY: r,
	}
	// Tag by the dominant signal: WiFi when APs were sent, else the cell (and
	// carry the cell IDs so the TPV/InfluxDB line stays honest).
	if haveAPs {
		f.Source, f.APCount = "wifi", len(aps)
	} else {
		f.Source = "cell"
		f.Radio, f.MCC, f.MNC, f.CID, f.TAC = cell.Radio, cell.MCC, cell.MNC, cell.CID, cell.TAC
	}
	return f, nil
}

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
	s.Logf("wifi: resolve failed: %s (serving cached fix until stale)", msg)
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
