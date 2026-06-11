package webfetch

import (
	"net"
	"syscall"

	"github.com/m-mizutani/goerr/v2"
)

// isBlockedIP reports whether ip falls in a range the webfetch tool must not
// reach. This is the SSRF guard: only public, global-unicast addresses are
// allowed so the agent cannot be steered (via untrusted Slack/case content)
// into hitting loopback, RFC1918/ULA private networks, or the cloud metadata
// endpoint (169.254.169.254, link-local).
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// Block Carrier-Grade NAT (CGNAT) 100.64.0.0/10 (RFC 6598). net.IP.IsPrivate
	// does not cover it, but overlay networks (e.g. Tailscale) route internal
	// hosts through this range, so it must not be reachable via the agent.
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] < 128 {
			return true
		}
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}

// safeDialControl is a net.Dialer.Control hook that rejects connections to
// blocked IP ranges. address is the already-resolved "ip:port" the dialer is
// about to connect to, so inspecting it here defeats DNS rebinding (the IP is
// the one actually dialed, not a re-resolved name) and covers every redirect
// hop, since each hop dials through the same Control.
func safeDialControl(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return goerr.Wrap(err, "failed to parse dial address", goerr.V("address", address))
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return goerr.New("dial address is not a literal IP", goerr.V("address", address))
	}
	if isBlockedIP(ip) {
		return goerr.New("connection to non-public IP is blocked by SSRF guard",
			goerr.V("network", network), goerr.V("ip", ip.String()))
	}
	return nil
}
