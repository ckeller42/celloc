// Package influx formats and writes geolocation fixes to InfluxDB. line.go is
// pure (Fix -> line protocol); writer.go does the HTTP POST behind a Doer.
package influx

import (
	"strconv"

	"github.com/ckeller42/celloc/internal/source"
)

// Measurement is the InfluxDB measurement name (kept identical to the original
// glinet-geoloc.sh schema so existing Grafana panels keep working).
const Measurement = "geo"

// FixLine renders a Fix as an InfluxDB line-protocol point, byte-identical to
// the seed script's output:
//
//	geo,source=cell,radio=LTE lat=..,lon=..,range_m=Ni,mcc=Ni,mnc=Ni,cid=Ni,tac=Ni
//
// No timestamp is appended — InfluxDB assigns server time (matches the seed and
// the ?precision=s write endpoint).
//
// Lat/lon are formatted with strconv.FormatFloat(-1) (shortest round-trippable
// form). Values are numerically identical to the seed's raw JSON substring and
// the schema/tags/field-order match byte-for-byte, but the float text may differ
// in trailing zeros (seed "48.10" vs "48.1"); InfluxDB parses both identically.
func FixLine(f source.Fix) string {
	lat := strconv.FormatFloat(f.Lat, 'f', -1, 64)
	lon := strconv.FormatFloat(f.Lon, 'f', -1, 64)
	return Measurement +
		",source=" + tagEscape(f.Source) +
		",radio=" + tagEscape(f.Radio) +
		" lat=" + lat +
		",lon=" + lon +
		",range_m=" + strconv.Itoa(int(f.EPH)) + "i" +
		",mcc=" + strconv.Itoa(f.MCC) + "i" +
		",mnc=" + strconv.Itoa(f.MNC) + "i" +
		",cid=" + strconv.FormatInt(f.CID, 10) + "i" +
		",tac=" + strconv.Itoa(f.TAC) + "i"
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
