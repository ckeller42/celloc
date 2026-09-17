// Package ratelog throttles repeated log lines so a persistent failure is
// visible without flooding the router's small log buffer.
package ratelog

import (
	"log"
	"sync"
	"time"
)

// DefaultEvery is the interval used by a zero-value Limiter.
const DefaultEvery = time.Minute

// Limiter lets each key through at most once per Every. The zero value is ready
// to use (DefaultEvery, real clock) and is safe for concurrent use; it must not
// be copied after first use.
type Limiter struct {
	Every time.Duration    // 0 means DefaultEvery
	Now   func() time.Time // nil means time.Now

	mu   sync.Mutex
	last map[string]time.Time
}

// Allow reports whether a line for key may be emitted now, and if so records it.
func (l *Limiter) Allow(key string) bool {
	now := time.Now()
	if l.Now != nil {
		now = l.Now()
	}
	every := l.Every
	if every <= 0 {
		every = DefaultEvery
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if at, ok := l.last[key]; ok && now.Sub(at) < every {
		return false
	}
	if l.last == nil {
		l.last = map[string]time.Time{}
	}
	l.last[key] = now
	return true
}

// Printf logs via the standard logger, throttled per format string: repeats of
// the same kind of failure are suppressed even when their details differ, while
// a different kind still logs immediately.
func (l *Limiter) Printf(format string, args ...any) {
	if l.Allow(format) {
		log.Printf(format, args...)
	}
}
