package service

import (
	"net/http"
	"time"
)

// VenueTransport carries every REST request to a venue, for trading and public data alike,
// so they share one connection pool.
//
// Venues answer over HTTP/2, where all requests to a host share one connection and a request
// that times out leaves it open. If the path dies without a reset, as when a VPN or NAT drops
// the flow, nothing closes it: every request waits out its own timeout on it, while new
// connections work fine. So a connection that has gone quiet gets a ping, and one that leaves
// the ping unanswered is closed, failing what was waiting on it. net/http does not resend
// those requests, so an order on such a connection stays unconfirmed, as it does on a timeout.
var VenueTransport = newVenueTransport()

func newVenueTransport() *http.Transport {
	// Keeps the default's proxy from the environment, its dial and TLS timeouts and its
	// 90 second limit on idle connections.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.HTTP2 = &http.HTTP2Config{SendPingTimeout: 10 * time.Second, PingTimeout: 5 * time.Second}
	// On HTTP/1.1, which a proxy can force, this closes a connection that stopped answering.
	transport.ResponseHeaderTimeout = 20 * time.Second
	return transport
}
