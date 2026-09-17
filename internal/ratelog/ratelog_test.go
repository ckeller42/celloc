package ratelog_test

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/ratelog"
)

func TestLimiterAllowsOncePerIntervalPerKey(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &ratelog.Limiter{Every: time.Minute, Now: func() time.Time { return now }}
	steps := []struct {
		advance time.Duration
		key     string
		want    bool
	}{
		{0, "a", true},
		{time.Second, "a", false},
		{0, "b", true}, // a different kind of failure is not hidden
		{58 * time.Second, "a", false},
		{time.Second, "a", true}, // interval elapsed (60s since first "a")
		{time.Second, "b", true}, // 60s since first "b"
		{0, "b", false},
	}
	for i, s := range steps {
		now = now.Add(s.advance)
		if got := l.Allow(s.key); got != s.want {
			t.Fatalf("step %d key %q: Allow=%v want %v", i, s.key, got, s.want)
		}
	}
}

func TestLimiterZeroValueDefaultsInterval(t *testing.T) {
	var l ratelog.Limiter
	if !l.Allow("k") {
		t.Fatal("first call must pass")
	}
	if l.Allow("k") {
		t.Fatal("zero-value limiter must still throttle (DefaultEvery)")
	}
}

func TestPrintfKeysOnFormat(t *testing.T) {
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })

	var l ratelog.Limiter
	l.Printf("x failed: %v", "one")
	l.Printf("x failed: %v", "two") // same format, different detail: throttled
	l.Printf("y failed: %v", "three")
	out := buf.String()
	if !strings.Contains(out, "x failed: one") || strings.Contains(out, "two") || !strings.Contains(out, "y failed: three") {
		t.Fatalf("unexpected log output:\n%s", out)
	}
}
