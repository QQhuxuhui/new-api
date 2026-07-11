package service

import (
	"context"
	"net"
	"testing"

	"github.com/QuantumNous/new-api/common"
)

// fakeResolver returns a fixed set of IPs regardless of the host, simulating a
// DNS response (including a malicious rebinding response pointing at a private IP).
type fakeResolver struct {
	ips []net.IPAddr
}

func (r *fakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	return r.ips, nil
}

func blacklistProtection() *common.SSRFProtection {
	// blacklist mode with empty lists => any public domain passes name checks,
	// but resolved private IPs are still rejected (AllowPrivateIp=false).
	return &common.SSRFProtection{
		AllowPrivateIp:         false,
		DomainFilterMode:       false, // blacklist
		DomainList:             []string{},
		IpFilterMode:           false, // blacklist
		IpList:                 []string{},
		AllowedPorts:           []int{},
		ApplyIPFilterForDomain: true, // enable resolved-IP validation at dial time
	}
}

// TestProtectedFetchDialerBlocksDNSRebinding is the core guarantee: a domain that
// passes the pre-dial name checks but resolves to a private IP is refused at dial
// time, and the underlying dialer is never invoked.
func TestProtectedFetchDialerBlocksDNSRebinding(t *testing.T) {
	dialCalled := false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return blacklistProtection(), true, nil
		},
	}

	_, err := dialer.DialContext(context.Background(), "tcp", "evil.example.com:443")
	if err == nil {
		t.Fatalf("expected dial to be blocked for domain resolving to private IP")
	}
	if dialCalled {
		t.Errorf("underlying dialer must NOT be called when resolved IP is blocked")
	}
}

// TestProtectedFetchDialerAllowsPublicIP confirms a public resolved IP dials through.
func TestProtectedFetchDialerAllowsPublicIP(t *testing.T) {
	dialCalled := false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}}, // example.com public IP
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return blacklistProtection(), true, nil
		},
	}

	if _, err := dialer.DialContext(context.Background(), "tcp", "example.com:443"); err != nil {
		t.Fatalf("expected public IP to dial through, got %v", err)
	}
	if !dialCalled {
		t.Errorf("underlying dialer should be called for an allowed public IP")
	}
}

// TestProtectedFetchDialerBlocksLiteralPrivateIP covers a literal private IP target
// (no DNS): it must be rejected by ValidateNetworkTarget before any dial.
func TestProtectedFetchDialerBlocksLiteralPrivateIP(t *testing.T) {
	dialCalled := false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return blacklistProtection(), true, nil
		},
	}

	if _, err := dialer.DialContext(context.Background(), "tcp", "169.254.169.254:80"); err == nil {
		t.Fatalf("expected literal link-local IP (cloud metadata) to be blocked")
	}
	if dialCalled {
		t.Errorf("underlying dialer must NOT be called for a blocked literal IP")
	}
}

// TestProtectedFetchDialerBypassWhenDisabled confirms that when SSRF protection is
// disabled the dialer passes straight through.
func TestProtectedFetchDialerBypassWhenDisabled(t *testing.T) {
	dialCalled := false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return nil, false, nil // disabled
		},
	}

	if _, err := dialer.DialContext(context.Background(), "tcp", "10.0.0.1:80"); err != nil {
		t.Fatalf("expected passthrough when protection disabled, got %v", err)
	}
	if !dialCalled {
		t.Errorf("underlying dialer should be called when protection is disabled")
	}
}
