package opencellid_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ckeller42/celloc/internal/opencellid"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(r *http.Request) (*http.Response, error) { return f(r) }

func resp(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}
}

func TestLookupBuildsRequestAndParses(t *testing.T) {
	var gotURL *url.URL
	c := &opencellid.Client{Key: "pk.test", HTTP: doerFunc(func(r *http.Request) (*http.Response, error) {
		gotURL = r.URL
		return resp(200, `{"lat":48.7698,"lon":9.1676,"range":1548}`), nil
	})}
	loc, st, err := c.Lookup(context.Background(), opencellid.Query{
		MCC: 262, MNC: 3, LAC: 59621, CellID: 23612222, Radio: "LTE",
	})
	if err != nil || st != opencellid.StatusOK {
		t.Fatalf("st=%v err=%v", st, err)
	}
	if loc.Lat != 48.7698 || loc.Range != 1548 {
		t.Fatalf("loc=%+v", loc)
	}
	q := gotURL.Query()
	for k, want := range map[string]string{
		"key": "pk.test", "mcc": "262", "mnc": "3", "lac": "59621",
		"cellid": "23612222", "radio": "LTE", "format": "json",
	} {
		if q.Get(k) != want {
			t.Fatalf("query %s=%q want %q", k, q.Get(k), want)
		}
	}
	if !strings.HasSuffix(gotURL.Path, "/cell/get") {
		t.Fatalf("path=%q", gotURL.Path)
	}
}

func TestLookupNetworkError(t *testing.T) {
	c := &opencellid.Client{HTTP: doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial timeout")
	})}
	if _, _, err := c.Lookup(context.Background(), opencellid.Query{}); err == nil {
		t.Fatal("want error on transport failure")
	}
}

func TestLookupUnknownCell(t *testing.T) {
	c := &opencellid.Client{HTTP: doerFunc(func(*http.Request) (*http.Response, error) {
		return resp(200, `{"lat":0,"lon":0}`), nil
	})}
	_, st, err := c.Lookup(context.Background(), opencellid.Query{})
	if err != nil || st != opencellid.StatusUnknownCell {
		t.Fatalf("st=%v err=%v", st, err)
	}
}
