# InfluxDB schema

`geoinflux` writes two measurements to the configured bucket through `POST /api/v2/write` with
`precision=ns` (source: `internal/influx`). The token is sent as an `Authorization: Token` header.

## `geo`: one point per fix

Written for TPV `mode>=2`, at most once per `-min-interval`.
The point is stamped with the fix's own time from the TPV, so a fix uploaded
late is stored at the time it was taken (a TPV without a time gets InfluxDB's
server time). The tags and fields depend on what resolved the fix:

```text
geo,source=wifi lat=<f>,lon=<f>,range_m=<n>i,ap_count=<n>i <ns>
geo,source=cell,radio=<LTE|NR5G-SA> lat=<f>,lon=<f>,range_m=<n>i,mcc=<n>i,mnc=<n>i,cid=<n>i,tac=<n>i <ns>
```

- `source=wifi`: the provider resolved the WiFi scan (plus the serving cell).
  Carries `ap_count` (APs sent) and has **no** `radio` tag or cell fields.
- `source=cell`: only the serving cell anchored the fix. This line keeps the
  legacy `geo` schema (tags, fields and field order), so existing cell-based
  Grafana panels keep working; panels that filter on `source="cell"` will not
  show WiFi fixes.

`range_m` is the reported error radius (gpsd `eph`) in metres, rounded down.

A `mode>=2` TPV that carries **neither** a `wifix` nor a `cellfix` object is
not a celloc fix (`geolocd` attaches one of them to every fix it serves; a
plain TPV means `-gpsd` points at some other gpsd). No `geo` point is written
for it, since there is no honest `source` to tag it with. The `geo_status`
heartbeat still records its `mode`, and `geoinflux` logs
`skipping geo point: ...` (at most once a minute). A tag whose value is empty
(for example a cell fix without a radio) is left out of the point; it is never
written as `radio=`.

## `geo_status`: uploader heartbeat

Written whether or not there is a fix:

```text
geo_status mode=<n>i,fix_age_s=<f>,connected=<bool> <now-ns>
```

| Field | Meaning |
| --- | --- |
| `mode` | gpsd TPV mode from the router (`2` = fix, below `2` = no fix); `0` while disconnected |
| `fix_age_s` | Seconds since the fix's own time (ms resolution), `-1` when the TPV has no time |
| `connected` | `false` while the router's gpsd socket is unreachable |

It is written at most once per `-min-interval` while TPVs arrive, and
immediately whenever connectivity changes (connect or disconnect). This tells a
dead uploader (no points at all) apart from an unreachable router
(`connected=false`), no fix (`mode<2`) and a stale fix (growing `fix_age_s`).
