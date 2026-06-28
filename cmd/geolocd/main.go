// Command geolocd is the celloc router daemon: it reads the modem serving cell,
// resolves it via OpenCelliD, and serves the position over the gpsd protocol.
// Configuration (incl. the OpenCelliD key) comes from uci (/etc/config/geolocd)
// so the key never appears in argv/ps.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/opencellid"
	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/source/cell"
	"github.com/ckeller42/celloc/internal/uciconf"
)

// Version is overridable via -ldflags "-X main.Version=v1.2.3".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("geolocd: %v", err)
	}
}

func run() error {
	streamEvery := flag.Duration("stream", time.Second, "gpsd TPV stream cadence to watching clients")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg, err := uciconf.Load(ctx)
	if err != nil {
		log.Printf("geolocd: uci load failed (%v); using defaults", err)
	}
	if cfg.Key == "" {
		return errNoKey
	}

	staleAfter := 2 * cfg.PollInterval
	if staleAfter < 2*time.Minute {
		staleAfter = 2 * time.Minute
	}
	src := cell.New(
		atrun.New(cfg.Runner, cfg.Bus),
		&opencellid.Client{Key: cfg.Key, HTTP: &http.Client{Timeout: 15 * time.Second}},
		cfg.Radio, staleAfter,
	)

	// Shared current fix, refreshed by the poll loop, read by the gpsd server.
	var cur atomic.Value
	cur.Store(source.Fix{Mode: 0})
	go pollLoop(ctx, src, cfg.PollInterval, &cur)

	srv := &gpsd.Server{
		Provider: func() source.Fix { return cur.Load().(source.Fix) },
		Interval: *streamEvery,
		Release:  "celloc-" + Version,
	}
	log.Printf("geolocd %s: listening on %s (radio=%s, poll=%s)", Version, cfg.Listen, cfg.Radio, cfg.PollInterval)
	return srv.ListenAndServe(ctx, cfg.Listen)
}

var errNoKey = errInvalidConfig("no OpenCelliD key — set it with: uci set geolocd.main.key='pk...'; uci commit geolocd")

type errInvalidConfig string

func (e errInvalidConfig) Error() string { return string(e) }

func pollLoop(ctx context.Context, src source.Source, every time.Duration, cur *atomic.Value) {
	poll := func() {
		f, err := src.Fix(ctx)
		if err != nil {
			cur.Store(source.Fix{Mode: 0})
			return
		}
		cur.Store(f)
	}
	poll()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			poll()
		}
	}
}
