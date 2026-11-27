package rt

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"sync"
	"syscall"
	"time"
)

// PlaygroundRequestTimeout is the per-request deadline in the hosted
// playground (Requirement 15.4.1). Guard enforces it; the host's watchdog
// backs it up one second later.
const PlaygroundRequestTimeout = 5 * time.Second

// In the playground (RIP_PLAYGROUND=1) every FETCH goes through the SSRF
// guard (Requirement 11.2, security.md). The check runs in net.Dialer.Control,
// so it sees the IP actually being connected to: after DNS resolution, on
// every redirect, and on every new connection, which also defeats DNS
// rebinding. The 1 MB response cap is in Fetch itself.
func init() {
	if isPlayground() {
		fetchDialer = playgroundTransport
	}
}

var playgroundTransport = sync.OnceValue(func() http.RoundTripper {
	return guardedTransport(checkPublicAddr)
})

// applyPlaygroundDeadline gives a request's Ctx the deadline that Guard
// checks. It does nothing outside the playground.
//
// TODO(task 5.3): call it from App.serveRequest in rt/app.go, right after
// NewCtx (one line, landed at merge time because app.go belongs to another
// track). Until then the host's watchdog is what answers 508.
func applyPlaygroundDeadline(c *Ctx) {
	if isPlayground() {
		c.SetDeadline(time.Now().Add(PlaygroundRequestTimeout))
	}
}

// guardedTransport returns a transport whose every connection is vetted by
// check(network, "ip:port") before it is made.
func guardedTransport(check func(network, address string) error) *http.Transport {
	d := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			return check(network, address)
		},
	}
	return &http.Transport{
		// Never use a proxy: it would make the real connection for us, unchecked.
		Proxy:                  nil,
		DialContext:            d.DialContext,
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           16,
		IdleConnTimeout:        90 * time.Second,
		TLSHandshakeTimeout:    10 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
}

// checkPublicAddr allows a TCP connection only to a public unicast address.
// Anything it cannot parse is refused (fail closed).
func checkPublicAddr(network, address string) error {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return fmt.Errorf("FETCH cannot use %s connections in the playground", network)
	}
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("FETCH cannot reach %s from the playground — it is not an IP address", address)
	}
	if ip := ap.Addr().WithZone("").Unmap(); blockedIP(ip) {
		return fmt.Errorf("FETCH cannot reach %s from the playground — private, loopback, link-local and cloud metadata addresses are blocked", ip)
	}
	return nil
}

// blockedPrefixes are the ranges that netip's Is* methods don't cover.
var blockedPrefixes = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8",          // "this network"
		"100.64.0.0/10",      // carrier-grade NAT (also Alibaba Cloud metadata, 100.100.100.200)
		"192.0.0.0/24",       // IETF protocol assignments
		"192.0.2.0/24",       // documentation
		"198.18.0.0/15",      // benchmarking
		"198.51.100.0/24",    // documentation
		"203.0.113.0/24",     // documentation
		"240.0.0.0/4",        // reserved, and 255.255.255.255
		"168.63.129.16/32",   // Azure host endpoint
		"::/96",              // IPv4-compatible (deprecated)
		"64:ff9b::/96",       // NAT64, which can embed a private IPv4 address
		"64:ff9b:1::/48",     // local-use NAT64
		"100::/64",           // discard-only
		"2001:db8::/32",      // documentation
		"2002::/16",          // 6to4, which can embed a private IPv4 address
		"fec0::/10",          // site-local (deprecated)
		"fd00:ec2::254/128",  // AWS metadata over IPv6 (already in fc00::/7)
		"169.254.169.254/32", // cloud metadata (already link-local)
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

// blockedIP reports whether FETCH must not connect to ip in the playground.
func blockedIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
