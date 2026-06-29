package cell_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/opencellid"
	"github.com/ckeller42/celloc/internal/source/cell"
)

const lteOut = `+QENG: "LTE","FDD",262,03,1684B3E,204,3350,7,5,5,E8E5,-83`

type runnerFunc func(context.Context, string) (string, error)

func (f runnerFunc) Run(ctx context.Context, cmd string) (string, error) { return f(ctx, cmd) }

type resolverFunc func(context.Context, opencellid.Query) (opencellid.Location, opencellid.Status, error)

func (f resolverFunc) Lookup(ctx context.Context, q opencellid.Query) (opencellid.Location, opencellid.Status, error) {
	return f(ctx, q)
}

func okResolver(q opencellid.Query) resolverFunc {
	return func(_ context.Context, got opencellid.Query) (opencellid.Location, opencellid.Status, error) {
		_ = q
		_ = got
		return opencellid.Location{Lat: 48.7698, Lon: 9.1676, Range: 1548}, opencellid.StatusOK, nil
	}
}

func TestFixHappyPath(t *testing.T) {
	var gotQ opencellid.Query
	res := resolverFunc(func(_ context.Context, q opencellid.Query) (opencellid.Location, opencellid.Status, error) {
		gotQ = q
		return opencellid.Location{Lat: 48.7698, Lon: 9.1676, Range: 1548}, opencellid.StatusOK, nil
	})
	s := cell.New(runnerFunc(func(context.Context, string) (string, error) { return lteOut, nil }), res, "LTE", time.Minute)
	f, err := s.Fix(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != 2 || f.Lat != 48.7698 || f.EPH != 1548 || f.EPX != 1548 {
		t.Fatalf("bad fix: %+v", f)
	}
	if f.MCC != 262 || f.MNC != 3 || f.CID != 0x1684B3E || f.TAC != 0xE8E5 {
		t.Fatalf("bad ids: %+v", f)
	}
	if gotQ.LAC != 0xE8E5 || gotQ.CellID != 0x1684B3E || gotQ.Radio != "LTE" {
		t.Fatalf("bad query to resolver: %+v", gotQ)
	}
}

func TestFixNoLTELineIsNoFix(t *testing.T) {
	nsa := `+QENG: "NR5G-NSA",262,03,451,-78,26,-10,638304,78,9,1`
	s := cell.New(runnerFunc(func(context.Context, string) (string, error) { return nsa, nil }),
		okResolver(opencellid.Query{}), "LTE", time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix when no geolocatable cell")
	}
}

func TestFixUnknownCellNoFix(t *testing.T) {
	res := resolverFunc(func(context.Context, opencellid.Query) (opencellid.Location, opencellid.Status, error) {
		return opencellid.Location{}, opencellid.StatusUnknownCell, nil
	})
	s := cell.New(runnerFunc(func(context.Context, string) (string, error) { return lteOut, nil }), res, "LTE", time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("unknown cell should be ErrNoFix")
	}
}

func TestCachedFixServedThenStale(t *testing.T) {
	now := time.Unix(1000, 0)
	clock := func() time.Time { return now }
	runErr := false
	runner := runnerFunc(func(context.Context, string) (string, error) {
		if runErr {
			return "", errors.New("modem busy")
		}
		return lteOut, nil
	})
	s := cell.New(runner, okResolver(opencellid.Query{}), "LTE", 90*time.Second)
	s.Now = clock

	// 1) first poll caches a good fix
	if _, err := s.Fix(context.Background()); err != nil {
		t.Fatalf("first fix: %v", err)
	}
	// 2) modem fails but within StaleAfter -> cached fix still served
	runErr = true
	now = now.Add(30 * time.Second)
	if f, err := s.Fix(context.Background()); err != nil || f.Lat != 48.7698 {
		t.Fatalf("cached fix expected, got %+v err=%v", f, err)
	}
	// 3) past StaleAfter -> no fix
	now = now.Add(120 * time.Second)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("stale cache should yield ErrNoFix")
	}
}

func TestResolveFailureIsLoggedAndClassified(t *testing.T) {
	now := time.Unix(2000, 0)
	var logs []string
	res := resolverFunc(func(context.Context, opencellid.Query) (opencellid.Location, opencellid.Status, error) {
		return opencellid.Location{}, opencellid.StatusAuth, nil
	})
	s := &cell.Source{
		Runner:     runnerFunc(func(context.Context, string) (string, error) { return lteOut, nil }),
		Resolver:   res,
		Radio:      "LTE",
		StaleAfter: time.Minute,
		Now:        func() time.Time { return now },
		Logf:       func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) },
	}

	// A bad key (StatusAuth) must surface a distinct, diagnosable log line.
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix on auth failure with empty cache")
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "opencellid: auth") {
		t.Fatalf("want one log mentioning 'opencellid: auth', got %v", logs)
	}

	// Same failure within the throttle window is suppressed.
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix")
	}
	if len(logs) != 1 {
		t.Fatalf("repeat within throttle should be suppressed, got %v", logs)
	}

	// After the throttle window it logs again.
	now = now.Add(2 * time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix")
	}
	if len(logs) != 2 {
		t.Fatalf("want a second log after throttle window, got %v", logs)
	}
}

func TestRunnerErrorNoCacheNoPanic(t *testing.T) {
	s := cell.New(runnerFunc(func(context.Context, string) (string, error) { return "", errors.New("x") }),
		okResolver(opencellid.Query{}), "LTE", time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix")
	}
}
