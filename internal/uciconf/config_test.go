package uciconf_test

import (
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/uciconf"
)

func TestParseUciShowOverlaysDefaults(t *testing.T) {
	out := `geolocd.main=geolocd
geolocd.main.key='pk.abc123'
geolocd.main.poll_interval='30'
geolocd.main.listen='0.0.0.0:2947'
geolocd.main.runner='ubus'`
	cfg := uciconf.ParseUciShow(out)
	if cfg.Key != "pk.abc123" {
		t.Fatalf("key=%q", cfg.Key)
	}
	if cfg.PollInterval != 30*time.Second {
		t.Fatalf("poll=%v", cfg.PollInterval)
	}
	if cfg.Listen != "0.0.0.0:2947" {
		t.Fatalf("listen=%q", cfg.Listen)
	}
	if cfg.Runner != "ubus" {
		t.Fatalf("runner=%q", cfg.Runner)
	}
	// Untouched options keep defaults.
	if cfg.Bus != "cpu" || cfg.Radio != "LTE" {
		t.Fatalf("defaults lost: %+v", cfg)
	}
}

func TestParseUciShowEmptyKeepsDefaults(t *testing.T) {
	cfg := uciconf.ParseUciShow("")
	def := uciconf.Defaults()
	if cfg != def {
		t.Fatalf("empty input changed defaults: %+v vs %+v", cfg, def)
	}
}

func TestParseUciShowEmptyValuesKeepDefaults(t *testing.T) {
	// Empty option values must NOT clobber the defaults (the `if val != ""` guards).
	out := `geolocd.main.listen=''
geolocd.main.bus=''
geolocd.main.radio=''
geolocd.main.runner=''
nodotline=value`
	cfg := uciconf.ParseUciShow(out)
	def := uciconf.Defaults()
	if cfg.Listen != def.Listen || cfg.Bus != def.Bus || cfg.Radio != def.Radio || cfg.Runner != def.Runner {
		t.Fatalf("empty values clobbered defaults: %+v", cfg)
	}
}

func TestParseUciShowZeroPollKeepsDefault(t *testing.T) {
	cfg := uciconf.ParseUciShow(`geolocd.main.poll_interval='0'`)
	if cfg.PollInterval != 60*time.Second {
		t.Fatalf("zero poll should keep default, got %v", cfg.PollInterval)
	}
}

func TestParseUciShowIgnoresBadPollAndUnknown(t *testing.T) {
	out := `geolocd.main.poll_interval='notanumber'
geolocd.main.bogus='x'
unrelated line`
	cfg := uciconf.ParseUciShow(out)
	if cfg.PollInterval != 60*time.Second {
		t.Fatalf("bad poll should keep default, got %v", cfg.PollInterval)
	}
}
