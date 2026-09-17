#!/usr/bin/env bash
# Provision a Google Geolocation API key for geolocd with the gcloud CLI.
#
# Idempotent: safe to re-run. Steps:
#   1. check gcloud login
#   2. project: must exist (or create it with -c)
#   3. billing: Maps Platform APIs need a billing account linked (-b links one)
#   4. enable geolocation, apikeys and cloudquotas APIs
#   5. API key restricted to geolocation.googleapis.com only (reused if a key
#      with the same display name exists)
#   6. daily request cap on the Geolocation API (Cloud Quotas), so a restart
#      loop can never leave the free tier
#   7. optionally write the key into the router's uci config over ssh (-r)
#
# The key string is never printed unless -k is given, and is sent to the
# router over ssh stdin (never argv).
#
# Free tier (Essentials SKU): 10,000 requests/month. geolocd makes one request
# per poll, so wifi_interval=300 s is 288/day (8,928 in a 31-day month). The
# default cap 320/day keeps even a 31-day month (9,920) inside the free tier.
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: gcloud-geolocation-key.sh -p PROJECT [options]

  -p PROJECT   GCP project id (required)
  -c           create the project if it does not exist
  -b ACCOUNT   billing account id to link (needed if billing is not enabled;
               list with: gcloud billing accounts list)
  -n NAME      API key display name (default: "celloc geolocd")
  -q N         daily Geolocation request cap (default: 320; 0 = leave as is)
  -r TARGET    also write the key to a router: ssh TARGET, e.g. root@<router>
  -j JUMP      ssh jump host for -r, e.g. pi@<pi>
  -k           print the key string to stdout (off by default)
  -d           dry run: show what would change, change nothing
  -h           help
EOF
}

PROJECT="" CREATE=0 BILLING="" NAME="celloc geolocd" CAP=320
ROUTER="" JUMP="" SHOWKEY=0 DRY=0
while getopts ":p:cb:n:q:r:j:kdh" o; do
  case "$o" in
    p) PROJECT=$OPTARG ;;
    c) CREATE=1 ;;
    b) BILLING=$OPTARG ;;
    n) NAME=$OPTARG ;;
    q) CAP=$OPTARG ;;
    r) ROUTER=$OPTARG ;;
    j) JUMP=$OPTARG ;;
    k) SHOWKEY=1 ;;
    d) DRY=1 ;;
    h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done
[ -n "$PROJECT" ] || { usage >&2; exit 2; }
case "$CAP" in *[!0-9]*|'') echo "-q must be a number" >&2; exit 2 ;; esac

SVC=geolocation.googleapis.com
log() { printf '==> %s\n' "$*" >&2; }
run() { if [ "$DRY" = 1 ]; then printf 'DRY: %s\n' "$*" >&2; else "$@"; fi; }

command -v gcloud >/dev/null || { echo "gcloud not found (install the Google Cloud CLI)" >&2; exit 1; }
acct=$(gcloud auth list --filter=status:ACTIVE --format='value(account)' 2>/dev/null)
[ -n "$acct" ] || { echo "not logged in: run 'gcloud auth login'" >&2; exit 1; }
log "gcloud account: $acct"

# 2. project
if gcloud projects describe "$PROJECT" >/dev/null 2>&1; then
  log "project $PROJECT exists"
elif [ "$CREATE" = 1 ]; then
  log "creating project $PROJECT"
  run gcloud projects create "$PROJECT" --quiet
else
  echo "project $PROJECT not found (use -c to create it)" >&2; exit 1
fi

# 3. billing
billing=$(gcloud billing projects describe "$PROJECT" --format='value(billingEnabled)' 2>/dev/null || true)
if [ "$billing" = "True" ]; then
  log "billing enabled"
elif [ -n "$BILLING" ]; then
  log "linking billing account"
  run gcloud billing projects link "$PROJECT" --billing-account="$BILLING" --quiet
else
  echo "billing is not enabled on $PROJECT; pass -b <ACCOUNT> (gcloud billing accounts list)" >&2
  exit 1
fi

# 4. APIs
log "enabling $SVC, apikeys, cloudquotas"
run gcloud services enable "$SVC" apikeys.googleapis.com cloudquotas.googleapis.com --project "$PROJECT" --quiet

# 5. key (restricted to the Geolocation API only)
key=$(gcloud services api-keys list --project "$PROJECT" \
  --filter="displayName=\"$NAME\"" --format='value(name)' 2>/dev/null | head -n1 || true)
if [ -n "$key" ]; then
  log "reusing key \"$NAME\"; enforcing restriction to $SVC"
  run gcloud services api-keys update "$key" --api-target=service="$SVC" --quiet >/dev/null
else
  log "creating key \"$NAME\" restricted to $SVC"
  if [ "$DRY" = 1 ]; then
    run gcloud services api-keys create --project "$PROJECT" --display-name="$NAME" --api-target=service="$SVC"
    key="<new-key>"
  else
    # --format picks only the resource name; the create response also holds the key string.
    key=$(gcloud services api-keys create --project "$PROJECT" --display-name="$NAME" \
      --api-target=service="$SVC" --format='value(response.name)' --quiet 2>/dev/null)
    [ -n "$key" ] || { echo "key creation failed" >&2; exit 1; }
  fi
fi
log "key: $key"

# 6. daily cap (Cloud Quotas). PATCH + allowMissing is create-or-update in one call.
# ignoreSafetyChecks is required: lowering an unlimited quota is a >10% decrease.
if [ "$CAP" -gt 0 ]; then
  log "setting $SVC daily cap to $CAP"
  base="https://cloudquotas.googleapis.com/v1/projects/$PROJECT/locations/global/quotaPreferences"
  tok=$(gcloud auth print-access-token)
  # Reuse an existing preference for this quota (only one may exist per quota).
  pref=$(curl -sS -G "$base" -H "Authorization: Bearer $tok" -H "X-Goog-User-Project: $PROJECT" \
    --data-urlencode "filter=service=\"$SVC\" AND quotaId=\"BillableDefaultPerDayPerProject\"" 2>/dev/null |
    python3 -c 'import sys,json
try: p=json.load(sys.stdin).get("quotaPreferences",[])
except Exception: p=[]
print(p[0]["name"].rsplit("/",1)[1] if p else "")' || true)
  pref=${pref:-geolocation-daily-cap}
  url="$base/$pref?allowMissing=true&ignoreSafetyChecks=QUOTA_DECREASE_PERCENTAGE_TOO_HIGH"
  body=$(printf '{"service":"%s","quotaId":"BillableDefaultPerDayPerProject","quotaConfig":{"preferredValue":"%s"},"justification":"geolocd daily cap: keep Geolocation API inside the free tier"}' "$SVC" "$CAP")
  if [ "$DRY" = 1 ]; then
    unset tok
    printf 'DRY: PATCH %s %s\n' "$url" "$body" >&2
  else
    resp=$(curl -sS -X PATCH "$url" -H "Authorization: Bearer $tok" \
      -H "X-Goog-User-Project: $PROJECT" -H "Content-Type: application/json" -d "$body")
    unset tok
    granted=$(printf '%s' "$resp" | python3 -c 'import sys,json; d=json.load(sys.stdin); e=d.get("error"); print("ERROR: "+e["message"]) if e else print(d.get("quotaConfig",{}).get("grantedValue","pending"))')
    case "$granted" in
      ERROR:*) echo "$granted" >&2; echo "(a freshly enabled API can take a few minutes; re-run)" >&2; exit 1 ;;
    esac
    log "daily cap granted: $granted"
  fi
fi

[ "$DRY" = 1 ] && { log "dry run done, nothing changed"; exit 0; }

# 7. key string: only to the router (stdin) and/or stdout when asked.
if [ -n "$ROUTER" ] || [ "$SHOWKEY" = 1 ]; then
  ks=$(gcloud services api-keys get-key-string "$key" --location global --format='value(keyString)')
  if [ -n "$ROUTER" ]; then
    log "writing key to $ROUTER (uci geolocd.main.google_key)"
    sshopts=(-o BatchMode=yes)
    [ -n "$JUMP" ] && sshopts+=(-J "$JUMP")
    # shellcheck disable=SC2016  # $K expands on the router, not here
    printf '%s\n' "$ks" | ssh "${sshopts[@]}" "$ROUTER" \
      'read -r K; uci set geolocd.main.google_key="$K"; uci commit geolocd; chmod 600 /etc/config/geolocd; /etc/init.d/geolocd restart; echo "router: key set (len ${#K})"'
  fi
  [ "$SHOWKEY" = 1 ] && printf '%s\n' "$ks"
  unset ks
else
  log "done. Key string: gcloud services api-keys get-key-string $key --location global"
fi
