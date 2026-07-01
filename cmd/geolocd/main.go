// Command geolocd is the celloc router daemon: it scans nearby WiFi APs and reads
// the modem's serving cell, resolves them together via the configured provider
// (Google by default), and serves the position over the gpsd protocol.
// Configuration (incl. the provider key) comes from uci (/etc/config/geolocd) so
// the key never appears in argv/ps.
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

	// Single position source: nearby WiFi APs plus the modem's serving cell,
	// blended by the configured provider (Google by default). OpenCelliD is no
	// longer used — the provider resolves cell + WiFi together.
	var cur atomic.Value
	cur.Store(source.Fix{Mode: 0})

	if !cfg.WifiEnable {
		return errNoSource
	}
	var resolver wifi.Resolver
	var provKey string
	switch cfg.WifiProvider {
	case "", "google":
		resolver = &google.Client{Key: cfg.GoogleKey, HTTP: &http.Client{Timeout: 15 * time.Second}}
		provKey = cfg.GoogleKey
	case "unwiredlabs":
		resolver = &unwiredlabs.Client{Token: cfg.Key, Endpoint: cfg.ULAEndpoint, HTTP: &http.Client{Timeout: 15 * time.Second}}
		provKey = cfg.Key
	default:
		log.Printf("geolocd: unknown wifi_provider %q", cfg.WifiProvider)
		return errNoSource
	}
	if provKey == "" {
		return errNoSource
	}

	staleWifi := 2 * cfg.WifiInterval
	if staleWifi < 2*time.Minute {
		staleWifi = 2 * time.Minute
	}
	src := wifi.New(wifiscan.NewScanner(strings.Fields(cfg.WifiIface)), resolver, cfg.WifiMinAPs, staleWifi)
	src.Cell = cell.NewServingCellReader(atrun.New(cfg.Runner, cfg.Bus))
	log.Printf("geolocd %s: source enabled (provider=%s iface=%q interval=%s min_aps=%d, cell-blended)",
		Version, cfg.WifiProvider, cfg.WifiIface, cfg.WifiInterval, cfg.WifiMinAPs)
	go pollLoop(ctx, src, cfg.WifiInterval, &cur)

	srv := &gpsd.Server{
		Provider: func() source.Fix { return cur.Load().(source.Fix) },
		Interval: *streamEvery,
		Release:  "celloc-" + Version,
	}
	log.Printf("geolocd %s: listening on %s", Version, cfg.Listen)
	return srv.ListenAndServe(ctx, cfg.Listen)
}

var errNoSource = errInvalidConfig("no source configured — enable wifi (uci geolocd.main.wifi_enable=1) and set a provider key (google_key, or key for wifi_provider=unwiredlabs)")

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
