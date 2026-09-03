// Package httpfetch is a tiny shared GET-and-read helper for the two
// packages that download over plain HTTP (the BBS index JSON and BBS cart
// art) — same timeout/status/body-read shape, one place to change it.
package httpfetch

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Transport is the RoundTripper Get uses. Tests that spin up an
// httptest.NewTLSServer can point this at srv.Client().Transport so the
// test server's certificate is trusted, without loosening the https-only
// check that production traffic actually relies on.
var Transport http.RoundTripper = http.DefaultTransport

// maxBody caps how much of a response we'll read. Both callers only ever
// expect a JSON index or a single cart-cover PNG (real ones run tens of KB),
// so this is generous headroom, not a real limit — it exists purely so a
// compromised or misbehaving server can't hand back an unbounded stream and
// run the launcher out of memory.
const maxBody = 32 << 20 // 32 MiB

// Get fetches url with the given timeout and returns the response body.
// Returns an error on a non-https URL, a network failure, a non-200 status,
// or a body larger than maxBody.
func Get(url string, timeout time.Duration) ([]byte, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, errors.New("refusing non-https url")
	}
	client := &http.Client{
		Timeout:   timeout,
		Transport: Transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if req.URL.Scheme != "https" {
				return errors.New("refusing redirect to non-https url")
			}
			return nil
		},
	}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", http.StatusText(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBody)
	}
	return body, nil
}
