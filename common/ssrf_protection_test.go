package common

import (
	"net"
	"testing"
)

func newBlacklistSSRFProtection() *SSRFProtection {
	return &SSRFProtection{
		AllowPrivateIp:   false,
		DomainFilterMode: false,
		IpFilterMode:     false,
	}
}

func TestSSRFProtectionRejectsSpecialPurposeAddresses(t *testing.T) {
	protection := newBlacklistSSRFProtection()
	tests := []struct {
		name string
		ip   string
	}{
		{name: "IPv4 unspecified", ip: "0.0.0.0"},
		{name: "carrier grade NAT", ip: "100.64.0.1"},
		{name: "cloud metadata in carrier grade NAT", ip: "100.100.100.200"},
		{name: "IETF protocol assignments", ip: "192.0.0.1"},
		{name: "documentation TEST-NET-1", ip: "192.0.2.1"},
		{name: "benchmarking", ip: "198.18.0.1"},
		{name: "documentation TEST-NET-2", ip: "198.51.100.1"},
		{name: "documentation TEST-NET-3", ip: "203.0.113.1"},
		{name: "IPv6 unspecified", ip: "::"},
		{name: "IPv4 compatible IPv6", ip: "::2"},
		{name: "IPv6 discard-only", ip: "100::1"},
		{name: "IPv6 dummy prefix", ip: "100:0:0:1::1"},
		{name: "IPv6 documentation", ip: "2001:db8::1"},
		{name: "IPv6 IANA reserved 2d00", ip: "2d00::1"},
		{name: "IPv6 IANA reserved 3000", ip: "3000::1"},
		{name: "returned 6bone space", ip: "3ffe::1"},
		{name: "IPv6 direct delegation AS112", ip: "2620:4f:8000::1"},
		{name: "IPv6 reserved beyond global unicast allocation", ip: "4000::1"},
		{name: "deprecated IPv6 site local", ip: "fec0::1"},
		{name: "IPv4 mapped carrier grade NAT", ip: "::ffff:100.64.0.1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ip := net.ParseIP(test.ip)
			if ip == nil {
				t.Fatalf("test address %q did not parse", test.ip)
			}
			if protection.IsIPAccessAllowed(ip) {
				t.Fatalf("expected special-purpose address %s to be rejected", test.ip)
			}
		})
	}
}

func TestSSRFProtectionAllowsAllocatedPublicIPv6(t *testing.T) {
	protection := newBlacklistSSRFProtection()
	if ip := net.ParseIP("2606:4700:4700::1111"); !protection.IsIPAccessAllowed(ip) {
		t.Fatal("expected allocated public IPv6 address to be allowed")
	}
}

func TestSSRFProtectionNormalizesTrailingDotForDomainBlacklist(t *testing.T) {
	protection := newBlacklistSSRFProtection()
	protection.DomainList = []string{"example.com"}

	if err := protection.ValidateNetworkTarget("EXAMPLE.COM.", 443); err == nil {
		t.Fatal("expected terminal-dot variant of blacklisted domain to be rejected")
	}
}

func TestSSRFProtectionNormalizesIDNAForDomainMatching(t *testing.T) {
	protection := newBlacklistSSRFProtection()
	protection.DomainList = []string{"bücher.example."}

	if err := protection.ValidateNetworkTarget("xn--bcher-kva.example", 443); err == nil {
		t.Fatal("expected Unicode blacklist entry to match its IDNA ASCII hostname")
	}
}
