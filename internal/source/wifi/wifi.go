// Package wifi implements source.Source via WiFi-AP geolocation: scan nearby
// access points, resolve them with the Unwired Labs LocationAPI, and serve the
// result. It keeps the last good fix until StaleAfter so a single failed scan or
// lookup doesn't blank the position. Mirrors internal/source/cell.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
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

// Resolver resolves APs to a location (satisfied by *unwiredlabs.Client).
type Resolver interface {
	LookupWifi(ctx context.Context, aps []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error)
}

// Source is a WiFi-AP positioning source.
type Source struct {
	Scanner    Scanner
	Resolver   Resolver
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
	aps, err := s.Scanner.Scan(ctx)
	if err != nil {
		return source.Fix{}, fmt.Errorf("wifi scan: %w", err)
	}
	if len(aps) < s.MinAPs {
		return source.Fix{}, errFewAPs
	}
	wlist := make([]unwiredlabs.WifiAP, 0, len(aps))
	for _, ap := range aps {
		wlist = append(wlist, unwiredlabs.WifiAP{BSSID: ap.BSSID, Signal: ap.Signal})
	}
	loc, st, err := s.Resolver.LookupWifi(ctx, wlist)
	if err != nil {
		return source.Fix{}, fmt.Errorf("unwiredlabs lookup: %w", err)
	}
	if st != unwiredlabs.StatusOK {
		return source.Fix{}, fmt.Errorf("unwiredlabs: %s", st)
	}
	r := loc.Accuracy
	return source.Fix{
		Time: s.now(), Mode: 2,
		Lat: loc.Lat, Lon: loc.Lon, EPH: r, EPX: r, EPY: r,
		Source: "wifi", APCount: len(aps),
	}, nil
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
