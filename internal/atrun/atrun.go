// Package atrun runs modem AT commands on the router. The Runner interface is
// satisfied by gl_modem and ubus implementations; both take an injectable Exec
// so the command wiring is unit-testable without a modem.
package atrun

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes a single AT command and returns the raw modem response.
type Runner interface {
	Run(ctx context.Context, atCmd string) (string, error)
}

// Exec runs an external command and returns its combined stdout. Injected so
// tests can stub the subprocess.
type Exec func(ctx context.Context, name string, args ...string) ([]byte, error)

// OSExec is the production Exec backed by os/exec.
func OSExec(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// GlModem runs `gl_modem -B <bus> AT '<atCmd>'` (the GL-iNet helper).
type GlModem struct {
	Bus  string
	Exec Exec
}

// Run implements Runner.
func (g GlModem) Run(ctx context.Context, atCmd string) (string, error) {
	out, err := g.Exec(ctx, "gl_modem", "-B", g.Bus, "AT", atCmd)
	if err != nil {
		return "", fmt.Errorf("gl_modem: %w", err)
	}
	return string(out), nil
}

// Ubus runs `ubus call modem.<BUS>.AT get_result_AT '{"cmd":..,"timeout":5}'`
// and extracts the AT text from the JSON reply.
type Ubus struct {
	Bus  string // bus name, any case; the ubus object is modem.<BUS>.AT (upper)
	Exec Exec
}

// Run implements Runner. The ubus object name uppercases the bus (modem.CPU.AT)
// while gl_modem expects it lowercased (-B cpu), so a single uci `bus` value
// (default "cpu") works for both runners.
func (u Ubus) Run(ctx context.Context, atCmd string) (string, error) {
	obj := "modem." + strings.ToUpper(u.Bus) + ".AT"
	payload, _ := json.Marshal(map[string]any{"cmd": atCmd, "timeout": 5})
	out, err := u.Exec(ctx, "ubus", "call", obj, "get_result_AT", string(payload))
	if err != nil {
		return "", fmt.Errorf("ubus %s: %w", obj, err)
	}
	return extractUbusAT(out), nil
}

// extractUbusAT pulls the AT text out of the ubus JSON reply, tolerating the
// field-name variations seen across firmwares ("result"/"data"/"value"). Falls
// back to the raw body so qeng can still scan it for +QENG lines.
func extractUbusAT(out []byte) string {
	var m map[string]any
	if err := json.Unmarshal(out, &m); err != nil {
		return string(out)
	}
	for _, k := range []string{"result", "data", "value", "AT", "at"} {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return string(out)
}

// New returns the Runner named by runner ("glmodem" or "ubus") for bus, using
// OSExec. Unknown names default to gl_modem.
func New(runner, bus string) Runner {
	if runner == "ubus" {
		return Ubus{Bus: bus, Exec: OSExec}
	}
	return GlModem{Bus: bus, Exec: OSExec}
}
