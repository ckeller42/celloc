---
name: google-geolocation-key
description: Use when setting up, rotating, restricting, or capping the Google Geolocation API key that geolocd needs — creating the GCP project, linking billing, enabling the API, creating a key restricted to geolocation.googleapis.com, setting a daily request cap to stay in the free tier, or writing the key into the router's uci config. Also use when geolocd logs Google auth/quota errors or when checking the API cost of a wifi_interval.
---

# Google Geolocation API key (gcloud)

geolocd resolves WiFi APs + the serving cell through the Google Geolocation API.
Everything the key needs is scriptable with the gcloud CLI:
`scripts/gcloud-geolocation-key.sh` does it idempotently. Run it rather than
clicking through the console.

## Prerequisites

- Google Cloud CLI: `mise use -g gcloud@latest` (or the official installer), then
  `gcloud auth login` (interactive, browser).
- A billing account (Maps Platform APIs require one even inside the free tier):
  `gcloud billing accounts list`.

## Run it

```sh
scripts/gcloud-geolocation-key.sh -p <PROJECT> -d            # dry run first
scripts/gcloud-geolocation-key.sh -p <PROJECT> -b <ACCOUNT>  # link billing if needed
scripts/gcloud-geolocation-key.sh -p <PROJECT> -r root@<router> -j <user>@<jump-host>
```

| Flag | Meaning |
|---|---|
| `-p` | project id (required); `-c` creates it |
| `-b` | billing account to link when billing is off |
| `-n` | key display name (default `celloc geolocd`); an existing key with that name is reused |
| `-q` | daily cap (default `320`; `0` leaves the quota alone) |
| `-r` / `-j` | write the key to the router's `geolocd.main.google_key` over ssh (optionally via a jump host) and restart geolocd |
| `-k` | print the key string (off by default) |
| `-d` | dry run |

## What it does

1. Checks `gcloud` login; project exists (or `-c`); billing enabled (or `-b`).
2. Enables `geolocation.googleapis.com`, `apikeys.googleapis.com`, `cloudquotas.googleapis.com`.
3. Creates (or reuses) a key **restricted to the Geolocation API only**.
4. Sets the Geolocation `BillableDefaultPerDayPerProject` quota via the Cloud
   Quotas REST API (`PATCH …?allowMissing=true`, reusing an existing preference).
5. Optionally sends the key to the router over **ssh stdin** (never argv, never
   printed) and restarts geolocd.

## Cost math

Geolocation is an Essentials SKU: **10,000 free requests/month**, then $5 per
1,000. geolocd sends **one request per poll**:

| `wifi_interval` | per day | 31-day month |
|---|---|---|
| 300 s | 288 | 8,928 |
| 240 s | 360 | 11,160 (over) |

Break-even is about 268 s. The default cap of 320/day allows at most 9,920 per
31-day month, and it also bounds a crash/restart loop: every daemon start polls
at once, and procd respawns without limit.

## Gotchas

| Symptom | Cause / fix |
|---|---|
| `gcloud beta quotas preferences create` → "decreases effective quota unsafely" | Lowering an unlimited quota is a >10% decrease; gcloud has no flag for it. The script uses REST with `ignoreSafetyChecks=QUOTA_DECREASE_PERCENTAGE_TOO_HIGH` (a **query** parameter, not a body field). |
| REST call → `SERVICE_DISABLED` for Cloud Quotas | `cloudquotas.googleapis.com` must be enabled on the project; the script enables it. A just-enabled API can take a minute; re-run. |
| `api-keys create --format=json` shows the key | The create response contains the key string. The script selects only `response.name`. |
| Which project owns a key? | `gcloud services api-keys lookup <KEY>` returns the key resource (project number); map it with `gcloud projects list`. Keep the key out of shell history. |
| Quota hit | geolocd gets errors until the daily reset (Pacific midnight) and serves no fix; since v0.3.0 this is logged and visible in `geo_status`. |

## Related

- `docs/INSTALL.md` — installing geolocd and setting `google_key` by hand.
- `opkg-package-dev` skill (user-level, if installed) — packaging geolocd.
