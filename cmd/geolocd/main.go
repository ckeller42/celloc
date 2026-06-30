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
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ckeller42/celloc/internal/atrun"
	"github.com/ckeller42/celloc/internal/google"
	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/opencellid"
	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/source/cell"
	"github.com/ckeller42/celloc/internal/source/wifi"
	"github.com/ckeller42/celloc/internal/uciconf"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
	"github.com/ckeller42/celloc/internal/wifiscan"
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

	// Per-source current fixes, refreshed by independent poll loops and read by
	// the gpsd server. WiFi (when enabled) outranks cell.
	var cellCur, wifiCur atomic.Value
	cellCur.Store(source.Fix{Mode: 0})
	wifiCur.Store(source.Fix{Mode: 0})
	go pollLoop(ctx, src, cfg.PollInterval, &cellCur)

	if cfg.WifiEnable {
		var resolver wifi.Resolver
		var provKey string
		switch cfg.WifiProvider {
		case "unwiredlabs":
			resolver = &unwiredlabs.Client{Token: cfg.Key, Endpoint: cfg.ULAEndpoint, HTTP: &http.Client{Timeout: 15 * time.Second}}
			provKey = cfg.Key
		default: // "google"
			resolver = &google.Client{Key: cfg.GoogleKey, HTTP: &http.Client{Timeout: 15 * time.Second}}
			provKey = cfg.GoogleKey
		}
		if provKey == "" {
			log.Printf("geolocd: wifi source disabled (provider %q has no key)", cfg.WifiProvider)
		} else {
			staleWifi := 2 * cfg.WifiInterval
			if staleWifi < 2*time.Minute {
				staleWifi = 2 * time.Minute
			}
			wsrc := wifi.New(wifiscan.NewScanner(strings.Fields(cfg.WifiIface)), resolver, cfg.WifiMinAPs, staleWifi)
			log.Printf("geolocd: wifi source enabled (provider=%s iface=%q interval=%s min_aps=%d)",
				cfg.WifiProvider, cfg.WifiIface, cfg.WifiInterval, cfg.WifiMinAPs)
			go pollLoop(ctx, wsrc, cfg.WifiInterval, &wifiCur)
		}
	}

	srv := &gpsd.Server{
		Provider: func() source.Fix {
			if w := wifiCur.Load().(source.Fix); w.HasFix() {
				return w
			}
			return cellCur.Load().(source.Fix)
		},
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
