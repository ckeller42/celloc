// Package influx formats and writes geolocation fixes to InfluxDB. line.go is
// pure (Fix -> line protocol); writer.go does the HTTP POST behind a Doer.
package influx

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ckeller42/celloc/internal/source"
)

// Measurement is the InfluxDB measurement name (kept identical to the original
// glinet-geoloc.sh schema so existing Grafana panels keep working).
const Measurement = "geo"

// ErrUnattributedFix is returned (wrapped) by FixLine for a fix whose Source is
// not one celloc emits ("wifi" or "cell"). geolocd tags every fix it serves via
// the TPV's wifix/cellfix extension, so a fix without one did not come from
// celloc's resolver: writing it would need an invented source tag (or an empty
// one, which is invalid line protocol) and zero-valued cell fields.
var ErrUnattributedFix = errors.New("influx: fix has no celloc source (neither wifix nor cellfix)")

// FixLine renders a Fix as an InfluxDB line-protocol point, byte-identical to
// the seed script's output:
//
//	geo,source=cell,radio=LTE lat=..,lon=..,range_m=Ni,mcc=Ni,mnc=Ni,cid=Ni,tac=Ni
//
// followed by the fix's own time as a nanosecond timestamp (Writer posts with
// ?precision=ns) so a fix uploaded late is stored at the time it was taken.
// A fix without a time gets no timestamp (InfluxDB assigns server time).
//
// Only Source "wifi" and "cell" are rendered; any other Source (notably the
// empty one of a TPV that carried neither extension) returns an error wrapping
// ErrUnattributedFix and no line. A tag whose value is empty (a cell fix
// without a radio) is omitted rather than written as "radio=", which InfluxDB
// rejects.
//
// Lat/lon are formatted with strconv.FormatFloat(-1) (shortest round-trippable
// form). Values are numerically identical to the seed's raw JSON substring and
// the schema/tags/field-order match byte-for-byte, but the float text may differ
// in trailing zeros (seed "48.10" vs "48.1"); InfluxDB parses both identically.
func FixLine(f source.Fix) (string, error) {
	switch f.Source {
	case "wifi", "cell":
		return fixFields(f) + timestamp(f.Time), nil
	default:
		return "", fmt.Errorf("%w: source=%q", ErrUnattributedFix, f.Source)
	}
}

func fixFields(f source.Fix) string {
	lat := strconv.FormatFloat(f.Lat, 'f', -1, 64)
	lon := strconv.FormatFloat(f.Lon, 'f', -1, 64)
	rangeM := strconv.Itoa(int(f.EPH)) + "i"
	if f.Source == "wifi" {
		return Measurement + tags("source", f.Source) +
			" lat=" + lat +
			",lon=" + lon +
			",range_m=" + rangeM +
			",ap_count=" + strconv.Itoa(f.APCount) + "i"
	}
	return Measurement + tags("source", f.Source, "radio", f.Radio) +
		" lat=" + lat +
		",lon=" + lon +
		",range_m=" + rangeM +
		",mcc=" + strconv.Itoa(f.MCC) + "i" +
		",mnc=" + strconv.Itoa(f.MNC) + "i" +
		",cid=" + strconv.FormatInt(f.CID, 10) + "i" +
		",tac=" + strconv.Itoa(f.TAC) + "i"
}

// tags renders key/value pairs as ",k=v" line-protocol tags, escaping values
// and omitting any pair whose value is empty: InfluxDB rejects an empty tag
// value, and a tag that is absent is what an unknown value means anyway.
func tags(kv ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] == "" {
			continue
		}
		b.WriteString("," + kv[i] + "=" + tagEscape(kv[i+1]))
	}
	return b.String()
}

// StatusMeasurement is the heartbeat measurement written by StatusLine.
const StatusMeasurement = "geo_status"

// StatusLine renders the uploader's view of the position feed, written every
// upload interval whether or not there is a fix, so InfluxDB can tell a dead
// uploader (no points) from an unreachable daemon (connected=false), no fix
// (mode<2) and a stale fix (growing fix_age_s):
//
//	geo_status mode=Ni,fix_age_s=F,connected=B <now-ns>
//
// fix_age_s is now minus the fix time in seconds (millisecond resolution), or
// -1 when the fix carries no time.
func StatusLine(f source.Fix, connected bool, now time.Time) string {
	age := -1.0
	if !f.Time.IsZero() {
		age = math.Round(now.Sub(f.Time).Seconds()*1000) / 1000
	}
	return StatusMeasurement +
		" mode=" + strconv.Itoa(f.Mode) + "i" +
		",fix_age_s=" + strconv.FormatFloat(age, 'f', -1, 64) +
		",connected=" + strconv.FormatBool(connected) +
		timestamp(now)
}

// timestamp renders t as a line-protocol nanosecond timestamp (with its leading
// space), or "" for the zero time.
func timestamp(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return " " + strconv.FormatInt(t.UnixNano(), 10)
}

// tagEscape escapes the line-protocol tag special characters (space, comma, =).
func tagEscape(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', ',', '=':
			out = append(out, '\\', s[i])
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}
