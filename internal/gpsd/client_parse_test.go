package gpsd

import (
	"bufio"
	"strings"
	"testing"
)

// White-box tests for ReadTPV's line handling without a network: skip non-TPV
// and garbage lines, tolerate CRLF framing, and surface EOF as an error.
func TestReadTPVSkipsNoiseAndHandlesCRLF(t *testing.T) {
	input := strings.Join([]string{
		`{"class":"VERSION","release":"x"}`,                        // non-TPV: skip
		`not json at all`,                                          // garbage: skip
		`{"class":"SKY","satellites":[]}`,                          // non-TPV: skip
		"{\"class\":\"TPV\",\"mode\":2,\"lat\":1.5,\"lon\":2.5}\r", // CRLF-terminated TPV
	}, "\n") + "\n"

	c := &Client{r: bufio.NewReader(strings.NewReader(input))}
	tpv, err := c.ReadTPV()
	if err != nil {
		t.Fatal(err)
	}
	if tpv.Mode != 2 || tpv.Lat == nil || *tpv.Lat != 1.5 {
		t.Fatalf("bad tpv: %+v", tpv)
	}
}

func TestReadTPVReturnsErrorOnDisconnect(t *testing.T) {
	// Only noise then EOF -> ReadTPV must return the read error, not hang.
	c := &Client{r: bufio.NewReader(strings.NewReader(`{"class":"VERSION"}` + "\n"))}
	if _, err := c.ReadTPV(); err == nil {
		t.Fatal("want error when stream ends before any TPV")
	}
}
