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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	log.Printf("geoinflux %s: gpsd %s -> %s (bucket %s, min-interval %s)", Version, *gpsdAddr, *influxURL, *bucket, *minInterval)

	for ctx.Err() == nil {
		if err := stream(ctx, *gpsdAddr, w, *minInterval); err != nil && ctx.Err() == nil {
			log.Printf("geoinflux: %v; reconnecting in 10s", err)
			select {
			case <-ctx.Done():
			case <-time.After(10 * time.Second):
			}
		}
	}
	return nil
}

// stream connects once and uploads fixes until the connection or ctx ends.
func stream(ctx context.Context, addr string, w *influx.Writer, minInterval time.Duration) error {
	c, err := gpsd.Dial(ctx, addr)
	if err != nil {
		return err
	}
	defer func() { _ = c.Close() }()
	go func() { <-ctx.Done(); _ = c.Close() }() // unblock ReadTPV on shutdown

	if err := c.Watch(); err != nil {
		return err
	}
	var last time.Time
	for {
		tpv, err := c.ReadTPV()
		if err != nil {
			return err
		}
		f := gpsd.FixFromTPV(tpv)
		if !f.HasFix() || time.Since(last) < minInterval {
			continue
		}
		if err := w.Write(ctx, influx.FixLine(f)); err != nil {
			log.Printf("geoinflux: write failed: %v", err)
			continue
		}
		last = time.Now()
		log.Printf("geoinflux: wrote %.4f,%.4f eph=%.0fm (%s)", f.Lat, f.Lon, f.EPH, f.Radio)
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
