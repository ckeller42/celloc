// Command geoinflux is the Pi-side uploader: it connects to the router's gpsd
// socket (celloc geolocd), reads position fixes, and writes them to InfluxDB.
// The InfluxDB token comes from the environment (INFLUXDB_TOKEN), never argv.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/influx"
	"github.com/ckeller42/celloc/internal/ratelog"
	"github.com/ckeller42/celloc/internal/source"
)

// Version is overridable via -ldflags "-X main.Version=v1.2.3".
var Version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatalf("geoinflux: %v", err)
	}
}

func run() error {
	gpsdAddr := flag.String("gpsd", env("GPSD_ADDR", "192.168.8.1:2947"), "router gpsd address")
	influxURL := flag.String("influx-url", env("INFLUX_URL", "http://localhost:8086"), "InfluxDB base URL")
	org := flag.String("org", env("INFLUX_ORG", "home"), "InfluxDB org")
	bucket := flag.String("bucket", env("INFLUX_BUCKET", "buspi"), "InfluxDB bucket")
	minInterval := flag.Duration("min-interval", envDur("UPLOAD_MIN_INTERVAL", 30*time.Second), "min time between writes")
	flag.Parse()

	token := os.Getenv("INFLUXDB_TOKEN")
	if token == "" {
		return errMsg("INFLUXDB_TOKEN not set")
	}

	w := &influx.Writer{
		URL: *influxURL, Org: *org, Bucket: *bucket, Token: token,
		HTTP: &http.Client{Timeout: 10 * time.Second},
	}
	u := &uploader{w: w, minInterval: *minInterval, now: time.Now}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("geoinflux %s: gpsd %s -> %s (bucket %s, min-interval %s)", Version, *gpsdAddr, *influxURL, *bucket, *minInterval)

	for ctx.Err() == nil {
		if err := stream(ctx, *gpsdAddr, u); err != nil && ctx.Err() == nil {
			log.Printf("geoinflux: %v; reconnecting in 10s", err)
			u.disconnected(ctx)
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
		}
	}
	return nil
}

// stream connects once and uploads fixes until the connection or ctx ends.
func stream(ctx context.Context, addr string, u *uploader) error {
	c, err := gpsd.Dial(ctx, addr)
	if err != nil {
		return err
	}
	c.ReadTimeout = 90 * time.Second // surface stalled connections -> reconnect
	defer func() { _ = c.Close() }()

	// Unblock ReadTPV on shutdown, but tie the goroutine to this attempt so it
	// can't leak across reconnects.
	stopped := make(chan struct{})
	defer close(stopped)
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-stopped:
		}
	}()

	if err := c.Watch(); err != nil {
		return err
	}
	for {
		tpv, err := c.ReadTPV()
		if err != nil {
			return err
		}
		u.onTPV(ctx, tpv)
	}
}

// lineWriter is the subset of *influx.Writer the uploader needs.
type lineWriter interface {
	Write(ctx context.Context, line string) error
}

// uploader turns received TPVs into InfluxDB writes: a geo point per fix and a
// geo_status heartbeat, each at most once per minInterval (attempts, not just
// successes, are debounced).
type uploader struct {
	w           lineWriter
	minInterval time.Duration
	now         func() time.Time

	lastFix, lastStatus time.Time
	lastConnected       bool
	fixState            int // log fix/no-fix transitions once
	logs                ratelog.Limiter
}

const (
	stateUnknown = iota
	stateFix
	stateNoFix
)

func (u *uploader) due(last, now time.Time) bool {
	return last.IsZero() || now.Sub(last) >= u.minInterval
}

func (u *uploader) onTPV(ctx context.Context, tpv gpsd.TPV) {
	f, err := gpsd.FixFromTPV(tpv)
	if err != nil {
		u.logs.Printf("geoinflux: %v (writing fix without its time)", err)
	}
	now := u.now()

	switch {
	case f.HasFix() && u.fixState != stateFix:
		log.Printf("geoinflux: fix acquired (%s, eph=%.0fm)", f.Source, f.EPH)
		u.fixState = stateFix
	case !f.HasFix() && u.fixState != stateNoFix:
		log.Printf("geoinflux: fix lost (router reports mode=%d)", f.Mode)
		u.fixState = stateNoFix
	}

	u.writeStatus(ctx, f, true, now)

	if !f.HasFix() || !u.due(u.lastFix, now) {
		return
	}
	u.lastFix = now
	if err := u.w.Write(ctx, influx.FixLine(f)); err != nil {
		log.Printf("geoinflux: write failed: %v", err)
		return
	}
	log.Printf("geoinflux: wrote %.4f,%.4f eph=%.0fm (%s)", f.Lat, f.Lon, f.EPH, f.Radio)
}

// disconnected records that the router's gpsd is unreachable.
func (u *uploader) disconnected(ctx context.Context) {
	u.fixState = stateUnknown
	u.writeStatus(ctx, source.Fix{}, false, u.now())
}

// writeStatus writes a geo_status point at most once per minInterval, except
// that a change of connection state is written immediately.
func (u *uploader) writeStatus(ctx context.Context, f source.Fix, connected bool, now time.Time) {
	if connected == u.lastConnected && !u.due(u.lastStatus, now) {
		return
	}
	u.lastStatus, u.lastConnected = now, connected
	if err := u.w.Write(ctx, influx.StatusLine(f, connected, now)); err != nil {
		log.Printf("geoinflux: status write failed: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envDur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

type errMsg string

func (e errMsg) Error() string { return string(e) }
