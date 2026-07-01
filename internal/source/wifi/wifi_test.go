package wifi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/geoloc"
	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/source/wifi"
	"github.com/ckeller42/celloc/internal/wifiscan"
)

type scanFunc func(context.Context) ([]wifiscan.AP, error)

func (f scanFunc) Scan(ctx context.Context) ([]wifiscan.AP, error) { return f(ctx) }

type resFunc func(context.Context, []wifiscan.AP, *geoloc.CellTower) (geoloc.Location, error)

func (f resFunc) Resolve(ctx context.Context, a []wifiscan.AP, c *geoloc.CellTower) (geoloc.Location, error) {
	return f(ctx, a, c)
}

type cellFunc func(context.Context) (*geoloc.CellTower, bool)

func (f cellFunc) ServingCell(ctx context.Context) (*geoloc.CellTower, bool) { return f(ctx) }

func lteCell() cellFunc {
	return cellFunc(func(context.Context) (*geoloc.CellTower, bool) {
		return &geoloc.CellTower{Radio: "LTE", MCC: 262, MNC: 3, CID: 23612222, TAC: 59621}, true
	})
}

func threeAPs(context.Context) ([]wifiscan.AP, error) {
	return []wifiscan.AP{{BSSID: "a", Signal: -40}, {BSSID: "b", Signal: -50}, {BSSID: "c", Signal: -60}}, nil
}

func okRes() resFunc {
	return resFunc(func(context.Context, []wifiscan.AP, *geoloc.CellTower) (geoloc.Location, error) {
		return geoloc.Location{Lat: 48.77, Lon: 9.17, Accuracy: 30}, nil
	})
}

func TestWifiFixHappyPath(t *testing.T) {
	var gotN int
	res := resFunc(func(_ context.Context, a []wifiscan.AP, _ *geoloc.CellTower) (geoloc.Location, error) {
		gotN = len(a)
		return geoloc.Location{Lat: 48.77, Lon: 9.17, Accuracy: 30}, nil
	})
	s := wifi.New(scanFunc(threeAPs), res, 2, time.Minute)
	f, err := s.Fix(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != 2 || f.Source != "wifi" || f.EPH != 30 || f.APCount != 3 || gotN != 3 {
		t.Fatalf("bad fix: %+v gotN=%d", f, gotN)
	}
}

func TestWifiTooFewAPsIsNoFix(t *testing.T) {
	one := scanFunc(func(context.Context) ([]wifiscan.AP, error) {
		return []wifiscan.AP{{BSSID: "a", Signal: -40}}, nil
	})
	s := wifi.New(one, okRes(), 2, time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix when below min APs")
	}
}

func TestWifiAuthFailureLogged(t *testing.T) {
	var logs int
	res := resFunc(func(context.Context, []wifiscan.AP, *geoloc.CellTower) (geoloc.Location, error) {
		return geoloc.Location{}, errors.New("unwiredlabs: auth")
	})
	s := wifi.New(scanFunc(threeAPs), res, 2, time.Minute)
	s.Logf = func(string, ...any) { logs++ }
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix on auth")
	}
	if logs != 1 {
		t.Fatalf("want one log, got %d", logs)
	}
}

func TestWifiCachedThenStale(t *testing.T) {
	now := time.Unix(1000, 0)
	scanErr := false
	sc := scanFunc(func(context.Context) ([]wifiscan.AP, error) {
		if scanErr {
			return nil, errors.New("iw failed")
		}
		return threeAPs(context.Background())
	})
	s := wifi.New(sc, okRes(), 2, 90*time.Second)
	s.Now = func() time.Time { return now }
	if _, err := s.Fix(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	scanErr = true
	now = now.Add(30 * time.Second)
	if f, err := s.Fix(context.Background()); err != nil || f.Lat != 48.77 {
		t.Fatalf("cached expected: %+v err=%v", f, err)
	}
	now = now.Add(120 * time.Second)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("stale cache should be ErrNoFix")
	}
}

func TestWifiOutranksCell(t *testing.T) {
	w := wifi.New(scanFunc(threeAPs), okRes(), 2, time.Minute)
	cell := stubSource{f: source.Fix{Mode: 2, Source: "cell", Lat: 1, Lon: 1}}
	f, err := source.Select(context.Background(), w, cell)
	if err != nil || f.Source != "wifi" {
		t.Fatalf("want wifi selected, got %+v err=%v", f, err)
	}
}

func TestWifiBlendsCell(t *testing.T) {
	var gotCell *geoloc.CellTower
	res := resFunc(func(_ context.Context, _ []wifiscan.AP, c *geoloc.CellTower) (geoloc.Location, error) {
		gotCell = c
		return geoloc.Location{Lat: 48.77, Lon: 9.17, Accuracy: 20}, nil
	})
	s := wifi.New(scanFunc(threeAPs), res, 2, time.Minute)
	s.Cell = lteCell()
	f, err := s.Fix(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != "wifi" || f.APCount != 3 {
		t.Fatalf("wifi-dominant fix expected: %+v", f)
	}
	if gotCell == nil || gotCell.CID != 23612222 {
		t.Fatalf("serving cell not blended into request: %+v", gotCell)
	}
}

func TestWifiCellOnlyWhenTooFewAPs(t *testing.T) {
	one := scanFunc(func(context.Context) ([]wifiscan.AP, error) {
		return []wifiscan.AP{{BSSID: "a", Signal: -40}}, nil // below MinAPs
	})
	res := resFunc(func(_ context.Context, aps []wifiscan.AP, c *geoloc.CellTower) (geoloc.Location, error) {
		if len(aps) != 0 || c == nil {
			return geoloc.Location{}, errors.New("expected cell-only request")
		}
		return geoloc.Location{Lat: 48.7, Lon: 9.1, Accuracy: 1400}, nil
	})
	s := wifi.New(one, res, 2, time.Minute)
	s.Cell = lteCell()
	f, err := s.Fix(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Source != "cell" || f.APCount != 0 || f.CID != 23612222 || f.EPH != 1400 {
		t.Fatalf("cell-only fix expected with cell IDs: %+v", f)
	}
}

type stubSource struct{ f source.Fix }

func (s stubSource) Name() string                            { return "cell" }
func (s stubSource) Fix(context.Context) (source.Fix, error) { return s.f, nil }
