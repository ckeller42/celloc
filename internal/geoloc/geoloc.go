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
