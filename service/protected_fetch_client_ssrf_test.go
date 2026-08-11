package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
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
	if !types.IsClientInputError(err) {
		t.Fatalf("DNS rebinding policy rejection must be a client-input error, got %v", err)
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

func TestProtectedFetchDialerBlocksPrivateDNSWithConfiguredIPFilterDisabled(t *testing.T) {
	dialCalled := false
	protection := blacklistProtection()
	protection.ApplyIPFilterForDomain = false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return protection, true, nil
		},
	}

	_, err := dialer.DialContext(context.Background(), "tcp", "evil.example.com:443")
	if err == nil {
		t.Fatal("expected private DNS result to be blocked even when configured IP filtering is disabled")
	}
	if dialCalled {
		t.Fatal("underlying dialer must not be called for a private DNS result")
	}
}

func TestProtectedFetchDialerPinsPublicDNSWhenConfiguredIPFilterDisabled(t *testing.T) {
	var dialAddress string
	protection := blacklistProtection()
	protection.IpFilterMode = true
	protection.IpList = []string{"1.1.1.1"}
	protection.ApplyIPFilterForDomain = false
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialAddress = address
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return protection, true, nil
		},
	}

	if _, err := dialer.DialContext(context.Background(), "tcp", "example.com:443"); err != nil {
		t.Fatalf("expected configured IP list to be ignored for a domain, got %v", err)
	}
	if dialAddress != "93.184.216.34:443" {
		t.Fatalf("expected connection to be pinned to validated DNS result, got %q", dialAddress)
	}
}

func TestProtectedFetchDialerAppliesConfiguredIPFilterWhenEnabledForDomain(t *testing.T) {
	dialCalled := false
	protection := blacklistProtection()
	protection.IpFilterMode = true
	protection.IpList = []string{"1.1.1.1"}
	protection.ApplyIPFilterForDomain = true
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalled = true
			return nil, nil
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			return protection, true, nil
		},
	}

	if _, err := dialer.DialContext(context.Background(), "tcp", "example.com:443"); err == nil {
		t.Fatal("expected configured IP whitelist to reject the resolved domain address")
	}
	if dialCalled {
		t.Fatal("underlying dialer must not be called for an address outside the configured whitelist")
	}
}

func TestProtectedFetchClientDoesNotUseEnvironmentProxy(t *testing.T) {
	client := newProtectedFetchHTTPClientWithDialer(nil, nil, nil)
	roundTripper, ok := client.Transport.(*ssrfProtectedRoundTripper)
	if !ok {
		t.Fatalf("unexpected protected transport type %T", client.Transport)
	}
	if roundTripper.proxy != nil {
		t.Fatal("protected client must not use an environment proxy")
	}
}

func TestProtectedFetchURLPreflightDoesNotResolveDNS(t *testing.T) {
	previousSetting := *system_setting.GetFetchSetting()
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowPrivateIp:       false,
		DomainFilterMode:     false,
		IpFilterMode:         false,
	}
	t.Cleanup(func() {
		*system_setting.GetFetchSetting() = previousSetting
	})

	if err := ValidateSSRFProtectedFetchURL("https://does-not-resolve.invalid/path"); err != nil {
		t.Fatalf("protected URL preflight must leave DNS resolution to the dialer: %v", err)
	}
}

func TestProtectedFetchDialerCapsCandidateAttempts(t *testing.T) {
	const expectedCandidateLimit = 8
	resolved := make([]net.IPAddr, 20)
	for index := range resolved {
		resolved[index] = net.IPAddr{IP: net.IPv4(93, 184, 216, byte(index+1))}
	}

	dialAttempts := 0
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: resolved},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialAttempts++
			return nil, errors.New("dial failed")
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			protection := blacklistProtection()
			protection.ApplyIPFilterForDomain = false
			return protection, true, nil
		},
	}

	_, _ = dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if dialAttempts != expectedCandidateLimit {
		t.Fatalf("expected %d bounded dial attempts, got %d", expectedCandidateLimit, dialAttempts)
	}
}

func TestProtectedFetchDialerSharesOneTotalDeadline(t *testing.T) {
	var deadlines []time.Time
	dialer := &protectedFetchDialer{
		resolver: &fakeResolver{ips: []net.IPAddr{
			{IP: net.ParseIP("93.184.216.34")},
			{IP: net.ParseIP("93.184.216.35")},
		}},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected every candidate dial to share a total deadline")
			}
			deadlines = append(deadlines, deadline)
			return nil, errors.New("dial failed")
		},
		getProtection: func() (*common.SSRFProtection, bool, error) {
			protection := blacklistProtection()
			protection.ApplyIPFilterForDomain = false
			return protection, true, nil
		},
	}

	_, _ = dialer.DialContext(context.Background(), "tcp", "example.com:443")
	if len(deadlines) != 2 {
		t.Fatalf("expected two candidate attempts, got %d", len(deadlines))
	}
	if !deadlines[0].Equal(deadlines[1]) {
		t.Fatalf("expected candidate attempts to share one deadline, got %v and %v", deadlines[0], deadlines[1])
	}
}

func TestProtectedFetchRedirectBlocksPrivateTarget(t *testing.T) {
	previousSetting := *system_setting.GetFetchSetting()
	*system_setting.GetFetchSetting() = system_setting.FetchSetting{
		EnableSSRFProtection: true,
		AllowPrivateIp:       false,
		DomainFilterMode:     false,
		IpFilterMode:         false,
	}
	t.Cleanup(func() {
		*system_setting.GetFetchSetting() = previousSetting
	})

	request, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/video", nil)
	if err != nil {
		t.Fatalf("create redirect request: %v", err)
	}
	if err := checkProtectedFetchRedirect(request, nil); err == nil {
		t.Fatal("expected protected redirect into private address to be blocked")
	} else if !types.IsClientInputError(err) {
		t.Fatalf("redirect policy rejection must be a client-input error, got %v", err)
	}
}

func TestProtectedFetchRedirectLimitIsClientInput(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/image.png", nil)
	if err != nil {
		t.Fatalf("create redirect request: %v", err)
	}
	via := make([]*http.Request, 10)

	err = checkProtectedFetchRedirect(request, via)
	if err == nil {
		t.Fatal("expected redirect limit rejection")
	}
	if !types.IsClientInputError(err) {
		t.Fatalf("redirect limit rejection must be a client-input error, got %v", err)
	}
}
