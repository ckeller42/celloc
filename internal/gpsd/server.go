package gpsd

import (
	"bufio"
	"context"
	"net"
	"strings"
	"time"

	"github.com/ckeller42/celloc/internal/source"
)

// Provider returns the current best fix for streaming to gpsd clients.
type Provider func() source.Fix

// Server speaks the gpsd JSON protocol over TCP. It is intentionally minimal:
// VERSION on connect, ?WATCH/?POLL/?VERSION/?DEVICES, and a periodic TPV/SKY
// stream to watching clients.
type Server struct {
	Device   string        // device path reported to clients, e.g. "cell0"
	Provider Provider      // current fix source
	Interval time.Duration // stream cadence for watching clients
	Release  string        // VERSION release string
}

func (s *Server) device() string {
	if s.Device == "" {
		return "cell0"
	}
	return s.Device
}

// ListenAndServe binds addr and serves until ctx is cancelled.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return s.Serve(ctx, ln)
}

// Serve accepts connections on ln until ctx is cancelled (then ln is closed).
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return err
		}
		go s.handleConn(ctx, conn)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer func() { _ = conn.Close() }()

	if !s.write(conn, s.version()) {
		return
	}

	done := make(chan struct{})
	defer close(done)
	cmds := make(chan string)
	go func() {
		sc := bufio.NewScanner(conn)
		for sc.Scan() {
			select {
			case cmds <- sc.Text():
			case <-done:
				return
			}
		}
		close(cmds)
	}()

	interval := s.Interval
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	watching := false
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-cmds:
			if !ok {
				return
			}
			if !s.handleCmd(conn, line, &watching) {
				return
			}
		case <-ticker.C:
			if watching && !s.sendFix(conn) {
				return
			}
		}
	}
}

// handleCmd processes one client command line. Returns false on write failure.
func (s *Server) handleCmd(conn net.Conn, line string, watching *bool) bool {
	cmd := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ";"))
	switch {
	case strings.HasPrefix(cmd, "?WATCH"):
		enable := true
		if i := strings.IndexByte(cmd, '{'); i >= 0 {
			// tolerate {"enable":false,...}
			enable = !strings.Contains(strings.ReplaceAll(cmd[i:], " ", ""), `"enable":false`)
		}
		*watching = enable
		if !s.write(conn, s.devices()) || !s.write(conn, Watch{Class: "WATCH", Enable: enable, JSON: true}) {
			return false
		}
		if enable {
			return s.sendFix(conn)
		}
		return true
	case strings.HasPrefix(cmd, "?POLL"):
		return s.write(conn, s.poll())
	case strings.HasPrefix(cmd, "?VERSION"):
		return s.write(conn, s.version())
	case strings.HasPrefix(cmd, "?DEVICES"):
		return s.write(conn, s.devices())
	default:
		return true // unknown command: ignore (gpsd behavior)
	}
}

func (s *Server) sendFix(conn net.Conn) bool {
	f := s.Provider()
	return s.write(conn, TPVFromFix(f, s.device())) && s.write(conn, SKYEmpty(s.device()))
}

func (s *Server) version() Version {
	rel := s.Release
	if rel == "" {
		rel = "celloc"
	}
	return Version{Class: "VERSION", Release: rel, ProtoMajor: ProtoMajor, ProtoMinor: ProtoMinor}
}

func (s *Server) devices() Devices {
	return Devices{Class: "DEVICES", Devices: []Device{{
		Class: "DEVICE", Path: s.device(), Driver: "celloc",
		Activated: time.Now().UTC().Format(timeFormat),
	}}}
}

func (s *Server) poll() Poll {
	f := s.Provider()
	return Poll{
		Class: "POLL", Time: time.Now().UTC().Format(timeFormat), Active: 1,
		TPV: []TPV{TPVFromFix(f, s.device())},
		SKY: []SKY{SKYEmpty(s.device())},
	}
}

func (s *Server) write(conn net.Conn, v any) bool {
	b, err := MarshalLine(v)
	if err != nil {
		return false
	}
	_, err = conn.Write(b)
	return err == nil
}
