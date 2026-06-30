# Security

## Reporting

Please report vulnerabilities privately via GitHub Security Advisories
("Report a vulnerability" on the Security tab) rather than a public issue.

## Secrets

- **OpenCelliD API key** lives in `uci` on the router (`/etc/config/geolocd`,
  `0600` root). `geolocd` reads it from uci itself, so it never appears in argv
  or `ps`.
- **InfluxDB write token** lives in `/etc/buspi/geo.env` (`0600`) on the Pi and
  is read from the environment by `geoinflux` — never passed on the command line.
- Keep router/device config backups **out of version control**; they bundle these
  secrets. CI runs `gitleaks` to catch accidental commits.

- **WiFi geolocation** sends the **BSSIDs (MAC addresses) of nearby networks**
  to the configured provider (Google or Unwired Labs). APs whose SSID ends in
  `_nomap` are excluded before the request is made (honoring the opt-out
  convention). The provider keys (`google_key`, OpenCelliD `key`) live in uci
  (`/etc/config/geolocd`, `0600`, read in-process, never in argv or `ps`).
  `:2947` stays LAN-only as noted below.

## Network exposure

- `geolocd` binds the gpsd socket on `:2947`. It must be reachable from the LAN
  (the Pi), so it is not loopback-only. On OpenWrt the default firewall drops
  inbound WAN — **do not** open a WAN port for it. Restrict to the LAN/Tailscale.
- The gpsd protocol is unauthenticated (as is upstream gpsd). Treat `:2947` as
  LAN-trusted only.

## Supply chain

- GitHub Actions are pinned to commit SHAs.
- No third-party Go modules in the daemon/uploader (standard library only),
  minimizing dependency risk.
