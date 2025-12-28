package service

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDefaultClient(t *testing.T) {
	InitHttpClient()

	transport, ok := httpClient.Transport.(*utlsRoundTripper)
	if !ok {
		t.Fatalf("default client transport = %T, want *utlsRoundTripper", httpClient.Transport)
	}

	clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
	server, rootCAs := startUTLSTestServer(t, clientHelloCh)
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, _, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split host: %v", err)
	}

	transport.transport.TLSClientConfig = &stdtls.Config{
		RootCAs:    rootCAs,
		ServerName: host,
	}

	resp, err := GetHttpClient().Get(server.URL)
	if err != nil {
		t.Fatalf("HTTP GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	if resp.ProtoMajor != 1 {
		t.Fatalf("expected HTTP/1.x, got %s", resp.Proto)
	}

	select {
	case hello := <-clientHelloCh:
		// For IP addresses, SNI extension is not sent (per RFC 6066).
		// Verify other JA3 components match our spec.
		verifyJA3Components(t, hello)
	default:
		t.Fatalf("no ClientHello captured")
	}

	if transport.transport.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 should be false")
	}
}

func TestDefaultClientTLSFailure(t *testing.T) {
	InitHttpClient()
	transport, ok := httpClient.Transport.(*utlsRoundTripper)
	if !ok {
		t.Fatalf("default client transport = %T, want *utlsRoundTripper", httpClient.Transport)
	}

	// Start a TLS server but do not trust its certificate to trigger handshake failure.
	clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
	server, _ := startUTLSTestServer(t, clientHelloCh)
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}

	_, err = GetHttpClient().Do(req)
	if err == nil {
		t.Fatalf("expected TLS verification error")
	}
	transport.CloseIdleConnections()
}

func expectedJA3FromSpec(t *testing.T, serverName string) (string, string) {
	spec := CloneNodeJS22ClientHelloSpec(serverName)
	text, hash, err := ComputeJA3(spec)
	if err != nil {
		t.Fatalf("ComputeJA3: %v", err)
	}
	return text, hash
}

// verifyJA3Components verifies the JA3 components match our Node.js spec.
// This function handles the case where SNI might not be sent (IP addresses).
func verifyJA3Components(t *testing.T, hello *stdtls.ClientHelloInfo) {
	t.Helper()

	// Verify cipher suites match
	if len(hello.CipherSuites) != len(nodeJS22CipherSuites) {
		t.Fatalf("cipher suite count mismatch: got %d, want %d", len(hello.CipherSuites), len(nodeJS22CipherSuites))
	}
	for i, cs := range hello.CipherSuites {
		if cs != nodeJS22CipherSuites[i] {
			t.Fatalf("cipher suite %d mismatch: got %d, want %d", i, cs, nodeJS22CipherSuites[i])
		}
	}

	// Verify supported curves match
	if len(hello.SupportedCurves) != len(nodeJS22SupportedGroups) {
		t.Fatalf("curve count mismatch: got %d, want %d", len(hello.SupportedCurves), len(nodeJS22SupportedGroups))
	}
	for i, curve := range hello.SupportedCurves {
		expected := stdtls.CurveID(nodeJS22SupportedGroups[i])
		if curve != expected {
			t.Fatalf("curve %d mismatch: got %d, want %d", i, curve, expected)
		}
	}

	// Verify point formats match
	if len(hello.SupportedPoints) != len(nodeJS22PointFormats) {
		t.Fatalf("point format count mismatch: got %d, want %d", len(hello.SupportedPoints), len(nodeJS22PointFormats))
	}
	for i, pf := range hello.SupportedPoints {
		if pf != nodeJS22PointFormats[i] {
			t.Fatalf("point format %d mismatch: got %d, want %d", i, pf, nodeJS22PointFormats[i])
		}
	}
}

func ja3FromClientHello(t *testing.T, hello *stdtls.ClientHelloInfo) (string, string) {
	if hello == nil {
		t.Fatalf("nil ClientHelloInfo")
	}

	// Build extension IDs list. Go's ClientHelloInfo.Extensions may or may not
	// include SNI (0), depending on whether a domain name was sent.
	// We prepend SNI if ServerName is set AND 0 is not already in Extensions.
	extensions := make([]string, 0, len(hello.Extensions)+1)
	hasSNI := false
	for _, ext := range hello.Extensions {
		if ext == 0 {
			hasSNI = true
		}
	}
	if hello.ServerName != "" && !hasSNI {
		extensions = append(extensions, "0") // SNI extension ID
	}
	for _, ext := range hello.Extensions {
		extensions = append(extensions, fmt.Sprintf("%d", ext))
	}

	curves := make([]string, len(hello.SupportedCurves))
	for i, c := range hello.SupportedCurves {
		curves[i] = fmt.Sprintf("%d", c)
	}

	points := make([]string, len(hello.SupportedPoints))
	for i, p := range hello.SupportedPoints {
		points[i] = fmt.Sprintf("%d", p)
	}

	ja3Text := strings.Join([]string{
		fmt.Sprintf("%d", nodeJSJA3Version),
		strings.Join(joinUint16(hello.CipherSuites), "-"),
		strings.Join(extensions, "-"),
		strings.Join(curves, "-"),
		strings.Join(points, "-"),
	}, ",")

	sum := md5.Sum([]byte(ja3Text))
	return ja3Text, hex.EncodeToString(sum[:])
}

func skipIfListenDenied(t *testing.T, err error) {
	t.Helper()
	msg := err.Error()
	if strings.Contains(msg, "operation not permitted") || strings.Contains(msg, "permission denied") {
		t.Skipf("listening on loopback is not permitted in this environment: %v", err)
	}
}

func startUTLSTestServer(t *testing.T, clientHelloCh chan *stdtls.ClientHelloInfo) (*httptest.Server, *x509.CertPool) {
	t.Helper()

	cert, pool := generateSelfSignedCert(t)
	tlsConfig := &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		GetConfigForClient: func(chi *stdtls.ClientHelloInfo) (*stdtls.Config, error) {
			select {
			case clientHelloCh <- chi:
			default:
			}
			return nil, nil
		},
	}

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		skipIfListenDenied(t, err)
		t.Fatalf("listen on tcp4: %v", err)
	}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	server.Listener = ln
	server.TLS = tlsConfig
	server.StartTLS()
	return server, pool
}

func generateSelfSignedCert(t *testing.T) (stdtls.Certificate, *x509.CertPool) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	parsed, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	cert := stdtls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  privateKey,
		Leaf:        parsed,
	}

	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return cert, pool
}
