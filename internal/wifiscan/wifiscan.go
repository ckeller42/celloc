// Package wifiscan parses `iw dev <if> scan` output into access points and runs
// the scan behind an injected Exec. wifiscan.go is pure (text -> []AP); the IO
// scanner lives in scanner.go.
package wifiscan

import (
	"strconv"
	"strings"
)

// AP is one scanned access point. Signal is dBm (negative; closer to 0 = stronger).
type AP struct {
	BSSID  string
	Signal int
	SSID   string
}

// ParseScan parses `iw dev <if> scan` text into APs, preserving scan order. It
// lowercases BSSIDs, skips APs whose SSID ends in "_nomap" (the opt-out
// convention), and skips entries that never yielded a BSSID.
func ParseScan(out string) []AP {
	var aps []AP
	var cur AP
	have := false

	flush := func() {
		if have && cur.BSSID != "" && !strings.HasSuffix(cur.SSID, "_nomap") {
			aps = append(aps, cur)
		}
	}

	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "BSS "):
			flush()
			cur = AP{BSSID: parseBSSID(line)}
			have = true
		case strings.HasPrefix(trimmed, "signal:"):
			cur.Signal = parseSignal(trimmed)
		case strings.HasPrefix(trimmed, "SSID:"):
			cur.SSID = strings.TrimSpace(strings.TrimPrefix(trimmed, "SSID:"))
		}
	}
	flush()
	return aps
}

// parseBSSID extracts the MAC from `BSS aa:bb:..:ff(on wlan0) -- assoc`.
func parseBSSID(line string) string {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "BSS "))
	if i := strings.IndexByte(rest, '('); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSpace(rest)
	if i := strings.IndexByte(rest, ' '); i >= 0 {
		rest = rest[:i]
	}
	return strings.ToLower(rest)
}

// parseSignal reads `-85.00 dBm` (after the `signal:` prefix), truncating toward
// zero (int(-42.5) == -42).
func parseSignal(trimmed string) int {
	v := strings.TrimSpace(strings.TrimPrefix(trimmed, "signal:"))
	if i := strings.IndexByte(v, ' '); i >= 0 {
		v = v[:i]
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return int(f)
}
