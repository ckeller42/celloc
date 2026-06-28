package influx_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ckeller42/celloc/internal/influx"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func TestWriterPostsLineProtocol(t *testing.T) {
	var got *http.Request
	var body string
	w := &influx.Writer{
		URL: "http://localhost:8086/", Org: "home", Bucket: "buspi", Token: "tok",
		HTTP: doerFunc(func(r *http.Request) (*http.Response, error) {
			got = r
			b, _ := io.ReadAll(r.Body)
			body = string(b)
			return &http.Response{StatusCode: 204, Body: io.NopCloser(strings.NewReader(""))}, nil
		}),
	}
	line := "geo,source=cell,radio=LTE lat=48.7,lon=9.1,range_m=1500i,mcc=262i,mnc=3i,cid=1i,tac=1i"
	if err := w.Write(context.Background(), line); err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPost {
		t.Fatalf("method %s", got.Method)
	}
	q := got.URL.Query()
	if q.Get("org") != "home" || q.Get("bucket") != "buspi" || q.Get("precision") != "s" {
		t.Fatalf("query: %v", q)
	}
	if got.Header.Get("Authorization") != "Token tok" {
		t.Fatalf("auth: %q", got.Header.Get("Authorization"))
	}
	if body != line {
		t.Fatalf("body=%q", body)
	}
}

func TestWriterNon2xxIsError(t *testing.T) {
	w := &influx.Writer{
		URL: "http://x", Org: "home", Bucket: "buspi", Token: "t",
		HTTP: doerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 401, Body: io.NopCloser(strings.NewReader("unauthorized"))}, nil
		}),
	}
	if err := w.Write(context.Background(), "geo x=1i"); err == nil {
		t.Fatal("want error on 401")
	}
}
