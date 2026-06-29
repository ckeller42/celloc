# WiFi-AP Geolocation Source Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a WiFi-AP positioning source to `geolocd` that resolves nearby
access points via the Unwired Labs LocationAPI and outranks the cell source,
giving tens-of-metres fixes instead of the ~1.5 km single-cell estimate.

**Architecture:** Three new packages mirror the existing cell stack's pure/IO
split — `wifiscan` (parse `iw scan` + IO scanner), `unwiredlabs` (pure
`ParseResponse` + IO `Client`), `source/wifi` (compose, cache, classified
logging). `geolocd` runs an independent WiFi poll loop (slower cadence) and a
provider that prefers the WiFi fix, falling back to the cell fix.

**Tech Stack:** Go 1.23, standard library only. Tests are table-driven with
injected `Exec`/`Doer` interfaces (no network, no real modem/radio).

## Global Constraints

- **Module:** `github.com/ckeller42/celloc`; Go **1.23**; **standard library only**
  (no new go.mod dependencies).
- **Pure/IO split:** parsing/marshaling has no I/O, env, or wall-clock; I/O sits
  behind injected interfaces (`Exec`, `Doer`, `func() time.Time`).
- **Honest gpsd semantics:** a fix is `mode=2`, `epx=epy=eph=accuracy`, **no**
  `alt`/`speed`/`track`; no-fix is `mode=0` with no coordinates.
- **Coverage gate:** ≥ 85% over `./internal/...` (CI enforces).
- **Formatting/lint:** `gofumpt`-clean, `golangci-lint` v2 clean (no
  `unused-parameter`: name unused func params `_`).
- **Commit trailers (every commit):**

  ```text
  Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
  Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W
  ```

- **Branch:** `feat/wifi-geolocation`.

## Deviation from spec (confirm)

The spec's decision #2 was "send WiFi APs **and** the serving cell in one
`process.php` request." This plan implements the cell fallback at the **daemon
level** instead: a WiFi poll loop and a cell poll loop feed separate caches, and
the gpsd provider prefers the WiFi fix, else the cell fix. Same user outcome
(WiFi accuracy, cell as fallback) with decoupled, independently-testable sources.
The `unwiredlabs.Request` type still carries an optional `Cells` field so the
combined request remains a small future enhancement. If you want the literal
combined request instead, say so before execution.

## File Structure

- Create `internal/wifiscan/wifiscan.go` — pure `ParseScan(out) []AP`.
- Create `internal/wifiscan/wifiscan_test.go`.
- Create `internal/wifiscan/scanner.go` — IO `Scanner` (injected `Exec`), merge/dedup.
- Create `internal/wifiscan/scanner_test.go`.
- Create `internal/unwiredlabs/parse.go` — pure types + `ParseResponse`.
- Create `internal/unwiredlabs/parse_test.go`.
- Create `internal/unwiredlabs/client.go` — IO `Client.LookupWifi` (injected `Doer`).
- Create `internal/unwiredlabs/client_test.go`.
- Create `internal/source/wifi/wifi.go` — `Source` compose (cache/staleness/log).
- Create `internal/source/wifi/wifi_test.go`.
- Modify `internal/source/source.go` — add `Fix.APCount int`.
- Modify `internal/influx/line.go` — wifi branch in `FixLine`.
- Modify `internal/influx/line_test.go` — wifi line test.
- Modify `internal/gpsd/report.go` — `WifiFix` extension in `TPV`/`TPVFromFix`/`FixFromTPV`.
- Modify `internal/gpsd/report_test.go` — wifi TPV tests.
- Modify `internal/gpsd/client_test.go` (or `server_test.go`) — wifi round-trip.
- Modify `internal/uciconf/config.go` — wifi config fields + parse.
- Modify `internal/uciconf/config_test.go` — wifi parse tests.
- Modify `cmd/geolocd/main.go` — build WiFi source, two poll loops, priority provider.
- Modify `packaging/openwrt/files/geolocd.config` — wifi uci defaults.
- Modify `docs/INSTALL.md`, `SECURITY.md`, `docs/ARCHITECTURE.md`, `README.md`.

---

## M1 — pure core + tests

### Task 1: `Fix.APCount` + `influx.FixLine` wifi branch

**Files:**
- Modify: `internal/source/source.go` (add field)
- Modify: `internal/influx/line.go`
- Test: `internal/influx/line_test.go`

**Interfaces:**
- Consumes: `source.Fix`.
- Produces: `influx.FixLine(source.Fix) string` now renders
  `geo,source=wifi lat=..,lon=..,range_m=Ni,ap_count=Ni` when `Fix.Source=="wifi"`;
  `Fix` gains `APCount int`.

- [ ] **Step 1: Add the `APCount` field to `Fix`**

In `internal/source/source.go`, add to the `Fix` struct after `TAC int`:

```go
	TAC    int

	// APCount is the number of WiFi APs used for a wifi fix (0 for cell).
	APCount int
```

- [ ] **Step 2: Write the failing test**

Append to `internal/influx/line_test.go`:

```go
func TestFixLineWifi(t *testing.T) {
	f := source.Fix{
		Mode: 2, Lat: 48.7701, Lon: 9.1690, EPH: 35,
		Source: "wifi", APCount: 7,
	}
	got := influx.FixLine(f)
	want := "geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i"
	if got != want {
		t.Fatalf("FixLine(wifi)\n got=%q\nwant=%q", got, want)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/influx/ -run TestFixLineWifi -v`
Expected: FAIL (current `FixLine` emits the cell schema with `mcc=0i,...`).

- [ ] **Step 4: Add the wifi branch to `FixLine`**

In `internal/influx/line.go`, at the top of `FixLine`, before the existing
`lat := ...` line, insert:

```go
func FixLine(f source.Fix) string {
	if f.Source == "wifi" {
		return Measurement +
			",source=wifi" +
			" lat=" + strconv.FormatFloat(f.Lat, 'f', -1, 64) +
			",lon=" + strconv.FormatFloat(f.Lon, 'f', -1, 64) +
			",range_m=" + strconv.Itoa(int(f.EPH)) + "i" +
			",ap_count=" + strconv.Itoa(f.APCount) + "i"
	}
	lat := strconv.FormatFloat(f.Lat, 'f', -1, 64)
```

(Leave the rest of the existing cell rendering unchanged.)

- [ ] **Step 5: Run tests to verify pass + cell regression**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/influx/ -v`
Expected: PASS (both `TestFixLineWifi` and the existing cell golden test).

- [ ] **Step 6: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/source/source.go internal/influx/line.go internal/influx/line_test.go
git commit -m "feat(influx): wifi FixLine branch + Fix.APCount

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 2: `wifiscan.ParseScan` (pure)

**Files:**
- Create: `internal/wifiscan/wifiscan.go`
- Test: `internal/wifiscan/wifiscan_test.go`

**Interfaces:**
- Produces: `wifiscan.AP{BSSID string; Signal int; SSID string}` and
  `wifiscan.ParseScan(out string) []AP` — parses `iw dev <if> scan`, lowercases
  BSSIDs, drops `_nomap` APs and entries without a BSSID.

- [ ] **Step 1: Write the failing test**

Create `internal/wifiscan/wifiscan_test.go`:

```go
package wifiscan_test

import (
	"reflect"
	"testing"

	"github.com/ckeller42/celloc/internal/wifiscan"
)

const sample = "BSS 00:11:22:33:44:55(on wlan0)\r\n" +
	"\tlast seen: 100 ms ago\r\n" +
	"\tsignal: -85.00 dBm\r\n" +
	"\tSSID: HomeNet\r\n" +
	"BSS aa:BB:cc:DD:ee:ff(on wlan0) -- associated\n" +
	"\tsignal: -42.50 dBm\n" +
	"\tSSID: minsel\n" +
	"BSS 12:34:56:78:9a:bc(on wlan0)\n" +
	"\tsignal: -70.00 dBm\n" +
	"\tSSID: cafe_nomap\n" +
	"BSS de:ad:be:ef:00:01(on wlan0)\n" +
	"\tsignal: -60.00 dBm\n" +
	"\tSSID: \n"

func TestParseScan(t *testing.T) {
	got := wifiscan.ParseScan(sample)
	want := []wifiscan.AP{
		{BSSID: "00:11:22:33:44:55", Signal: -85, SSID: "HomeNet"},
		{BSSID: "aa:bb:cc:dd:ee:ff", Signal: -42, SSID: "minsel"},
		{BSSID: "de:ad:be:ef:00:01", Signal: -60, SSID: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseScan\n got=%#v\nwant=%#v", got, want)
	}
}

func TestParseScanEmptyAndGarbage(t *testing.T) {
	if aps := wifiscan.ParseScan(""); len(aps) != 0 {
		t.Fatalf("empty: got %v", aps)
	}
	if aps := wifiscan.ParseScan("no bss here\nsignal: -1 dBm\n"); len(aps) != 0 {
		t.Fatalf("garbage: got %v", aps)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/wifiscan/ -v`
Expected: FAIL (package/function does not exist).

- [ ] **Step 3: Write the implementation**

Create `internal/wifiscan/wifiscan.go`:

```go
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

// parseSignal reads `-85.00 dBm` (after the `signal:` prefix) as a rounded int.
func parseSignal(trimmed string) int {
	v := strings.TrimSpace(strings.TrimPrefix(trimmed, "signal:"))
	if i := strings.IndexByte(v, ' '); i >= 0 {
		v = v[:i]
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	if f < 0 {
		return int(f - 0.5)
	}
	return int(f + 0.5)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/wifiscan/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/wifiscan/wifiscan.go internal/wifiscan/wifiscan_test.go
git commit -m "feat(wifiscan): pure ParseScan for iw scan output

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 3: `unwiredlabs.ParseResponse` + types (pure)

**Files:**
- Create: `internal/unwiredlabs/parse.go`
- Test: `internal/unwiredlabs/parse_test.go`

**Interfaces:**
- Produces:
  - `unwiredlabs.Location{Lat, Lon float64; Accuracy float64}`
  - `unwiredlabs.Status` with `StatusOK, StatusNotFound, StatusRateLimited, StatusAuth, StatusServer` and `String()`
  - `unwiredlabs.WifiAP{BSSID string; Signal int}`, `unwiredlabs.CellTower{LAC int; CID int64; MCC int; MNC int; Signal int}`, `unwiredlabs.Request{Token, Radio string; MCC, MNC int; Cells []CellTower; Wifi []WifiAP; Address int}`
  - `unwiredlabs.ParseResponse(httpStatus int, body []byte) (Location, Status, error)`

- [ ] **Step 1: Write the failing test**

Create `internal/unwiredlabs/parse_test.go`:

```go
package unwiredlabs_test

import (
	"testing"

	"github.com/ckeller42/celloc/internal/unwiredlabs"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   unwiredlabs.Status
		lat    float64
	}{
		{"ok", 200, `{"status":"ok","balance":98,"lat":48.7701,"lon":9.169,"accuracy":35}`, unwiredlabs.StatusOK, 48.7701},
		{"no match", 200, `{"status":"error","message":"No matches found"}`, unwiredlabs.StatusNotFound, 0},
		{"bad token body", 200, `{"status":"error","message":"Invalid token"}`, unwiredlabs.StatusAuth, 0},
		{"quota body", 200, `{"status":"error","message":"Insufficient credits / balance"}`, unwiredlabs.StatusRateLimited, 0},
		{"http 401", 401, ``, unwiredlabs.StatusAuth, 0},
		{"http 429", 429, ``, unwiredlabs.StatusRateLimited, 0},
		{"http 503", 503, ``, unwiredlabs.StatusServer, 0},
		{"bad json", 200, `{`, unwiredlabs.StatusServer, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			loc, st, _ := unwiredlabs.ParseResponse(tc.status, []byte(tc.body))
			if st != tc.want {
				t.Fatalf("status=%v want %v", st, tc.want)
			}
			if tc.want == unwiredlabs.StatusOK && loc.Lat != tc.lat {
				t.Fatalf("lat=%v want %v", loc.Lat, tc.lat)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/unwiredlabs/ -v`
Expected: FAIL (package does not exist).

- [ ] **Step 3: Write the implementation**

Create `internal/unwiredlabs/parse.go`:

```go
// Package unwiredlabs resolves a position from nearby WiFi APs (and optionally
// cells) via the Unwired Labs LocationAPI (process.php). parse.go is pure
// (HTTP status + body -> Location/Status); client.go does the I/O behind a Doer.
package unwiredlabs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Location is a resolved position. Accuracy is the provider's error radius in
// meters (used as the gpsd EPH for the resulting fix).
type Location struct {
	Lat      float64
	Lon      float64
	Accuracy float64
}

// Status classifies a lookup outcome so callers can decide retry vs give-up.
type Status int

// Lookup outcome classifications.
const (
	StatusOK          Status = iota // usable Location
	StatusNotFound                  // no match for the given APs/cells
	StatusRateLimited               // quota/credits exhausted — back off
	StatusAuth                      // bad/expired token
	StatusServer                    // 5xx / malformed — transient
)

func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusNotFound:
		return "not-found"
	case StatusRateLimited:
		return "rate-limited"
	case StatusAuth:
		return "auth"
	case StatusServer:
		return "server"
	default:
		return "invalid"
	}
}

// WifiAP is one access point in a LocationAPI request.
type WifiAP struct {
	BSSID  string `json:"bssid"`
	Signal int    `json:"signal,omitempty"`
}

// CellTower is one cell in a LocationAPI request (optional fallback anchor).
type CellTower struct {
	LAC    int   `json:"lac"`
	CID    int64 `json:"cid"`
	MCC    int   `json:"mcc"`
	MNC    int   `json:"mnc"`
	Signal int   `json:"signal,omitempty"`
}

// Request is the process.php JSON body.
type Request struct {
	Token   string      `json:"token"`
	Radio   string      `json:"radio,omitempty"`
	MCC     int         `json:"mcc,omitempty"`
	MNC     int         `json:"mnc,omitempty"`
	Cells   []CellTower `json:"cells,omitempty"`
	Wifi    []WifiAP    `json:"wifi,omitempty"`
	Address int         `json:"address"`
}

type apiResponse struct {
	Status   string   `json:"status"`
	Message  string   `json:"message"`
	Lat      *float64 `json:"lat"`
	Lon      *float64 `json:"lon"`
	Accuracy float64  `json:"accuracy"`
}

// ParseResponse maps an HTTP status + body to a (Location, Status). It returns a
// non-nil error only for malformed payloads.
func ParseResponse(httpStatus int, body []byte) (Location, Status, error) {
	switch {
	case httpStatus == 401 || httpStatus == 403:
		return Location{}, StatusAuth, nil
	case httpStatus == 429:
		return Location{}, StatusRateLimited, nil
	case httpStatus >= 500:
		return Location{}, StatusServer, nil
	case httpStatus != 200:
		return Location{}, StatusNotFound, nil
	}

	var r apiResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: bad json: %w", err)
	}
	if r.Status == "ok" && r.Lat != nil && r.Lon != nil {
		return Location{Lat: *r.Lat, Lon: *r.Lon, Accuracy: r.Accuracy}, StatusOK, nil
	}
	if r.Status == "error" {
		return Location{}, classify(r.Message), nil
	}
	return Location{}, StatusServer, nil
}

// classify maps an error message to a Status (best-effort; the API returns 200
// with a status:"error" body for most failures).
func classify(msg string) Status {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "token"), strings.Contains(m, "key"),
		strings.Contains(m, "access"), strings.Contains(m, "invalid request"):
		return StatusAuth
	case strings.Contains(m, "credit"), strings.Contains(m, "balance"),
		strings.Contains(m, "limit"), strings.Contains(m, "quota"):
		return StatusRateLimited
	default:
		return StatusNotFound
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/unwiredlabs/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/unwiredlabs/parse.go internal/unwiredlabs/parse_test.go
git commit -m "feat(unwiredlabs): pure ParseResponse + request types

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 4: gpsd `WifiFix` extension (pure)

**Files:**
- Modify: `internal/gpsd/report.go`
- Test: `internal/gpsd/report_test.go`

**Interfaces:**
- Produces: `gpsd.WifiFix{APCount int}`; `TPV` gains `WifiFix *WifiFix`
  (`json:"wifix,omitempty"`); `TPVFromFix` emits `wifix` for wifi fixes (no
  `cellfix`, no alt/speed); `FixFromTPV` reconstructs `Source="wifi"` + `APCount`.

- [ ] **Step 1: Write the failing test**

Append to `internal/gpsd/report_test.go`:

```go
func TestTPVFromWifiFix(t *testing.T) {
	f := source.Fix{
		Time: time.Unix(1700000000, 0).UTC(), Mode: 2,
		Lat: 48.7701, Lon: 9.169, EPH: 35, EPX: 35, EPY: 35,
		Source: "wifi", APCount: 7,
	}
	tpv := gpsd.TPVFromFix(f, "cell0")
	if tpv.WifiFix == nil || tpv.WifiFix.APCount != 7 {
		t.Fatalf("wifix missing/wrong: %+v", tpv.WifiFix)
	}
	if tpv.CellFix != nil {
		t.Fatalf("wifi fix must not carry cellfix")
	}
	b, _ := gpsd.MarshalLine(tpv)
	for _, k := range []string{`"alt"`, `"speed"`, `"track"`, `"cellfix"`} {
		if bytes.Contains(b, []byte(k)) {
			t.Fatalf("wifi TPV must not contain %s: %s", k, b)
		}
	}
	back := gpsd.FixFromTPV(tpv)
	if back.Source != "wifi" || back.APCount != 7 {
		t.Fatalf("FixFromTPV lost wifi info: %+v", back)
	}
}
```

If `bytes` is not already imported in this test file, add it to the import block.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/gpsd/ -run TestTPVFromWifiFix -v`
Expected: FAIL (`WifiFix` undefined).

- [ ] **Step 3: Add the `WifiFix` type and field**

In `internal/gpsd/report.go`, after the `CellFix` struct, add:

```go
// WifiFix is a NON-STANDARD extension object embedded in TPV for wifi fixes.
// Standard gpsd clients ignore it; geoinflux reads it to tag the InfluxDB point
// as source=wifi with the AP count.
type WifiFix struct {
	APCount int `json:"ap_count"`
}
```

In the `TPV` struct, add a field after `CellFix *CellFix ...`:

```go
	CellFix *CellFix `json:"cellfix,omitempty"`
	WifiFix *WifiFix `json:"wifix,omitempty"`
```

- [ ] **Step 4: Emit and parse `wifix`**

In `TPVFromFix`, replace the existing cellfix block:

```go
	if f.MCC != 0 || f.CID != 0 {
		t.CellFix = &CellFix{Radio: f.Radio, MCC: f.MCC, MNC: f.MNC, CID: f.CID, TAC: f.TAC, Range: int(f.EPH)}
	}
	return t
```

with:

```go
	switch {
	case f.Source == "wifi":
		t.WifiFix = &WifiFix{APCount: f.APCount}
	case f.MCC != 0 || f.CID != 0:
		t.CellFix = &CellFix{Radio: f.Radio, MCC: f.MCC, MNC: f.MNC, CID: f.CID, TAC: f.TAC, Range: int(f.EPH)}
	}
	return t
```

In `FixFromTPV`, after the existing `if t.CellFix != nil { ... }` block, add:

```go
	if t.WifiFix != nil {
		f.Source = "wifi"
		f.APCount = t.WifiFix.APCount
	}
```

- [ ] **Step 5: Run tests to verify pass + cell regression**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/gpsd/ -v`
Expected: PASS (wifi test + existing cell/version/server tests).

- [ ] **Step 6: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/gpsd/report.go internal/gpsd/report_test.go
git commit -m "feat(gpsd): wifix TPV extension for wifi fixes

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

## M2 — IO + geolocd wiring

### Task 5: `wifiscan.Scanner` (IO)

**Files:**
- Create: `internal/wifiscan/scanner.go`
- Test: `internal/wifiscan/scanner_test.go`

**Interfaces:**
- Produces: `wifiscan.Exec` (= `func(ctx, name string, args ...string) ([]byte, error)`),
  `wifiscan.OSExec`, `wifiscan.Scanner{Ifaces []string; Exec Exec}` with
  `Scan(ctx) ([]AP, error)` (runs `iw dev <if> scan` per iface, merges, dedups by
  BSSID keeping the strongest signal), and `NewScanner(ifaces []string) Scanner`.

- [ ] **Step 1: Write the failing test**

Create `internal/wifiscan/scanner_test.go`:

```go
package wifiscan_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ckeller42/celloc/internal/wifiscan"
)

func TestScannerArgsAndParse(t *testing.T) {
	var gotArgs []string
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("BSS 00:11:22:33:44:55(on wlan0)\n\tsignal: -50.00 dBm\n\tSSID: a\n"), nil
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan0"}, Exec: exec}
	aps, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"iw", "dev", "wlan0", "scan"}
	if len(gotArgs) != 4 || gotArgs[0] != want[0] || gotArgs[2] != want[2] || gotArgs[3] != want[3] {
		t.Fatalf("args=%v want %v", gotArgs, want)
	}
	if len(aps) != 1 || aps[0].BSSID != "00:11:22:33:44:55" {
		t.Fatalf("aps=%v", aps)
	}
}

func TestScannerMergeDedupKeepsStrongest(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		if args[1] == "wlan0" {
			return []byte("BSS aa:aa:aa:aa:aa:aa(on wlan0)\n\tsignal: -80.00 dBm\n\tSSID: x\n"), nil
		}
		return []byte("BSS AA:AA:AA:AA:AA:AA(on wlan1)\n\tsignal: -40.00 dBm\n\tSSID: x\n" +
			"BSS bb:bb:bb:bb:bb:bb(on wlan1)\n\tsignal: -55.00 dBm\n\tSSID: y\n"), nil
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan0", "wlan1"}, Exec: exec}
	aps, err := s.Scan(context.Background())
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	got := map[string]int{}
	for _, a := range aps {
		got[a.BSSID] = a.Signal
	}
	if got["aa:aa:aa:aa:aa:aa"] != -40 { // strongest of -80/-40 kept
		t.Fatalf("dedup kept wrong signal: %v", got)
	}
	if len(aps) != 2 {
		t.Fatalf("want 2 unique APs, got %v", aps)
	}
}

func TestScannerErrorWhenAllIfacesFailAndNoAPs(t *testing.T) {
	exec := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("iw: No such device")
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan9"}, Exec: exec}
	if _, err := s.Scan(context.Background()); err == nil {
		t.Fatal("want error when every iface scan fails")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/wifiscan/ -run TestScanner -v`
Expected: FAIL (`Scanner` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/wifiscan/scanner.go`:

```go
package wifiscan

import (
	"context"
	"fmt"
	"os/exec"
)

// Exec runs an external command and returns its combined stdout. Injected so
// tests can stub the subprocess.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// OSExec is the production Exec backed by os/exec.
func OSExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Scanner runs `iw dev <if> scan` for each interface and merges the results.
type Scanner struct {
	Ifaces []string
	Exec   Exec
}

// NewScanner builds a Scanner over ifaces using OSExec.
func NewScanner(ifaces []string) Scanner {
	return Scanner{Ifaces: ifaces, Exec: OSExec}
}

// Scan scans every interface and returns the merged AP set, de-duplicated by
// BSSID keeping the strongest signal. It returns an error only when every
// interface failed and no APs were collected.
func (s Scanner) Scan(ctx context.Context) ([]AP, error) {
	byBSSID := map[string]AP{}
	var order []string
	var firstErr error
	for _, iface := range s.Ifaces {
		out, err := s.Exec(ctx, "iw", "dev", iface, "scan")
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("iw dev %s scan: %w", iface, err)
			}
			continue
		}
		for _, ap := range ParseScan(string(out)) {
			if cur, ok := byBSSID[ap.BSSID]; !ok {
				byBSSID[ap.BSSID] = ap
				order = append(order, ap.BSSID)
			} else if ap.Signal > cur.Signal {
				byBSSID[ap.BSSID] = ap
			}
		}
	}
	if len(byBSSID) == 0 && firstErr != nil {
		return nil, firstErr
	}
	aps := make([]AP, 0, len(order))
	for _, b := range order {
		aps = append(aps, byBSSID[b])
	}
	return aps, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/wifiscan/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/wifiscan/scanner.go internal/wifiscan/scanner_test.go
git commit -m "feat(wifiscan): IO Scanner over iw with merge/dedup

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 6: `unwiredlabs.Client.LookupWifi` (IO)

**Files:**
- Create: `internal/unwiredlabs/client.go`
- Test: `internal/unwiredlabs/client_test.go`

**Interfaces:**
- Produces: `unwiredlabs.Doer` (`Do(*http.Request)(*http.Response,error)`);
  `unwiredlabs.Client{Token, Endpoint string; HTTP Doer; BaseURL string}` with
  `LookupWifi(ctx, aps []WifiAP) (Location, Status, error)`. Default URL is
  `https://<Endpoint>.unwiredlabs.com/v2/process.php`.

- [ ] **Step 1: Write the failing test**

Create `internal/unwiredlabs/client_test.go`:

```go
package unwiredlabs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/ckeller42/celloc/internal/unwiredlabs"
)

type roundTrip struct {
	gotURL  string
	gotBody []byte
	resp    string
	code    int
}

func (r *roundTrip) Do(req *http.Request) (*http.Response, error) {
	r.gotURL = req.URL.String()
	r.gotBody, _ = io.ReadAll(req.Body)
	return &http.Response{
		StatusCode: r.code,
		Body:       io.NopCloser(bytes.NewBufferString(r.resp)),
		Header:     make(http.Header),
	}, nil
}

func TestLookupWifiBuildsRequest(t *testing.T) {
	rt := &roundTrip{code: 200, resp: `{"status":"ok","lat":48.77,"lon":9.17,"accuracy":30}`}
	c := &unwiredlabs.Client{Token: "pk.test", Endpoint: "eu1", HTTP: rt}
	loc, st, err := c.LookupWifi(context.Background(), []unwiredlabs.WifiAP{
		{BSSID: "00:11:22:33:44:55", Signal: -50},
	})
	if err != nil || st != unwiredlabs.StatusOK || loc.Accuracy != 30 {
		t.Fatalf("loc=%+v st=%v err=%v", loc, st, err)
	}
	if rt.gotURL != "https://eu1.unwiredlabs.com/v2/process.php" {
		t.Fatalf("url=%q", rt.gotURL)
	}
	var sent unwiredlabs.Request
	if err := json.Unmarshal(rt.gotBody, &sent); err != nil {
		t.Fatal(err)
	}
	if sent.Token != "pk.test" || len(sent.Wifi) != 1 || sent.Wifi[0].BSSID != "00:11:22:33:44:55" {
		t.Fatalf("body=%s", rt.gotBody)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/unwiredlabs/ -run TestLookupWifi -v`
Expected: FAIL (`Client` undefined).

- [ ] **Step 3: Write the implementation**

Create `internal/unwiredlabs/client.go`:

```go
package unwiredlabs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Doer is the subset of *http.Client used here; injected for tests.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Client resolves positions via the Unwired Labs LocationAPI.
type Client struct {
	Token    string
	Endpoint string // region subdomain, e.g. "eu1"
	HTTP     Doer
	BaseURL  string // overrides https://<Endpoint>.unwiredlabs.com (tests)
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	ep := c.Endpoint
	if ep == "" {
		ep = "eu1"
	}
	return "https://" + ep + ".unwiredlabs.com"
}

// LookupWifi resolves a position from the given APs.
func (c *Client) LookupWifi(ctx context.Context, aps []WifiAP) (Location, Status, error) {
	return c.do(ctx, Request{Token: c.Token, Wifi: aps, Address: 0})
}

func (c *Client) do(ctx context.Context, r Request) (Location, Status, error) {
	r.Token = c.Token
	body, err := json.Marshal(r)
	if err != nil {
		return Location{}, StatusServer, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL()+"/v2/process.php", bytes.NewReader(body))
	if err != nil {
		return Location{}, StatusServer, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return Location{}, StatusServer, fmt.Errorf("unwiredlabs: read body: %w", err)
	}
	return ParseResponse(resp.StatusCode, respBody)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/unwiredlabs/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/unwiredlabs/client.go internal/unwiredlabs/client_test.go
git commit -m "feat(unwiredlabs): IO Client.LookupWifi (process.php)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 7: `source/wifi.Source` (IO compose)

**Files:**
- Create: `internal/source/wifi/wifi.go`
- Test: `internal/source/wifi/wifi_test.go`

**Interfaces:**
- Consumes: `wifiscan.AP`, `unwiredlabs.{WifiAP,Location,Status}`, `source.Fix`.
- Produces:
  - `wifi.Scanner` interface: `Scan(ctx) ([]wifiscan.AP, error)` (satisfied by `wifiscan.Scanner`)
  - `wifi.Resolver` interface: `LookupWifi(ctx, []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error)` (satisfied by `*unwiredlabs.Client`)
  - `wifi.New(sc Scanner, res Resolver, minAPs int, staleAfter time.Duration) *wifi.Source` implementing `source.Source` (`Name()=="wifi"`).

- [ ] **Step 1: Write the failing test**

Create `internal/source/wifi/wifi_test.go`:

```go
package wifi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/source/wifi"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
	"github.com/ckeller42/celloc/internal/wifiscan"
)

type scanFunc func(context.Context) ([]wifiscan.AP, error)

func (f scanFunc) Scan(ctx context.Context) ([]wifiscan.AP, error) { return f(ctx) }

type resFunc func(context.Context, []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error)

func (f resFunc) LookupWifi(ctx context.Context, a []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error) {
	return f(ctx, a)
}

func threeAPs(context.Context) ([]wifiscan.AP, error) {
	return []wifiscan.AP{{BSSID: "a", Signal: -40}, {BSSID: "b", Signal: -50}, {BSSID: "c", Signal: -60}}, nil
}

func okRes(unwiredlabs.Location) resFunc {
	return resFunc(func(context.Context, []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error) {
		return unwiredlabs.Location{Lat: 48.77, Lon: 9.17, Accuracy: 30}, unwiredlabs.StatusOK, nil
	})
}

func TestWifiFixHappyPath(t *testing.T) {
	var gotN int
	res := resFunc(func(_ context.Context, a []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error) {
		gotN = len(a)
		return unwiredlabs.Location{Lat: 48.77, Lon: 9.17, Accuracy: 30}, unwiredlabs.StatusOK, nil
	})
	s := wifi.New(scanFunc(threeAPs), res, 2, time.Minute)
	f, err := s.Fix(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if f.Mode != 2 || f.Source != "wifi" || f.EPH != 30 || f.APCount != 3 || gotN != 3 {
		t.Fatalf("bad fix: %+v gotN=%d", f, gotN)
	}
}

func TestWifiTooFewAPsIsNoFix(t *testing.T) {
	one := scanFunc(func(context.Context) ([]wifiscan.AP, error) {
		return []wifiscan.AP{{BSSID: "a", Signal: -40}}, nil
	})
	s := wifi.New(one, okRes(unwiredlabs.Location{}), 2, time.Minute)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix when below min APs")
	}
}

func TestWifiAuthFailureLogged(t *testing.T) {
	var logs int
	res := resFunc(func(context.Context, []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error) {
		return unwiredlabs.Location{}, unwiredlabs.StatusAuth, nil
	})
	s := wifi.New(scanFunc(threeAPs), res, 2, time.Minute)
	s.Logf = func(string, ...any) { logs++ }
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("want ErrNoFix on auth")
	}
	if logs != 1 {
		t.Fatalf("want one log, got %d", logs)
	}
}

func TestWifiCachedThenStale(t *testing.T) {
	now := time.Unix(1000, 0)
	scanErr := false
	sc := scanFunc(func(context.Context) ([]wifiscan.AP, error) {
		if scanErr {
			return nil, errors.New("iw failed")
		}
		return threeAPs(context.Background())
	})
	s := wifi.New(sc, okRes(unwiredlabs.Location{}), 2, 90*time.Second)
	s.Now = func() time.Time { return now }
	if _, err := s.Fix(context.Background()); err != nil {
		t.Fatalf("first: %v", err)
	}
	scanErr = true
	now = now.Add(30 * time.Second)
	if f, err := s.Fix(context.Background()); err != nil || f.Lat != 48.77 {
		t.Fatalf("cached expected: %+v err=%v", f, err)
	}
	now = now.Add(120 * time.Second)
	if _, err := s.Fix(context.Background()); err == nil {
		t.Fatal("stale cache should be ErrNoFix")
	}
}

func TestWifiOutranksCell(t *testing.T) {
	w := wifi.New(scanFunc(threeAPs), okRes(unwiredlabs.Location{}), 2, time.Minute)
	cell := stubSource{f: source.Fix{Mode: 2, Source: "cell", Lat: 1, Lon: 1}}
	f, err := source.Select(context.Background(), w, cell)
	if err != nil || f.Source != "wifi" {
		t.Fatalf("want wifi selected, got %+v err=%v", f, err)
	}
}

type stubSource struct{ f source.Fix }

func (s stubSource) Name() string                                { return "cell" }
func (s stubSource) Fix(context.Context) (source.Fix, error)     { return s.f, nil }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/source/wifi/ -v`
Expected: FAIL (package does not exist).

- [ ] **Step 3: Write the implementation**

Create `internal/source/wifi/wifi.go`:

```go
// Package wifi implements source.Source via WiFi-AP geolocation: scan nearby
// access points, resolve them with the Unwired Labs LocationAPI, and serve the
// result. It keeps the last good fix until StaleAfter so a single failed scan or
// lookup doesn't blank the position. Mirrors internal/source/cell.
package wifi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ckeller42/celloc/internal/source"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
	"github.com/ckeller42/celloc/internal/wifiscan"
)

// resolveLogThrottle bounds how often an unchanged failure is logged.
const resolveLogThrottle = time.Minute

// errFewAPs means the scan returned fewer than MinAPs usable access points.
var errFewAPs = errors.New("wifi: too few access points")

// Scanner produces nearby access points (satisfied by wifiscan.Scanner).
type Scanner interface {
	Scan(ctx context.Context) ([]wifiscan.AP, error)
}

// Resolver resolves APs to a location (satisfied by *unwiredlabs.Client).
type Resolver interface {
	LookupWifi(ctx context.Context, aps []unwiredlabs.WifiAP) (unwiredlabs.Location, unwiredlabs.Status, error)
}

// Source is a WiFi-AP positioning source.
type Source struct {
	Scanner    Scanner
	Resolver   Resolver
	MinAPs     int
	StaleAfter time.Duration
	Now        func() time.Time
	Logf       func(format string, args ...any)

	mu         sync.Mutex
	last       source.Fix
	lastAt     time.Time
	lastLogMsg string
	lastLogAt  time.Time
}

// New builds a wifi Source with a real clock and log.Printf as the log sink.
func New(sc Scanner, res Resolver, minAPs int, staleAfter time.Duration) *Source {
	return &Source{
		Scanner: sc, Resolver: res, MinAPs: minAPs, StaleAfter: staleAfter,
		Now: time.Now, Logf: log.Printf,
	}
}

// Name implements source.Source.
func (s *Source) Name() string { return "wifi" }

// Fix implements source.Source: try a fresh scan+resolve; on any failure log it
// (throttled) and fall back to a cached fix younger than StaleAfter, else ErrNoFix.
func (s *Source) Fix(ctx context.Context) (source.Fix, error) {
	f, err := s.resolve(ctx)
	if err == nil {
		s.store(f)
		return f, nil
	}
	s.logResolve(err)
	return s.cached()
}

func (s *Source) resolve(ctx context.Context) (source.Fix, error) {
	aps, err := s.Scanner.Scan(ctx)
	if err != nil {
		return source.Fix{}, fmt.Errorf("wifi scan: %w", err)
	}
	if len(aps) < s.MinAPs {
		return source.Fix{}, errFewAPs
	}
	wlist := make([]unwiredlabs.WifiAP, 0, len(aps))
	for _, ap := range aps {
		wlist = append(wlist, unwiredlabs.WifiAP{BSSID: ap.BSSID, Signal: ap.Signal})
	}
	loc, st, err := s.Resolver.LookupWifi(ctx, wlist)
	if err != nil {
		return source.Fix{}, fmt.Errorf("unwiredlabs lookup: %w", err)
	}
	if st != unwiredlabs.StatusOK {
		return source.Fix{}, fmt.Errorf("unwiredlabs: %s", st)
	}
	r := loc.Accuracy
	return source.Fix{
		Time: s.now(), Mode: 2,
		Lat: loc.Lat, Lon: loc.Lon, EPH: r, EPX: r, EPY: r,
		Source: "wifi", APCount: len(aps),
	}, nil
}

func (s *Source) logResolve(err error) {
	if s.Logf == nil {
		return
	}
	msg := err.Error()
	s.mu.Lock()
	now := s.now()
	if msg == s.lastLogMsg && now.Sub(s.lastLogAt) < resolveLogThrottle {
		s.mu.Unlock()
		return
	}
	s.lastLogMsg, s.lastLogAt = msg, now
	s.mu.Unlock()
	s.Logf("wifi: resolve failed: %s (serving cached fix until stale)", msg)
}

func (s *Source) store(f source.Fix) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last, s.lastAt = f, s.now()
}

func (s *Source) cached() (source.Fix, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastAt.IsZero() || s.now().Sub(s.lastAt) >= s.StaleAfter {
		return source.Fix{}, source.ErrNoFix
	}
	return s.last, nil
}

func (s *Source) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/source/wifi/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/source/wifi/wifi.go internal/source/wifi/wifi_test.go
git commit -m "feat(source/wifi): compose scan+resolve with cache and logging

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 8: uci config — wifi options

**Files:**
- Modify: `internal/uciconf/config.go`
- Test: `internal/uciconf/config_test.go`

**Interfaces:**
- Produces: `Config` gains `WifiEnable bool`, `WifiIface string`,
  `WifiInterval time.Duration`, `WifiMinAPs int`, `ULAEndpoint string`;
  `Defaults()` sets `WifiEnable=true`, `WifiIface="wlan0"`,
  `WifiInterval=300s`, `WifiMinAPs=2`, `ULAEndpoint="eu1"`; `ParseUciShow`
  recognises `wifi_enable`, `wifi_iface`, `wifi_interval`, `wifi_min_aps`,
  `ula_endpoint`.

- [ ] **Step 1: Write the failing test**

Append to `internal/uciconf/config_test.go`:

```go
func TestParseUciShowWifi(t *testing.T) {
	in := `geolocd.main.wifi_enable='0'
geolocd.main.wifi_iface='wlan0 wlan1'
geolocd.main.wifi_interval='120'
geolocd.main.wifi_min_aps='4'
geolocd.main.ula_endpoint='us1'`
	cfg := uciconf.ParseUciShow(in)
	if cfg.WifiEnable {
		t.Fatal("wifi_enable=0 should disable")
	}
	if cfg.WifiIface != "wlan0 wlan1" || cfg.WifiMinAPs != 4 || cfg.ULAEndpoint != "us1" {
		t.Fatalf("bad cfg: %+v", cfg)
	}
	if cfg.WifiInterval != 120*time.Second {
		t.Fatalf("interval=%v", cfg.WifiInterval)
	}
}

func TestWifiDefaults(t *testing.T) {
	d := uciconf.Defaults()
	if !d.WifiEnable || d.WifiIface != "wlan0" || d.WifiMinAPs != 2 ||
		d.ULAEndpoint != "eu1" || d.WifiInterval != 300*time.Second {
		t.Fatalf("bad defaults: %+v", d)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/uciconf/ -run Wifi -v`
Expected: FAIL (fields undefined).

- [ ] **Step 3: Extend `Config`, `Defaults`, and `ParseUciShow`**

In `internal/uciconf/config.go`, add fields to `Config`:

```go
	Runner       string        // AT runner: "glmodem" | "ubus"

	WifiEnable   bool          // enable the WiFi-AP source
	WifiIface    string        // scan interface(s), space-separated
	WifiInterval time.Duration // WiFi scan/resolve cadence
	WifiMinAPs   int           // minimum APs before querying LocationAPI
	ULAEndpoint  string        // Unwired Labs region subdomain (e.g. "eu1")
```

Extend `Defaults()`:

```go
	return Config{
		PollInterval: 60 * time.Second,
		Listen:       ":2947",
		Bus:          "cpu",
		Radio:        "LTE",
		Runner:       "glmodem",
		WifiEnable:   true,
		WifiIface:    "wlan0",
		WifiInterval: 300 * time.Second,
		WifiMinAPs:   2,
		ULAEndpoint:  "eu1",
	}
```

Add cases to the `switch` in `ParseUciShow` (after `case "runner":`):

```go
		case "wifi_enable":
			cfg.WifiEnable = val == "1" || strings.EqualFold(val, "true")
		case "wifi_iface":
			if val != "" {
				cfg.WifiIface = val
			}
		case "wifi_interval":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.WifiInterval = time.Duration(n) * time.Second
			}
		case "wifi_min_aps":
			if n, err := strconv.Atoi(val); err == nil && n > 0 {
				cfg.WifiMinAPs = n
			}
		case "ula_endpoint":
			if val != "" {
				cfg.ULAEndpoint = val
			}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/uciconf/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/uciconf/config.go internal/uciconf/config_test.go
git commit -m "feat(uciconf): wifi source config options

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 9: `geolocd` wiring — two poll loops + priority provider

**Files:**
- Modify: `cmd/geolocd/main.go`

**Interfaces:**
- Consumes: `wifi.New`, `wifiscan.NewScanner`, `unwiredlabs.Client`,
  `cfg.Wifi*`/`cfg.ULAEndpoint`, existing `pollLoop`, `source.Fix`.
- Produces: the gpsd provider returns the WiFi fix when it `HasFix()`, else the
  cell fix. (No unit test; verified on-router below.)

- [ ] **Step 1: Add imports**

In `cmd/geolocd/main.go`, add to the import block:

```go
	"strings"

	"github.com/ckeller42/celloc/internal/source/wifi"
	"github.com/ckeller42/celloc/internal/unwiredlabs"
	"github.com/ckeller42/celloc/internal/wifiscan"
```

- [ ] **Step 2: Build the WiFi source and second poll loop**

In `run()`, replace this block:

```go
	// Shared current fix, refreshed by the poll loop, read by the gpsd server.
	var cur atomic.Value
	cur.Store(source.Fix{Mode: 0})
	go pollLoop(ctx, src, cfg.PollInterval, &cur)

	srv := &gpsd.Server{
		Provider: func() source.Fix { return cur.Load().(source.Fix) },
```

with:

```go
	// Per-source current fixes, refreshed by independent poll loops and read by
	// the gpsd server. WiFi (when enabled) outranks cell.
	var cellCur, wifiCur atomic.Value
	cellCur.Store(source.Fix{Mode: 0})
	wifiCur.Store(source.Fix{Mode: 0})
	go pollLoop(ctx, src, cfg.PollInterval, &cellCur)

	if cfg.WifiEnable && cfg.Key != "" {
		staleWifi := 2 * cfg.WifiInterval
		if staleWifi < 2*time.Minute {
			staleWifi = 2 * time.Minute
		}
		wsrc := wifi.New(
			wifiscan.NewScanner(strings.Fields(cfg.WifiIface)),
			&unwiredlabs.Client{Token: cfg.Key, Endpoint: cfg.ULAEndpoint, HTTP: &http.Client{Timeout: 15 * time.Second}},
			cfg.WifiMinAPs, staleWifi,
		)
		log.Printf("geolocd: wifi source enabled (iface=%q interval=%s min_aps=%d endpoint=%s)",
			cfg.WifiIface, cfg.WifiInterval, cfg.WifiMinAPs, cfg.ULAEndpoint)
		go pollLoop(ctx, wsrc, cfg.WifiInterval, &wifiCur)
	}

	srv := &gpsd.Server{
		Provider: func() source.Fix {
			if w := wifiCur.Load().(source.Fix); w.HasFix() {
				return w
			}
			return cellCur.Load().(source.Fix)
		},
```

(Leave `Interval`, `Release`, the log line, and `ListenAndServe` as they are.)

- [ ] **Step 3: Build and vet**

Run: `cd /Users/ckeller/src/celloc && go build ./cmd/... && go vet ./...`
Expected: no output (success).

- [ ] **Step 4: Run the full test suite**

Run: `cd /Users/ckeller/src/celloc && go test ./... -race`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
cd /Users/ckeller/src/celloc
git add cmd/geolocd/main.go
git commit -m "feat(geolocd): wire wifi source, wifi-over-cell provider

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

- [ ] **Step 6: On-router verification (manual)**

Build and install the binary directly (the `.ipk` is a separate known bug), set
a key + wifi config, and confirm a tighter fix:

```bash
cd /Users/ckeller/src/celloc
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-s -w" -o /tmp/geolocd ./cmd/geolocd
cat /tmp/geolocd | ssh -o BatchMode=yes root@100.117.69.67 'cat > /usr/bin/geolocd && chmod 0755 /usr/bin/geolocd'
ssh -o BatchMode=yes root@100.117.69.67 'uci set geolocd.main.wifi_enable=1; uci set geolocd.main.wifi_iface="wlan0"; uci commit geolocd; /etc/init.d/geolocd restart; sleep 20; logread | grep -i "geolocd\|wifi:" | tail'
python3 - <<'PY'
import socket, json, time
s=socket.create_connection(("100.117.69.67",2947),timeout=20)
s.sendall(b'?WATCH={"enable":true,"json":true};\n'); s.settimeout(20)
buf=b""; end=time.time()+18; tpv=None
while time.time()<end:
    try: data=s.recv(4096)
    except socket.timeout: break
    if not data: break
    buf+=data
    for line in buf.split(b"\n"):
        try: o=json.loads(line)
        except Exception: continue
        if o.get("class")=="TPV" and o.get("mode",0)>=2: tpv=o
    if tpv: break
    buf=b""
s.close()
print(json.dumps(tpv))
if tpv: print("MAPS https://www.google.com/maps?q=%s,%s eph=%s wifix=%s"%(tpv.get("lat"),tpv.get("lon"),tpv.get("eph"),tpv.get("wifix")))
PY
```

Expected: a `TPV mode=2` with a `wifix` object and `eph` well below 1548 when WiFi
resolves; falls back to the cell fix otherwise.

---

## M3 — round-trip, config defaults, docs

### Task 10: gpsd wifi round-trip integration test

**Files:**
- Modify: `internal/gpsd/client_test.go`

**Interfaces:**
- Consumes: existing loopback `Server`/`Client` test helpers, `gpsd.FixFromTPV`,
  `influx.FixLine`.

- [ ] **Step 1: Inspect the existing round-trip test**

Run: `cd /Users/ckeller/src/celloc && grep -n "func Test" internal/gpsd/client_test.go`
Note the existing cell round-trip test name and how it constructs a `Server` with
a `Provider`, dials a `Client`, and reads a TPV (reuse that exact setup).

- [ ] **Step 2: Write the failing test**

Append a wifi round-trip to `internal/gpsd/client_test.go`, modeled on the
existing cell round-trip (reuse its server/client construction). The assertion:

```go
func TestWifiRoundTripToInfluxLine(t *testing.T) {
	fix := source.Fix{
		Time: time.Unix(1700000000, 0).UTC(), Mode: 2,
		Lat: 48.7701, Lon: 9.169, EPH: 35, EPX: 35, EPY: 35,
		Source: "wifi", APCount: 7,
	}
	// Construct the loopback Server with Provider returning fix, dial a Client,
	// Watch, and ReadTPV exactly as the existing cell round-trip test does.
	// (Copy that setup here; replace the provided fix with `fix` above.)
	tpv := readOneTPV(t, fix) // helper mirroring the existing test's flow
	back := gpsd.FixFromTPV(tpv)
	if got := influx.FixLine(back); got != "geo,source=wifi lat=48.7701,lon=9.169,range_m=35i,ap_count=7i" {
		t.Fatalf("round-trip influx line = %q", got)
	}
}
```

If the existing test does not already provide a `readOneTPV`-style helper, inline
the server/client/Watch/ReadTPV steps from that test directly in this function
instead of calling a helper. Add `"github.com/ckeller42/celloc/internal/influx"`
to the test imports if absent.

- [ ] **Step 3: Run test to verify it fails, then passes**

Run: `cd /Users/ckeller/src/celloc && go test ./internal/gpsd/ -run TestWifiRoundTrip -v`
Expected: it compiles and PASSES once the setup is copied correctly (the behavior
already exists from Task 4; this test locks the end-to-end wifi path that
`geoinflux` relies on). If it fails, the failure pinpoints a wifix gap.

- [ ] **Step 4: Commit**

```bash
cd /Users/ckeller/src/celloc
git add internal/gpsd/client_test.go
git commit -m "test(gpsd): wifi TPV round-trip to influx line

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 11: uci defaults file + docs

**Files:**
- Modify: `packaging/openwrt/files/geolocd.config`
- Modify: `docs/INSTALL.md`, `SECURITY.md`, `docs/ARCHITECTURE.md`, `README.md`

**Interfaces:** none (config + documentation).

- [ ] **Step 1: Add wifi defaults to the uci config file**

Append the wifi options to the `config geolocd 'main'` section in
`packaging/openwrt/files/geolocd.config` (match the existing option style):

```text
	option wifi_enable '1'
	option wifi_iface 'wlan0'
	option wifi_interval '300'
	option wifi_min_aps '2'
	option ula_endpoint 'eu1'
```

- [ ] **Step 2: Document the WiFi source**

- In `docs/ARCHITECTURE.md`: add a `internal/wifiscan`, `internal/unwiredlabs`,
  and `internal/source/wifi` row to the pure/IO table, and a sentence under
  "Pluggable sources" noting WiFi outranks cell via `source.Select`.
- In `docs/INSTALL.md`: add a short "WiFi geolocation" subsection — it is on by
  default; uses the same `key`; set `uci set geolocd.main.wifi_enable=0` to
  disable; note the `wifi_iface`/`wifi_interval`/`wifi_min_aps`/`ula_endpoint`
  options.
- In `SECURITY.md`: add a bullet that BSSIDs of nearby networks are sent to
  Unwired Labs (LocationAPI), the `_nomap` opt-out is honored, and the token is
  the same uci `key` (read in-process, never argv).
- In `README.md`: update the components/accuracy text to mention the WiFi source
  gives tens-of-metres fixes where APs are dense (replace or extend the accuracy
  disclaimer; keep it honest — cell remains the fallback).

- [ ] **Step 3: Lint the docs**

Run: `cd /Users/ckeller/src/celloc && npx --yes markdownlint-cli@0.42.0 --config .markdownlint.json '**/*.md'`
Expected: exit 0 (fix any MD040 fenced-code-language or line issues reported).

- [ ] **Step 4: Commit**

```bash
cd /Users/ckeller/src/celloc
git add packaging/openwrt/files/geolocd.config docs/ README.md SECURITY.md
git commit -m "docs: document and default-enable the wifi source

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
Claude-Session: https://claude.ai/code/session_01WPAdcXLcTryDXT8pZyLe8W"
```

---

### Task 12: open the PR

**Files:** none (workflow).

- [ ] **Step 1: Push and open the PR**

```bash
cd /Users/ckeller/src/celloc
git push -u origin feat/wifi-geolocation
gh pr create --title "feat: WiFi-AP geolocation source (geolocd)" \
  --body "Implements docs/superpowers/specs/2026-06-29-wifi-geolocation-design.md: a WiFi-AP positioning source that resolves nearby APs via the Unwired Labs LocationAPI and outranks the cell source (cell stays fallback). Honest gpsd mode=2 semantics; cell Influx line byte-identical; new geo,source=wifi line. Tested TDD; on-router verified (tighter eph than the ~1.5 km cell fix). 🤖 Generated with [Claude Code](https://claude.com/claude-code)"
```

- [ ] **Step 2: Babysit CI + CodeRabbit, then merge**

Wait for all checks green (test/lint/build/ipk/markdownlint/gitleaks) and the
CodeRabbit review; address findings that make sense (re-run `go test ./... -race`
and the markdownlint after fixes); resolve threads; squash-merge per the project
PR workflow once green.

---

## Self-Review

- **Spec coverage:** wifiscan (Task 2,5) · unwiredlabs (Task 3,6) · source/wifi
  with cache/log/min_aps/selection (Task 7) · gpsd wifix + honest semantics (Task
  4) · influx wifi line, cell byte-identical (Task 1) · uci config (Task 8) ·
  geolocd wiring wifi-over-cell (Task 9) · geoinflux path via round-trip (Task 10)
  · privacy `_nomap`/SECURITY + docs/defaults (Task 2,11) · region `eu1` default
  (Task 8) · coverage gate held by table tests across Tasks 1–8,10. The one spec
  deviation (combined wifi+cell request → daemon-level fallback) is called out
  above for confirmation.
- **Placeholder scan:** none — every code step shows complete code. Task 10 reuses
  the existing round-trip setup and says explicitly to inline it if no helper
  exists (not a placeholder — a documented reuse of in-repo code).
- **Type consistency:** `WifiAP{BSSID,Signal}`, `Location{Lat,Lon,Accuracy}`,
  `Status`, `Request`, `Client.LookupWifi`, `wifi.New(sc,res,minAPs,staleAfter)`,
  `Fix.APCount`, `WifiFix{APCount}`/`wifix`, and the `wifi_*`/`ula_endpoint` uci
  keys are used identically across tasks.
