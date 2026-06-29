package atrun_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ckeller42/celloc/internal/atrun"
)

func TestGlModemBuildsArgsAndReturnsOutput(t *testing.T) {
	var gotName string
	var gotArgs []string
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte(`+QENG: "LTE",...`), nil
	}
	g := atrun.GlModem{Bus: "cpu", Exec: exec}
	out, err := g.Run(context.Background(), `AT+QENG="servingcell"`)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "gl_modem" {
		t.Fatalf("cmd=%q", gotName)
	}
	want := []string{"-B", "cpu", "AT", `AT+QENG="servingcell"`}
	if len(gotArgs) != 4 || gotArgs[0] != want[0] || gotArgs[1] != want[1] || gotArgs[3] != want[3] {
		t.Fatalf("args=%v want %v", gotArgs, want)
	}
	if out != `+QENG: "LTE",...` {
		t.Fatalf("out=%q", out)
	}
}

func TestGlModemPropagatesError(t *testing.T) {
	exec := func(context.Context, string, ...string) ([]byte, error) {
		return nil, errors.New("modem busy")
	}
	if _, err := (atrun.GlModem{Bus: "cpu", Exec: exec}).Run(context.Background(), "AT"); err == nil {
		t.Fatal("want error")
	}
}

func TestUbusExtractsResultField(t *testing.T) {
	exec := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "ubus" || args[0] != "call" || args[1] != "modem.CPU.AT" {
			t.Fatalf("unexpected ubus call: %s %v", name, args)
		}
		return []byte(`{"result":"+QENG: \"LTE\",262"}`), nil
	}
	out, err := (atrun.Ubus{Bus: "CPU", Exec: exec}).Run(context.Background(), `AT+QENG="servingcell"`)
	if err != nil {
		t.Fatal(err)
	}
	if out != `+QENG: "LTE",262` {
		t.Fatalf("extracted=%q", out)
	}
}

func TestUbusUppercasesBusInObjectName(t *testing.T) {
	// uci default is bus 'cpu' (lowercase, correct for gl_modem -B cpu); the ubus
	// object must still be modem.CPU.AT.
	exec := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[1] != "modem.CPU.AT" {
			t.Fatalf("object=%q want modem.CPU.AT", args[1])
		}
		return []byte(`{"result":"+QENG: \"LTE\",262"}`), nil
	}
	if _, err := (atrun.Ubus{Bus: "cpu", Exec: exec}).Run(context.Background(), "AT"); err != nil {
		t.Fatal(err)
	}
}

func TestUbusFallsBackToRawOnUnknownShape(t *testing.T) {
	exec := func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"weird":123}`), nil
	}
	out, _ := (atrun.Ubus{Bus: "CPU", Exec: exec}).Run(context.Background(), "AT")
	if out != `{"weird":123}` {
		t.Fatalf("want raw fallback, got %q", out)
	}
}

func TestNewDefaultsToGlModem(t *testing.T) {
	if _, ok := atrun.New("anything", "cpu").(atrun.GlModem); !ok {
		t.Fatal("unknown runner should default to GlModem")
	}
	if _, ok := atrun.New("ubus", "CPU").(atrun.Ubus); !ok {
		t.Fatal("ubus runner expected")
	}
}
