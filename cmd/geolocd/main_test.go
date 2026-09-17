package main

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/source"
)

// fixFunc adapts a func to source.Source.
type fixFunc func(ctx context.Context) (source.Fix, error)

func (fixFunc) Name() string                                  { return "stub" }
func (f fixFunc) Fix(ctx context.Context) (source.Fix, error) { return f(ctx) }

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

func TestPollOnceBoundedByPollTimeout(t *testing.T) {
	old := pollTimeout
	pollTimeout = 50 * time.Millisecond
	t.Cleanup(func() { pollTimeout = old })
	captureLog(t)

	// A wedged source (e.g. gl_modem hung) blocks until its ctx is done. The
	// daemon ctx never ends, so only the per-poll timeout can unblock it.
	wedged := fixFunc(func(ctx context.Context) (source.Fix, error) {
		<-ctx.Done()
		return source.Fix{}, ctx.Err()
	})
	var cur atomic.Value
	cur.Store(source.Fix{Mode: 2, Lat: 1, Lon: 2})
	hadFix := true

	done := make(chan struct{})
	go func() {
		pollOnce(context.Background(), wedged, &cur, &hadFix)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("pollOnce not bounded by pollTimeout")
	}
	if cur.Load().(source.Fix).HasFix() {
		t.Fatal("timed-out poll must clear the served fix, not keep serving the last one")
	}
}

func TestPollOnceLogsTransitionsOnce(t *testing.T) {
	buf := captureLog(t)
	good := source.Fix{Mode: 2, Lat: 48.7, Lon: 9.1, Source: "wifi"}
	seq := []error{nil, errors.New("boom"), errors.New("boom"), nil, nil}
	i := 0
	src := fixFunc(func(context.Context) (source.Fix, error) {
		err := seq[i]
		i++
		if err != nil {
			return source.Fix{}, err
		}
		return good, nil
	})
	var cur atomic.Value
	cur.Store(source.Fix{})
	hadFix := false
	for range seq {
		pollOnce(context.Background(), src, &cur, &hadFix)
	}
	out := buf.String()
	if n := strings.Count(out, "position lost"); n != 1 {
		t.Fatalf("want 1 'position lost' log, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "boom") {
		t.Fatalf("lost-log must carry the error:\n%s", out)
	}
	if n := strings.Count(out, "position acquired"); n != 2 { // startup + recovery
		t.Fatalf("want 2 'position acquired' logs, got %d:\n%s", n, out)
	}
	if !cur.Load().(source.Fix).HasFix() {
		t.Fatal("final fix should be served")
	}
}
