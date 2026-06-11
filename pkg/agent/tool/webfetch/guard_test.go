package webfetch_test

import (
	"testing"

	"github.com/m-mizutani/gt"
	"github.com/secmon-lab/hecatoncheires/pkg/agent/tool/webfetch"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		name    string
		ip      string
		blocked bool
	}{
		{name: "loopback v4", ip: "127.0.0.1", blocked: true},
		{name: "loopback v6", ip: "::1", blocked: true},
		{name: "private 10/8", ip: "10.1.2.3", blocked: true},
		{name: "private 172.16/12", ip: "172.16.0.1", blocked: true},
		{name: "private 192.168/16", ip: "192.168.1.1", blocked: true},
		{name: "ULA v6", ip: "fc00::1", blocked: true},
		{name: "link-local metadata", ip: "169.254.169.254", blocked: true},
		{name: "link-local v6", ip: "fe80::1", blocked: true},
		{name: "unspecified v4", ip: "0.0.0.0", blocked: true},
		{name: "unspecified v6", ip: "::", blocked: true},
		{name: "multicast v4", ip: "224.0.0.1", blocked: true},
		{name: "ipv4-mapped loopback", ip: "::ffff:127.0.0.1", blocked: true},
		{name: "cgnat low edge", ip: "100.64.0.1", blocked: true},
		{name: "cgnat high edge", ip: "100.127.255.255", blocked: true},
		{name: "just below cgnat", ip: "100.63.255.255", blocked: false},
		{name: "just above cgnat", ip: "100.128.0.0", blocked: false},
		{name: "public v4", ip: "8.8.8.8", blocked: false},
		{name: "public v4 cloudflare", ip: "1.1.1.1", blocked: false},
		{name: "public v6", ip: "2606:4700:4700::1111", blocked: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ip := webfetch.ParseIPForTest(c.ip)
			gt.Value(t, ip).NotNil().Required()
			if c.blocked {
				gt.Bool(t, webfetch.IsBlockedIPForTest(ip)).True()
			} else {
				gt.Bool(t, webfetch.IsBlockedIPForTest(ip)).False()
			}
		})
	}

	t.Run("nil IP is blocked", func(t *testing.T) {
		gt.Bool(t, webfetch.IsBlockedIPForTest(nil)).True()
	})
}
