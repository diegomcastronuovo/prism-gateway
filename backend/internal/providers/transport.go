package providers

import (
	"net/http"
	"time"
)

// newTransport returns a tuned http.Transport suitable for high-concurrency LLM provider calls.
// Using http.DefaultTransport (MaxIdleConnsPerHost=2) causes TLS handshake storms under load.
func newTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}
