// Package geoloc holds provider-neutral geolocation result types shared by the
// resolver providers (unwiredlabs, google) and the wifi source. Leaf package: no
// other internal deps.
package geoloc

// Location is a resolved position. Accuracy is the provider's error radius in
// meters (used as the gpsd EPH for the resulting fix).
type Location struct {
	Lat      float64
	Lon      float64
	Accuracy float64
}

// CellTower is a serving cell passed to a provider to anchor/blend a fix. Radio
// is the access technology from the modem (e.g. "LTE", "NR5G-NSA"); Signal is
// RSRP in dBm, 0 when unknown.
type CellTower struct {
	Radio  string
	MCC    int
	MNC    int
	CID    int64
	TAC    int
	Signal int
}
