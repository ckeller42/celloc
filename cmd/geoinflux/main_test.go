package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/gpsd"
)

type fakeWriter struct {
	lines []string
	err   error
}

func (w *fakeWriter) Write(_ context.Context, line string) error {
	w.lines = append(w.lines, line)
	return w.err
}

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func fixTPV(ts time.Time) gpsd.TPV {
	lat, lon, eph := 48.7, 9.1, 35.0
	return gpsd.TPV{
		Class: "TPV", Mode: 2, Time: ts.UTC().Format("2006-01-02T15:04:05.000Z07:00"),
		Lat: &lat, Lon: &lon, EPH: &eph, WifiFix: &gpsd.WifiFix{APCount: 3},
	}
}

func count(lines []string, prefix string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

func TestUploaderWritesStatusEvenWithoutFix(t *testing.T) {
	captureLog(t)
	now := time.Unix(1789632000, 0)
	w := &fakeWriter{}
	u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}

	u.onTPV(context.Background(), gpsd.TPV{Class: "TPV", Mode: 0})
	if len(w.lines) != 1 || w.lines[0] != "geo_status mode=0i,fix_age_s=-1,connected=true 1789632000000000000" {
		t.Fatalf("want one no-fix status line, got %q", w.lines)
	}
	now = now.Add(time.Second)
	u.onTPV(context.Background(), gpsd.TPV{Class: "TPV", Mode: 0})
	if len(w.lines) != 1 {
		t.Fatalf("status must be debounced by minInterval, got %q", w.lines)
	}
	now = now.Add(30 * time.Second)
	u.onTPV(context.Background(), gpsd.TPV{Class: "TPV", Mode: 0})
	if len(w.lines) != 2 {
		t.Fatalf("status due again after minInterval, got %q", w.lines)
	}
}

func TestUploaderWritesFixWithTimestampAndStatus(t *testing.T) {
	captureLog(t)
	now := time.Unix(1789632000, 0)
	w := &fakeWriter{}
	u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}

	u.onTPV(context.Background(), fixTPV(now.Add(-2*time.Second)))
	if count(w.lines, "geo_status mode=2i,fix_age_s=2,connected=true ") != 1 {
		t.Fatalf("status line missing/wrong: %q", w.lines)
	}
	if count(w.lines, "geo,source=wifi ") != 1 || !strings.HasSuffix(w.lines[len(w.lines)-1], " 1789631998000000000") {
		t.Fatalf("fix line missing or without fix timestamp: %q", w.lines)
	}
	now = now.Add(time.Second)
	u.onTPV(context.Background(), fixTPV(now))
	if len(w.lines) != 2 {
		t.Fatalf("writes must be debounced, got %q", w.lines)
	}
}

func TestUploaderDisconnectedStatus(t *testing.T) {
	captureLog(t)
	now := time.Unix(1789632000, 0)
	w := &fakeWriter{err: errors.New("influx down")}
	u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}

	u.disconnected(context.Background())
	u.disconnected(context.Background()) // debounced
	if len(w.lines) != 1 || w.lines[0] != "geo_status mode=0i,fix_age_s=-1,connected=false 1789632000000000000" {
		t.Fatalf("want one disconnected status line, got %q", w.lines)
	}
}

func TestUploaderLogsFixTransitionsOnce(t *testing.T) {
	buf := captureLog(t)
	now := time.Unix(1789632000, 0)
	u := &uploader{w: &fakeWriter{}, minInterval: 30 * time.Second, now: func() time.Time { return now }}
	noFix := gpsd.TPV{Class: "TPV", Mode: 0}
	for _, tpv := range []gpsd.TPV{fixTPV(now), fixTPV(now), noFix, noFix, noFix, fixTPV(now)} {
		now = now.Add(time.Second)
		u.onTPV(context.Background(), tpv)
	}
	out := buf.String()
	if n := strings.Count(out, "fix lost"); n != 1 {
		t.Fatalf("want 1 'fix lost', got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "fix acquired"); n != 2 {
		t.Fatalf("want 2 'fix acquired', got %d:\n%s", n, out)
	}
}

func TestUploaderStatusConnectionChangeBypassesDebounce(t *testing.T) {
	captureLog(t)
	now := time.Unix(1789632000, 0)
	w := &fakeWriter{}
	u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}

	u.onTPV(context.Background(), gpsd.TPV{Class: "TPV", Mode: 0})
	now = now.Add(time.Second)
	u.disconnected(context.Background()) // state changed: must not wait for the interval
	now = now.Add(time.Second)
	u.disconnected(context.Background()) // same state: debounced
	now = now.Add(time.Second)
	u.onTPV(context.Background(), gpsd.TPV{Class: "TPV", Mode: 0}) // reconnected: immediate
	if count(w.lines, "geo_status ") != 3 ||
		!strings.Contains(w.lines[1], "connected=false") || !strings.Contains(w.lines[2], "connected=true") {
		t.Fatalf("want connected, disconnected, connected status lines, got %q", w.lines)
	}
}

func TestUploaderLogsBadTPVTimeRateLimited(t *testing.T) {
	buf := captureLog(t)
	now := time.Unix(1789632000, 0)
	w := &fakeWriter{}
	u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}
	tpv := fixTPV(now)
	tpv.Time = "yesterday-ish"
	for i := 0; i < 3; i++ {
		now = now.Add(time.Second)
		u.onTPV(context.Background(), tpv)
	}
	if n := strings.Count(buf.String(), "unparsable TPV time"); n != 1 || !strings.Contains(buf.String(), "yesterday-ish") {
		t.Fatalf("want bad TPV time logged once, got %d:\n%s", n, buf.String())
	}
	if count(w.lines, "geo,source=wifi ") != 1 {
		t.Fatalf("fix with bad time must still be written: %q", w.lines)
	}
}

// TestUploaderSkipsUnattributedFix: a mode>=2 TPV with neither wifix nor
// cellfix is not a celloc fix. It must never become a geo point (least of all
// one with an empty source tag), but the heartbeat still reports its mode and
// the skip is logged, rate-limited.
func TestUploaderSkipsUnattributedFix(t *testing.T) {
	bare := func(ts time.Time) gpsd.TPV {
		tpv := fixTPV(ts)
		tpv.WifiFix = nil
		return tpv
	}
	tests := []struct {
		name      string
		tpvs      func(now time.Time) []gpsd.TPV
		wantGeo   int
		wantSkips int
	}{
		{"bare fix only", func(now time.Time) []gpsd.TPV { return []gpsd.TPV{bare(now), bare(now), bare(now)} }, 0, 1},
		{"bare then wifi is written immediately", func(now time.Time) []gpsd.TPV { return []gpsd.TPV{bare(now), fixTPV(now)} }, 1, 1},
		{"cellfix is attributed", func(now time.Time) []gpsd.TPV {
			tpv := bare(now)
			tpv.CellFix = &gpsd.CellFix{Radio: "LTE", MCC: 262, MNC: 3, CID: 1, TAC: 2}
			return []gpsd.TPV{tpv}
		}, 1, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buf := captureLog(t)
			now := time.Unix(1789632000, 0)
			w := &fakeWriter{}
			u := &uploader{w: w, minInterval: 30 * time.Second, now: func() time.Time { return now }}
			for _, tpv := range tc.tpvs(now) {
				now = now.Add(time.Second)
				u.onTPV(context.Background(), tpv)
			}
			for _, l := range w.lines {
				if strings.Contains(l, "source=,") || strings.Contains(l, "source= ") {
					t.Fatalf("empty source tag written: %q", l)
				}
			}
			if got := count(w.lines, "geo,"); got != tc.wantGeo {
				t.Fatalf("geo points = %d, want %d: %q", got, tc.wantGeo, w.lines)
			}
			if count(w.lines, "geo_status mode=2i,") < 1 {
				t.Fatalf("heartbeat must still report mode=2: %q", w.lines)
			}
			if got := strings.Count(buf.String(), "skipping geo point"); got != tc.wantSkips {
				t.Fatalf("skip logs = %d, want %d:\n%s", got, tc.wantSkips, buf.String())
			}
			if tc.wantSkips > 0 && !strings.Contains(buf.String(), "fix acquired (no wifix/cellfix") {
				t.Fatalf("fix-acquired log must name the missing extension:\n%s", buf.String())
			}
		})
	}
}
