// Package gpsd implements the JSON subset of the gpsd protocol needed by common
// clients (gpspipe -w, cgps, gps.py). report.go holds the pure report structs +
// marshaling; server.go (separate) does the TCP I/O.
package gpsd

import (
	"encoding/json"
	"time"

	"github.com/ckeller42/celloc/internal/source"
)

// Protocol version advertised on connect. gpsd clients block until VERSION.
const (
	ProtoMajor = 3
	ProtoMinor = 14
)

// Version is sent immediately on every new connection.
type Version struct {
	Class      string `json:"class"` // "VERSION"
	Release    string `json:"release"`
	Rev        string `json:"rev"`
	ProtoMajor int    `json:"proto_major"`
	ProtoMinor int    `json:"proto_minor"`
}

// Device describes the single synthetic device celloc exposes.
type Device struct {
	Class     string `json:"class"` // "DEVICE"
	Path      string `json:"path"`
	Driver    string `json:"driver"`
	Activated string `json:"activated,omitempty"`
}

// Devices is the reply to ?DEVICES and part of the WATCH handshake.
type Devices struct {
	Class   string   `json:"class"` // "DEVICES"
	Devices []Device `json:"devices"`
}

// Watch echoes the watch state back to the client.
type Watch struct {
	Class  string `json:"class"` // "WATCH"
	Enable bool   `json:"enable"`
	JSON   bool   `json:"json"`
	NMEA   bool   `json:"nmea"`
}

// CellFix is a NON-STANDARD extension object embedded in TPV. Standard gpsd
// clients ignore unknown keys; celloc's own uploader reads it to preserve the
// MCC/MNC/CID/TAC the InfluxDB schema wants without breaking gpsd compatibility.
type CellFix struct {
	Radio string `json:"radio"`
	MCC   int    `json:"mcc"`
	MNC   int    `json:"mnc"`
	CID   int64  `json:"cid"`
	TAC   int    `json:"tac"`
	Range int    `json:"range"`
}

// TPV is a time-position-velocity report. Optional numeric fields are pointers
// so they are OMITTED (not zero) when unknown — never fake altitude/speed, and
// never emit a 0,0 position for a no-fix.
type TPV struct {
	Class   string   `json:"class"` // "TPV"
	Device  string   `json:"device,omitempty"`
	Mode    int      `json:"mode"` // 0/1 = no fix, 2 = 2D
	Time    string   `json:"time,omitempty"`
	Lat     *float64 `json:"lat,omitempty"`
	Lon     *float64 `json:"lon,omitempty"`
	EPX     *float64 `json:"epx,omitempty"`
	EPY     *float64 `json:"epy,omitempty"`
	EPH     *float64 `json:"eph,omitempty"`
	CellFix *CellFix `json:"cellfix,omitempty"`
}

// SKY is emitted so cgps shows data; celloc has no satellites (not GNSS), so it
// reports an empty list and fabricates no DOP values.
type SKY struct {
	Class      string `json:"class"` // "SKY"
	Device     string `json:"device,omitempty"`
	Satellites []any  `json:"satellites"`
}

// Poll is the reply to ?POLL with the latest tpv/sky.
type Poll struct {
	Class  string `json:"class"` // "POLL"
	Time   string `json:"time"`
	Active int    `json:"active"`
	TPV    []TPV  `json:"tpv"`
	SKY    []SKY  `json:"sky"`
}

// timeFormat is gpsd's ISO-8601 with milliseconds in UTC.
const timeFormat = "2006-01-02T15:04:05.000Z07:00"

// TPVFromFix builds a TPV from a source.Fix, flagging cell fixes honestly:
// mode=2, epx=epy=eph=range, no alt/speed; no-fix -> mode 0 with no coordinates.
func TPVFromFix(f source.Fix, device string) TPV {
	t := TPV{Class: "TPV", Device: device, Mode: f.Mode}
	if !f.HasFix() {
		return t
	}
	lat, lon := f.Lat, f.Lon
	epx, epy, eph := f.EPX, f.EPY, f.EPH
	t.Time = f.Time.UTC().Format(timeFormat)
	t.Lat, t.Lon = &lat, &lon
	t.EPX, t.EPY, t.EPH = &epx, &epy, &eph
	if f.MCC != 0 || f.CID != 0 {
		t.CellFix = &CellFix{Radio: f.Radio, MCC: f.MCC, MNC: f.MNC, CID: f.CID, TAC: f.TAC, Range: int(f.EPH)}
	}
	return t
}

// FixFromTPV reconstructs a source.Fix from a received TPV (the inverse of
// TPVFromFix), used by the Pi uploader. Cell identifiers come from the cellfix
// extension; a TPV without a fix yields Mode 0.
func FixFromTPV(t TPV) source.Fix {
	f := source.Fix{Mode: t.Mode}
	if t.Lat != nil {
		f.Lat = *t.Lat
	}
	if t.Lon != nil {
		f.Lon = *t.Lon
	}
	if t.EPH != nil {
		f.EPH = *t.EPH
	}
	if t.EPX != nil {
		f.EPX = *t.EPX
	}
	if t.EPY != nil {
		f.EPY = *t.EPY
	}
	if t.Time != "" {
		if ts, err := time.Parse(timeFormat, t.Time); err == nil {
			f.Time = ts
		}
	}
	if t.CellFix != nil {
		f.Source = "cell"
		f.Radio = t.CellFix.Radio
		f.MCC, f.MNC, f.CID, f.TAC = t.CellFix.MCC, t.CellFix.MNC, t.CellFix.CID, t.CellFix.TAC
	}
	return f
}

// SKYEmpty returns the satellite-less SKY report for the device.
func SKYEmpty(device string) SKY {
	return SKY{Class: "SKY", Device: device, Satellites: []any{}}
}

// MarshalLine JSON-encodes v and appends the gpsd CRLF line terminator.
func MarshalLine(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\r', '\n'), nil
}
