package wifiscan_test

import (
	"reflect"
	"testing"

	"github.com/ckeller42/celloc/internal/wifiscan"
)

const sample = "BSS 00:11:22:33:44:55(on wlan0)\r\n" +
	"\tlast seen: 100 ms ago\r\n" +
	"\tsignal: -85.00 dBm\r\n" +
	"\tSSID: HomeNet\r\n" +
	"BSS aa:BB:cc:DD:ee:ff(on wlan0) -- associated\n" +
	"\tsignal: -42.50 dBm\n" +
	"\tSSID: minsel\n" +
	"BSS 12:34:56:78:9a:bc(on wlan0)\n" +
	"\tsignal: -70.00 dBm\n" +
	"\tSSID: cafe_nomap\n" +
	"BSS de:ad:be:ef:00:01(on wlan0)\n" +
	"\tsignal: -60.00 dBm\n" +
	"\tSSID: \n"

func TestParseScan(t *testing.T) {
	got := wifiscan.ParseScan(sample)
	want := []wifiscan.AP{
		{BSSID: "00:11:22:33:44:55", Signal: -85, SSID: "HomeNet"},
		{BSSID: "aa:bb:cc:dd:ee:ff", Signal: -42, SSID: "minsel"},
		{BSSID: "de:ad:be:ef:00:01", Signal: -60, SSID: ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseScan\n got=%#v\nwant=%#v", got, want)
	}
}

func TestParseScanMissingSignalIsWeak(t *testing.T) {
	in := "BSS 00:11:22:33:44:55(on wlan0)\n\tSSID: nosig\n"
	aps := wifiscan.ParseScan(in)
	if len(aps) != 1 || aps[0].Signal != -127 {
		t.Fatalf("missing signal should be weak sentinel, got %#v", aps)
	}
}

func TestParseScanEmptyAndGarbage(t *testing.T) {
	if aps := wifiscan.ParseScan(""); len(aps) != 0 {
		t.Fatalf("empty: got %v", aps)
	}
	if aps := wifiscan.ParseScan("no bss here\nsignal: -1 dBm\n"); len(aps) != 0 {
		t.Fatalf("garbage: got %v", aps)
	}
}
