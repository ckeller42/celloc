// Package qeng parses the Quectel `AT+QENG="servingcell"` modem response into
// serving-cell identifiers. Pure: no I/O, no env, fully table-testable.
//
// Observed GL-E5800 (SDX75) line formats (the leading "servingcell",<state>
// tokens are omitted by this modem variant):
//
//	+QENG: "LTE","FDD",<MCC>,<MNC>,<CID-hex>,<PCID>,<EARFCN>,<band>,<UL>,<DL>,<TAC-hex>,<RSRP>,...
//	+QENG: "NR5G-NSA",<MCC>,<MNC>,<PCID>,<RSRP>,<SINR>,<RSRQ>,<ARFCN>,<band>,...
//	+QENG: "NR5G-SA",<duplex>,<MCC>,<MNC>,<NCI-hex>,<PCID>,<TAC-hex>,<ARFCN>,<band>,...
//
// Only LTE carries IDs usable for OpenCelliD in v1 (and is present as the anchor
// under NR5G-NSA). NR lines are decoded best-effort for completeness/future use.
package qeng

import (
	"errors"
	"strconv"
	"strings"
)

// Radio is the access technology of a serving-cell line.
type Radio string

const (
	RadioLTE     Radio = "LTE"
	RadioNR5GNSA Radio = "NR5G-NSA"
	RadioNR5GSA  Radio = "NR5G-SA"
)

// Cell is one decoded serving-cell line.
type Cell struct {
	Radio Radio
	MCC   int
	MNC   int
	CID   int64 // 0 when absent (e.g. NR5G-NSA). NR NCI can exceed int32.
	TAC   int   // 0 when absent
	HasID bool  // true when both CID and TAC were decoded (geolocatable)
	Raw   string
}

// ErrNoCells is returned when the output contains no decodable +QENG line.
var ErrNoCells = errors.New("qeng: no serving-cell lines")

// ParseServingCell decodes every +QENG line in the raw modem output, preserving
// order. Lines that fail to decode are skipped (not fatal) unless none decode.
func ParseServingCell(out string) ([]Cell, error) {
	var cells []Cell
	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimRight(raw, "\r")
		i := strings.Index(line, "+QENG:")
		if i < 0 {
			continue
		}
		body := strings.TrimSpace(line[i+len("+QENG:"):])
		f := splitFields(body)
		if len(f) == 0 {
			continue
		}
		if c, ok := decode(f, line); ok {
			cells = append(cells, c)
		}
	}
	if len(cells) == 0 {
		return nil, ErrNoCells
	}
	return cells, nil
}

// SelectGeolocatable returns the best cell to geolocate in v1: the first LTE
// cell with IDs (also the NSA anchor). Returns false if there is none.
func SelectGeolocatable(cells []Cell) (Cell, bool) {
	for _, c := range cells {
		if c.Radio == RadioLTE && c.HasID {
			return c, true
		}
	}
	return Cell{}, false
}

func splitFields(body string) []string {
	parts := strings.Split(body, ",")
	for i, p := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(p), `"`)
	}
	return parts
}

func decode(f []string, raw string) (Cell, bool) {
	switch Radio(f[0]) {
	case RadioLTE:
		// f: [LTE FDD MCC MNC CID PCID EARFCN band UL DL TAC ...]
		if len(f) < 11 {
			return Cell{}, false
		}
		mcc, e1 := atoiDec(f[2])
		mnc, e2 := atoiDec(f[3])
		cid, e3 := atoiHex(f[4])
		tac, e4 := atoiHex(f[10])
		if e1 != nil || e2 != nil {
			return Cell{}, false
		}
		c := Cell{Radio: RadioLTE, MCC: mcc, MNC: mnc, Raw: raw}
		if e3 == nil && e4 == nil {
			c.CID, c.TAC, c.HasID = cid, int(tac), true
		}
		return c, true
	case RadioNR5GNSA:
		// f: [NR5G-NSA MCC MNC PCID ...] — no CID/TAC on the NSA line.
		if len(f) < 3 {
			return Cell{}, false
		}
		mcc, e1 := atoiDec(f[1])
		mnc, e2 := atoiDec(f[2])
		if e1 != nil || e2 != nil {
			return Cell{}, false
		}
		return Cell{Radio: RadioNR5GNSA, MCC: mcc, MNC: mnc, Raw: raw}, true
	case RadioNR5GSA:
		// f: [NR5G-SA duplex MCC MNC NCI PCID TAC ...] (best-effort)
		if len(f) < 7 {
			return Cell{}, false
		}
		mcc, e1 := atoiDec(f[2])
		mnc, e2 := atoiDec(f[3])
		nci, e3 := atoiHex(f[4])
		tac, e4 := atoiHex(f[6])
		if e1 != nil || e2 != nil {
			return Cell{}, false
		}
		c := Cell{Radio: RadioNR5GSA, MCC: mcc, MNC: mnc, Raw: raw}
		if e3 == nil && e4 == nil {
			c.CID, c.TAC, c.HasID = nci, int(tac), true
		}
		return c, true
	default:
		return Cell{}, false
	}
}

// atoiDec parses a base-10 int, tolerating leading zeros ("03" -> 3, not octal).
func atoiDec(s string) (int, error) { return strconv.Atoi(strings.TrimSpace(s)) }

func atoiHex(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 16, 64)
}
