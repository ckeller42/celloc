# celloc review style guide (for Gemini Code Assist)

Review Go changes against these project rules:

- **Pure vs I/O split.** Packages that parse/marshal (`qeng`, `opencellid/parse`,
  `gpsd/report`, `influx/line`, `uciconf/config`, `source`) must stay free of
  network, filesystem, env, and wall-clock access — inject those via interfaces.
  Flag any I/O sneaking into a pure package.
- **No fabricated positioning data.** A cell-tower fix is `TPV mode=2` with
  `eph==epx==epy` from the OpenCelliD range and **no** `alt`/`speed`/`track`. A
  no-fix/stale position must be `mode=0` with no coordinates — never emit `0,0`.
- **Secrets.** The OpenCelliD and InfluxDB tokens must never appear in argv,
  logs, error messages, or committed files. The daemon reads the key from uci.
- **Tests.** Prefer table-driven tests; require edge cases (garbage/partial AT
  input, error/transient/unknown-cell paths, CRLF). Don't weaken assertions to
  chase coverage. Coverage gate on `./internal/...` is 85%.
- **OpenWrt.** Packaging must use procd, mark `/etc/config/geolocd` as a
  conffile, and the firmware-upgrade caveat (sysupgrade wipes `/usr/bin`) must be
  handled or documented.
- **Style.** gofumpt-formatted; golangci-lint (v2) clean; exported identifiers
  documented.
