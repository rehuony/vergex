package service

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// silentRelay forwards TCP to a target, and can stop forwarding the connections it has without
// closing them. That is what a dropped VPN or NAT flow looks like from both ends. Connections
// made after that still get through.
type silentRelay struct {
	listener   net.Listener
	generation atomic.Int64
	accepted   atomic.Int64
	mu         sync.Mutex
	conns      []net.Conn
}

func startSilentRelay(t *testing.T, target string) *silentRelay {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	r := &silentRelay{listener: listener}
	t.Cleanup(func() {
		listener.Close()
		r.mu.Lock()
		defer r.mu.Unlock()
		for _, conn := range r.conns {
			conn.Close()
		}
	})
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			server, err := net.Dial("tcp", target)
			if err != nil {
				client.Close()
				continue
			}
			r.accepted.Add(1)
			r.mu.Lock()
			r.conns = append(r.conns, client, server)
			r.mu.Unlock()
			generation := r.generation.Load()
			go r.forward(server, client, generation)
			go r.forward(client, server, generation)
		}
	}()
	return r
}

func (r *silentRelay) forward(dst io.Writer, src io.Reader, generation int64) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 && r.generation.Load() == generation {
			if _, err := dst.Write(buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (r *silentRelay) silence() { r.generation.Add(1) }

func TestVenueTransportDropsASilentConnection(t *testing.T) {
	backend := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "{}")
	}))
	backend.EnableHTTP2 = true
	backend.StartTLS()
	defer backend.Close()
	relay := startSilentRelay(t, backend.Listener.Addr().String())

	const ping = 100 * time.Millisecond
	transport := VenueTransport.Clone()
	defer transport.CloseIdleConnections()
	transport.TLSClientConfig = backend.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	transport.HTTP2.SendPingTimeout = ping
	transport.HTTP2.PingTimeout = ping
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}

	get := func() error {
		resp, err := client.Get("https://" + relay.listener.Addr().String())
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.ProtoMajor != 2 {
			t.Fatalf("got %s, this test is about HTTP/2", resp.Proto)
		}
		_, err = io.Copy(io.Discard, resp.Body)
		return err
	}

	// A connection that answers its pings is kept.
	if err := get(); err != nil {
		t.Fatalf("first request: %v", err)
	}
	time.Sleep(5 * ping)
	if err := get(); err != nil {
		t.Fatalf("request after pings: %v", err)
	}
	if got := relay.accepted.Load(); got != 1 {
		t.Fatalf("opened %d connections, a healthy one should be kept", got)
	}

	// A request caught on the dead connection fails when the ping goes unanswered, not when
	// its own timeout runs out, and the next one gets through on a new connection.
	relay.silence()
	err := get()
	if err == nil {
		t.Fatal("a request on the silent connection should fail")
	}
	if os.IsTimeout(err) {
		t.Fatalf("the request waited for its own timeout: %v", err)
	}
	if err := get(); err != nil {
		t.Fatalf("request after the connection was dropped: %v", err)
	}
	if got := relay.accepted.Load(); got != 2 {
		t.Fatalf("opened %d connections, want one new one", got)
	}
}
