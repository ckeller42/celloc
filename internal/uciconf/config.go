// Package uciconf loads geolocd configuration from OpenWrt uci. config.go is
// pure (parse `uci show` text -> Config with defaults); load.go does the I/O.
package uciconf

import (
	"strconv"
	"strings"
	"time"
)

// Config is the daemon's runtime configuration (uci section geolocd.main).
type Config struct {
	Key          string        // OpenCelliD API key (secret; never logged)
	PollInterval time.Duration // between AT polls
	Listen       string        // gpsd socket bind address
	Bus          string        // gl_modem bus (e.g. "cpu")
	Radio        string        // geolocation radio (v1: "LTE")
	Runner       string        // AT runner: "glmodem" | "ubus"
}

// Defaults returns the baseline config; ParseUciShow overlays any set options.
func Defaults() Config {
	return Config{
		PollInterval: 60 * time.Second,
		Listen:       ":2947",
		Bus:          "cpu",
		Radio:        "LTE",
		Runner:       "glmodem",
	}
}

// ParseUciShow parses the output of `uci -N show geolocd` (lines like
// `geolocd.main.key='pk.xxx'`) and overlays recognised options onto Defaults().
// Unknown options are ignored.
func ParseUciShow(out string) Config {
	cfg := Defaults()
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key, val := line[:eq], unquote(line[eq+1:])
		// key looks like geolocd.<section>.<option>; we only care about <option>.
		dot := strings.LastIndexByte(key, '.')
		if dot < 0 {
			continue
		}
		switch key[dot+1:] {
		case "key":
			cfg.Key = val
		case "poll_interval":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.PollInterval = time.Duration(n) * time.Second
			}
		case "listen":
			if val != "" {
				cfg.Listen = val
			}
		case "bus":
			if val != "" {
				cfg.Bus = val
			}
		case "radio":
			if val != "" {
				cfg.Radio = val
			}
		case "runner":
			if val != "" {
				cfg.Runner = val
			}
		}
	}
	return cfg
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
