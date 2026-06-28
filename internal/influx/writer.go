package influx

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Doer is the subset of *http.Client used by Writer; injected for tests.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Writer posts line-protocol points to an InfluxDB v2 /api/v2/write endpoint.
type Writer struct {
	URL    string // base, e.g. http://localhost:8086
	Org    string
	Bucket string
	Token  string
	HTTP   Doer
}

// Write sends a single line-protocol record (second precision).
func (w *Writer) Write(ctx context.Context, line string) error {
	endpoint := strings.TrimRight(w.URL, "/") + "/api/v2/write?" + url.Values{
		"org":       {w.Org},
		"bucket":    {w.Bucket},
		"precision": {"s"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(line))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Token "+w.Token)
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("influx write: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("influx write: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}
