package wifiscan_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ckeller42/celloc/internal/wifiscan"
)

func TestScannerArgsAndParse(t *testing.T) {
	var gotArgs []string
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte("BSS 00:11:22:33:44:55(on wlan0)\n\tsignal: -50.00 dBm\n\tSSID: a\n"), nil
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan0"}, Exec: exec}
	aps, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"iw", "dev", "wlan0", "scan"}
	if len(gotArgs) != 4 || gotArgs[0] != want[0] || gotArgs[2] != want[2] || gotArgs[3] != want[3] {
		t.Fatalf("args=%v want %v", gotArgs, want)
	}
	if len(aps) != 1 || aps[0].BSSID != "00:11:22:33:44:55" {
		t.Fatalf("aps=%v", aps)
	}
}

func TestScannerMergeDedupKeepsStrongest(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		calls++
		if args[1] == "wlan0" {
			return []byte("BSS aa:aa:aa:aa:aa:aa(on wlan0)\n\tsignal: -80.00 dBm\n\tSSID: x\n"), nil
		}
		return []byte("BSS AA:AA:AA:AA:AA:AA(on wlan1)\n\tsignal: -40.00 dBm\n\tSSID: x\n" +
			"BSS bb:bb:bb:bb:bb:bb(on wlan1)\n\tsignal: -55.00 dBm\n\tSSID: y\n"), nil
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan0", "wlan1"}, Exec: exec}
	aps, err := s.Scan(context.Background())
	if err != nil || calls != 2 {
		t.Fatalf("err=%v calls=%d", err, calls)
	}
	got := map[string]int{}
	for _, a := range aps {
		got[a.BSSID] = a.Signal
	}
	if got["aa:aa:aa:aa:aa:aa"] != -40 { // strongest of -80/-40 kept
		t.Fatalf("dedup kept wrong signal: %v", got)
	}
	if len(aps) != 2 {
		t.Fatalf("want 2 unique APs, got %v", aps)
	}
}

func TestScannerDedupPrefersRealSignalOverMissing(t *testing.T) {
	exec := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[1] == "wlan0" {
			return []byte("BSS aa:aa:aa:aa:aa:aa(on wlan0)\n\tSSID: x\n"), nil // no signal -> -127
		}
		return []byte("BSS AA:AA:AA:AA:AA:AA(on wlan1)\n\tsignal: -55.00 dBm\n\tSSID: x\n"), nil
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan0", "wlan1"}, Exec: exec}
	aps, err := s.Scan(context.Background())
	if err != nil || len(aps) != 1 || aps[0].Signal != -55 {
		t.Fatalf("dedup should keep the real -55 reading, got %#v err=%v", aps, err)
	}
}

func TestScannerErrorWhenAllIfacesFailAndNoAPs(t *testing.T) {
	exec := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errors.New("iw: No such device")
	}
	s := wifiscan.Scanner{Ifaces: []string{"wlan9"}, Exec: exec}
	if _, err := s.Scan(context.Background()); err == nil {
		t.Fatal("want error when every iface scan fails")
	}
}
