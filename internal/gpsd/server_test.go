package gpsd_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/ckeller42/celloc/internal/gpsd"
	"github.com/ckeller42/celloc/internal/source"
)

func startServer(t *testing.T, provider gpsd.Provider) (string, context.CancelFunc) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	srv := &gpsd.Server{Device: "cell0", Provider: provider, Interval: 20 * time.Millisecond, Release: "celloc-test"}
	go func() { _ = srv.Serve(ctx, ln) }()
	return ln.Addr().String(), cancel
}

func readClass(t *testing.T, r *bufio.Reader) map[string]any {
	t.Helper()
	line, err := r.ReadString('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &m); err != nil {
		t.Fatalf("bad json %q: %v", line, err)
	}
	return m
}

func TestServerWatchHandshakeAndStream(t *testing.T) {
	fix := source.Fix{
		Time: time.Now().UTC(), Mode: 2, Lat: 48.7698, Lon: 9.1676,
		EPH: 1548, EPX: 1548, EPY: 1548, Source: "cell", Radio: "LTE",
		MCC: 262, MNC: 3, CID: 0x1684B3E, TAC: 0xE8E5,
	}
	addr, cancel := startServer(t, func() source.Fix { return fix })
	defer cancel()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)

	// VERSION on connect.
	if m := readClass(t, r); m["class"] != "VERSION" {
		t.Fatalf("first message=%v, want VERSION", m["class"])
	}
	// Send WATCH; expect DEVICES, WATCH, then a TPV.
	if _, err := conn.Write([]byte(`?WATCH={"enable":true,"json":true};` + "\n")); err != nil {
		t.Fatal(err)
	}
	if m := readClass(t, r); m["class"] != "DEVICES" {
		t.Fatalf("want DEVICES, got %v", m["class"])
	}
	if m := readClass(t, r); m["class"] != "WATCH" || m["enable"] != true {
		t.Fatalf("want WATCH enable, got %v", m)
	}
	// Next non-SKY message should be a TPV with our position.
	var tpv map[string]any
	for i := 0; i < 4; i++ {
		m := readClass(t, r)
		if m["class"] == "TPV" {
			tpv = m
			break
		}
	}
	if tpv == nil {
		t.Fatal("no TPV received")
	}
	if tpv["mode"].(float64) != 2 || tpv["lat"].(float64) != 48.7698 {
		t.Fatalf("bad TPV: %v", tpv)
	}
}

func TestServerCommands(t *testing.T) {
	addr, cancel := startServer(t, func() source.Fix { return source.Fix{Mode: 0} })
	defer cancel()
	conn, _ := net.Dial("tcp", addr)
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	readClass(t, r) // VERSION on connect

	cases := []struct {
		send string
		want string
	}{
		{"?VERSION;\n", "VERSION"},
		{"?DEVICES;\n", "DEVICES"},
	}
	for _, c := range cases {
		if _, err := conn.Write([]byte(c.send)); err != nil {
			t.Fatal(err)
		}
		if m := readClass(t, r); m["class"] != c.want {
			t.Fatalf("%q -> %v, want %v", c.send, m["class"], c.want)
		}
	}

	// Unknown command is ignored (no reply); a following ?VERSION still answers.
	if _, err := conn.Write([]byte("?GARBAGE;\n?VERSION;\n")); err != nil {
		t.Fatal(err)
	}
	if m := readClass(t, r); m["class"] != "VERSION" {
		t.Fatalf("unknown cmd not ignored: got %v", m["class"])
	}
}

func TestServerWatchDisable(t *testing.T) {
	addr, cancel := startServer(t, func() source.Fix { return source.Fix{Mode: 0} })
	defer cancel()
	conn, _ := net.Dial("tcp", addr)
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	readClass(t, r) // VERSION
	if _, err := conn.Write([]byte(`?WATCH={"enable":false};` + "\n")); err != nil {
		t.Fatal(err)
	}
	readClass(t, r) // DEVICES
	if m := readClass(t, r); m["class"] != "WATCH" || m["enable"] != false {
		t.Fatalf("want WATCH enable=false, got %v", m)
	}
}

func TestListenAndServeBindsAndStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	srv := &gpsd.Server{Device: "cell0", Provider: func() source.Fix { return source.Fix{Mode: 0} }, Interval: time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe(ctx, "127.0.0.1:0") }()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-errc:
		if err != nil {
			t.Fatalf("clean shutdown should be nil, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not stop on ctx cancel")
	}
}

func TestServerPoll(t *testing.T) {
	addr, cancel := startServer(t, func() source.Fix { return source.Fix{Mode: 0} })
	defer cancel()
	conn, _ := net.Dial("tcp", addr)
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	readClass(t, r) // VERSION
	if _, err := conn.Write([]byte("?POLL;\n")); err != nil {
		t.Fatal(err)
	}
	if m := readClass(t, r); m["class"] != "POLL" {
		t.Fatalf("want POLL, got %v", m["class"])
	}
}
