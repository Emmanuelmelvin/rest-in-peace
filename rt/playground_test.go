package rt

import (
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockedIP(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.8.9.10", true},
		{"::1", true},
		{"10.0.0.5", true},
		{"172.16.3.4", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true}, // AWS/GCP/Azure metadata
		{"169.254.0.1", true},
		{"100.100.100.200", true}, // Alibaba metadata
		{"168.63.129.16", true},   // Azure host endpoint
		{"fd00:ec2::254", true},   // AWS metadata over IPv6
		{"fe80::1", true},
		{"fc00::1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"255.255.255.255", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
		{"64:ff9b::a00:1", true},   // NAT64 of 10.0.0.1
		{"2002:c0a8:101::1", true}, // 6to4 of 192.168.1.1
		{"::ffff:127.0.0.1", true}, // IPv4-mapped loopback
		{"::ffff:169.254.169.254", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"93.184.216.34", false},
		{"2606:4700:4700::1111", false},
	}
	for _, tc := range cases {
		ip := netip.MustParseAddr(tc.ip).Unmap()
		if got := blockedIP(ip); got != tc.blocked {
			t.Errorf("blockedIP(%s) = %v, want %v", tc.ip, got, tc.blocked)
		}
	}
}

func TestCheckPublicAddr(t *testing.T) {
	for _, tc := range []struct {
		network, addr string
		ok            bool
	}{
		{"tcp4", "8.8.8.8:443", true},
		{"tcp6", "[2606:4700:4700::1111]:443", true},
		{"tcp4", "127.0.0.1:8090", false},
		{"tcp6", "[::1]:8090", false},
		{"tcp6", "[fe80::1%eth0]:80", false},
		{"tcp6", "[::ffff:10.0.0.1]:80", false},
		{"tcp4", "localhost:80", false}, // not an IP: fail closed
		{"udp4", "8.8.8.8:53", false},
		{"unix", "/var/run/docker.sock", false},
	} {
		err := checkPublicAddr(tc.network, tc.addr)
		if (err == nil) != tc.ok {
			t.Errorf("checkPublicAddr(%s, %s) = %v, want ok=%v", tc.network, tc.addr, err, tc.ok)
		}
	}
}

func TestGuardedTransportBlocksLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer srv.Close()

	client := &http.Client{Transport: guardedTransport(checkPublicAddr), Timeout: 5 * time.Second}
	_, err := client.Get(srv.URL)
	if err == nil || !strings.Contains(err.Error(), "cannot reach 127.0.0.1") {
		t.Fatalf("want a blocked-address error, got %v", err)
	}
	// A hostname that resolves to loopback is checked after resolution.
	_, err = client.Get(strings.Replace(srv.URL, "127.0.0.1", "localhost", 1))
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("want localhost blocked after DNS resolution, got %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the blocked server saw %d requests", n)
	}
}

// TestGuardedTransportChecksRedirects allows the first server and blocks the
// second; a redirect from one to the other must not reach the second.
func TestGuardedTransportChecksRedirects(t *testing.T) {
	var forbiddenHits atomic.Int32
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { forbiddenHits.Add(1) }))
	defer forbidden.Close()
	open := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, forbidden.URL+"/secret", http.StatusFound)
	}))
	defer open.Close()

	port := forbidden.Listener.Addr().(*net.TCPAddr).Port
	check := func(network, address string) error {
		if int(netip.MustParseAddrPort(address).Port()) == port {
			return checkPublicAddr(network, address)
		}
		return nil
	}
	client := &http.Client{Transport: guardedTransport(check), Timeout: 5 * time.Second}
	_, err := client.Get(open.URL)
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("want the redirect target blocked, got %v", err)
	}
	if n := forbiddenHits.Load(); n != 0 {
		t.Fatalf("the redirect target saw %d requests", n)
	}
}

// TestFetchUsesPlaygroundGuard checks the wiring: with the hook set, FETCH
// to a loopback server raises a catchable ?HTTP ERROR.
func TestFetchUsesPlaygroundGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("the loopback server must not be reached")
	}))
	defer srv.Close()

	saved := fetchDialer
	fetchDialer = playgroundTransport
	defer func() { fetchDialer = saved }()

	_, err := catchFetch(newCtx(), srv.URL+"/admin")
	if err == nil || err.Kind != KindHTTP || !strings.Contains(err.Message, "cannot reach 127.0.0.1") {
		t.Fatalf("want ?HTTP ERROR for a loopback FETCH, got %v", err)
	}
}

func TestApplyPlaygroundDeadline(t *testing.T) {
	t.Setenv("RIP_PLAYGROUND", "")
	c := newCtx()
	applyPlaygroundDeadline(c)
	if !c.deadline.IsZero() {
		t.Fatal("no deadline outside the playground")
	}
	t.Setenv("RIP_PLAYGROUND", "1")
	applyPlaygroundDeadline(c)
	if d := time.Until(c.deadline); d <= 0 || d > PlaygroundRequestTimeout {
		t.Fatalf("deadline %v from now, want within %v", d, PlaygroundRequestTimeout)
	}
}

func TestFetchCapsResponseAt1MB(t *testing.T) {
	big := strings.Repeat("A", fetchMaxBody+4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(big))
	}))
	defer srv.Close()

	saved := fetchDialer
	fetchDialer = func() http.RoundTripper {
		return guardedTransport(func(_, _ string) error { return nil })
	}
	defer func() { fetchDialer = saved }()

	res, err := catchFetch(newCtx(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	body, _ := res.Get("body")
	s, ok := body.(string)
	if !ok {
		t.Fatalf("expected string body, got %T", body)
	}
	if len(s) != fetchMaxBody {
		t.Fatalf("body length = %d, want %d (1 MB cap)", len(s), fetchMaxBody)
	}
}
