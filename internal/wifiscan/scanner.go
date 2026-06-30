package wifiscan

import (
	"context"
	"fmt"
	"os/exec"
)

// Exec runs an external command and returns its combined stdout. Injected so
// tests can stub the subprocess.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// OSExec is the production Exec backed by os/exec.
func OSExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// Scanner runs `iw dev <if> scan` for each interface and merges the results.
type Scanner struct {
	Ifaces []string
	Exec   Exec
}

// NewScanner builds a Scanner over ifaces using OSExec.
func NewScanner(ifaces []string) Scanner {
	return Scanner{Ifaces: ifaces, Exec: OSExec}
}

// Scan scans every interface and returns the merged AP set, de-duplicated by
// BSSID keeping the strongest signal. It returns an error only when every
// interface failed and no APs were collected.
func (s Scanner) Scan(ctx context.Context) ([]AP, error) {
	byBSSID := map[string]AP{}
	var order []string
	var firstErr error
	for _, iface := range s.Ifaces {
		out, err := s.Exec(ctx, "iw", "dev", iface, "scan")
		if err != nil {
			if firstErr == nil {
				firstErr = fmt.Errorf("iw dev %s scan: %w", iface, err)
			}
			continue
		}
		for _, ap := range ParseScan(string(out)) {
			if cur, ok := byBSSID[ap.BSSID]; !ok {
				byBSSID[ap.BSSID] = ap
				order = append(order, ap.BSSID)
			} else if ap.Signal > cur.Signal {
				byBSSID[ap.BSSID] = ap
			}
		}
	}
	if len(byBSSID) == 0 && firstErr != nil {
		return nil, firstErr
	}
	aps := make([]AP, 0, len(order))
	for _, b := range order {
		aps = append(aps, byBSSID[b])
	}
	return aps, nil
}
