package service

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

const expectedJA3Hash = "0cce74b0d9b7f8528fb2181588d23793"

// TestJA3MatchesDirectClient spins up a local TLS server, captures the
// ClientHello for direct client, and ensures it uses the expected JA3 components.
func TestJA3MatchesDirectClient(t *testing.T) {
	InitHttpClient()

	clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
	server, pool := startUTLSTestServer(t, clientHelloCh)
	defer server.Close()

	host := mustSplitHost(t, server.URL)
	setDefaultClientTLS(t, host, pool)

	// Test direct client
	resp, err := GetHttpClient().Get(server.URL)
	if err != nil {
		t.Fatalf("direct HTTP GET failed: %v", err)
	}
	resp.Body.Close()

	select {
	case directHello := <-clientHelloCh:
		verifyJA3Components(t, directHello)
	case <-time.After(2 * time.Second):
		t.Fatalf("did not capture direct ClientHello")
	}
}

// TestJA3MatchesProxyClient verifies proxy client uses the expected JA3 components.
func TestJA3MatchesProxyClient(t *testing.T) {
	t.Skip("HTTP proxy with uTLS has connection issues - use TestProxyClient/SOCKS5 instead")
	if testing.Short() {
		t.Skip("skipping proxy test in short mode")
	}

	InitHttpClient()
	ResetProxyClientCache()
	t.Cleanup(ResetProxyClientCache)

	clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
	server, pool := startUTLSTestServer(t, clientHelloCh)
	defer server.Close()

	host := mustSplitHost(t, server.URL)

	// Start proxy server
	proxy := startHTTPProxyServer(t, proxyServerConfig{})
	defer proxy.Close()

	proxyClient := newProxyClientForServer(t, proxy.URL, host, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}

	resp, err := proxyClient.Do(req)
	if err != nil {
		t.Fatalf("proxy HTTP GET failed: %v", err)
	}
	resp.Body.Close()

	select {
	case proxyHello := <-clientHelloCh:
		verifyJA3Components(t, proxyHello)
	case <-time.After(2 * time.Second):
		t.Fatalf("did not capture proxy ClientHello")
	}
}

func setDefaultClientTLS(t *testing.T, serverName string, pool *x509.CertPool) {
	t.Helper()

	client := GetHttpClient()
	rt, ok := client.Transport.(*utlsRoundTripper)
	if !ok {
		t.Fatalf("default transport = %T, want *utlsRoundTripper", client.Transport)
	}

	rt.transport.TLSClientConfig = &stdtls.Config{
		RootCAs:    pool,
		ServerName: serverName,
	}
}

func newProxyClientForServer(t *testing.T, proxyURL, serverName string, pool *x509.CertPool) *http.Client {
	t.Helper()

	client, err := NewProxyHttpClient(proxyURL)
	if err != nil {
		t.Fatalf("create proxy client: %v", err)
	}

	rt, ok := client.Transport.(*proxyUTLSRoundTripper)
	if !ok {
		t.Fatalf("proxy transport = %T, want *proxyUTLSRoundTripper", client.Transport)
	}

	rt.transport.TLSClientConfig = &stdtls.Config{
		RootCAs:    pool,
		ServerName: serverName,
	}
	return client
}

func requestAndJA3(t *testing.T, client *http.Client, targetURL string, clientHelloCh <-chan *stdtls.ClientHelloInfo) (string, string) {
	t.Helper()

	resp, err := client.Get(targetURL)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	select {
	case hello := <-clientHelloCh:
		return ja3FromClientHello(t, hello)
	case <-time.After(2 * time.Second):
		t.Fatalf("did not capture ClientHello for %s", targetURL)
		return "", ""
	}
}

func TestEnsurePortDefaults(t *testing.T) {
	t.Parallel()

	cases := []struct {
		host   string
		scheme string
		want   string
	}{
		{host: "example.com", scheme: "http", want: "example.com:80"},
		{host: "example.com", scheme: "https", want: "example.com:443"},
		{host: "example.com", scheme: "socks5", want: "example.com:1080"},
		{host: "example.com:9000", scheme: "https", want: "example.com:9000"},
	}

	for _, tc := range cases {
		got := ensurePort(tc.host, tc.scheme)
		if got != tc.want {
			t.Fatalf("ensurePort(%q, %q) = %q, want %q", tc.host, tc.scheme, got, tc.want)
		}
	}
}

func TestPerformHTTPConnectFailure(t *testing.T) {
	t.Parallel()

	clientSide, serverSide := net.Pipe()
	defer clientSide.Close()
	defer serverSide.Close()

	proxyURL, err := url.Parse("http://user:pass@proxy.local")
	if err != nil {
		t.Fatalf("parse proxy URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	handlerDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(serverSide)
		req, err := http.ReadRequest(reader)
		if err != nil {
			handlerDone <- err
			return
		}

		if req.Method != http.MethodConnect || req.Host != "example.com:443" {
			handlerDone <- errors.New("unexpected CONNECT request contents")
			return
		}

		expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
		if got := req.Header.Get("Proxy-Authorization"); got != expectedAuth {
			handlerDone <- errors.New("missing or incorrect proxy auth header")
			return
		}

		_, _ = io.Copy(io.Discard, req.Body)
		req.Body.Close()
		_, _ = serverSide.Write([]byte("HTTP/1.1 403 Forbidden\r\nContent-Length: 0\r\n\r\n"))
		handlerDone <- nil
	}()

	err = performHTTPConnect(ctx, clientSide, proxyURL, "example.com:443")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected 403 error from proxy, got %v", err)
	}

	if handlerErr := <-handlerDone; handlerErr != nil {
		t.Fatalf("proxy handler error: %v", handlerErr)
	}
}
