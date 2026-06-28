package gpsd

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
)

// Client is a minimal gpsd client: connect, enable watching, read TPV reports.
type Client struct {
	conn net.Conn
	r    *bufio.Reader
}

// Dial connects to a gpsd server (e.g. the router's geolocd on :2947).
func Dial(ctx context.Context, addr string) (*Client, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn, r: bufio.NewReader(conn)}, nil
}

// Watch enables JSON watch mode so the server streams TPV/SKY reports.
func (c *Client) Watch() error {
	_, err := c.conn.Write([]byte(`?WATCH={"enable":true,"json":true};` + "\n"))
	return err
}

// ReadTPV reads and discards reports until a TPV arrives, then returns it.
// Returns the underlying read error (e.g. on Close) so callers can reconnect.
func (c *Client) ReadTPV() (TPV, error) {
	for {
		line, err := c.r.ReadBytes('\n')
		if err != nil {
			return TPV{}, err
		}
		s := bytes.TrimSpace(line)
		if len(s) == 0 {
			continue
		}
		var probe struct {
			Class string `json:"class"`
		}
		if json.Unmarshal(s, &probe) != nil || probe.Class != "TPV" {
			continue
		}
		var t TPV
		if json.Unmarshal(s, &t) != nil {
			continue
		}
		return t, nil
	}
}

// Close closes the connection (also unblocks a pending ReadTPV).
func (c *Client) Close() error { return c.conn.Close() }
