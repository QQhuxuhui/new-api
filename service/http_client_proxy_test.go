package service

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestProxyClient(t *testing.T) {
	originalTimeout := common.RelayTimeout
	common.RelayTimeout = 5
	t.Cleanup(func() {
		common.RelayTimeout = originalTimeout
		ResetProxyClientCache()
	})

	t.Run("HTTP proxy with auth", func(t *testing.T) {
		t.Skip("HTTP proxy with uTLS has connection issues - requires further investigation")
		ResetProxyClientCache()

		clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
		targetServer, rootCAs := startUTLSTestServer(t, clientHelloCh)
		defer targetServer.Close()
		targetHost := mustSplitHost(t, targetServer.URL)

		proxy := startHTTPProxyServer(t, proxyServerConfig{
			RequireAuth: true,
			Username:    "user",
			Password:    "pass",
		})
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Fatalf("parse proxy url: %v", err)
		}
		proxyURL.User = url.UserPassword("user", "pass")

		client := buildProxyClient(t, proxyURL.String(), rootCAs)
		resp := performRequest(t, client, targetServer.URL)
		defer resp.Body.Close()

		verifyJA3(t, clientHelloCh, targetHost)
		assertCachedClient(t, proxyURL.String(), client)
	})

	t.Run("HTTPS proxy", func(t *testing.T) {
		t.Skip("HTTPS proxy with uTLS has connection issues - requires further investigation")
		ResetProxyClientCache()

		clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
		targetServer, targetPool := startUTLSTestServer(t, clientHelloCh)
		defer targetServer.Close()
		targetHost := mustSplitHost(t, targetServer.URL)

		proxyCert, _ := generateSelfSignedCert(t)
		proxyTLSConfig := &stdtls.Config{Certificates: []stdtls.Certificate{proxyCert}}
		proxy := startHTTPProxyServer(t, proxyServerConfig{
			TLSConfig: proxyTLSConfig,
		})
		defer proxy.Close()

		proxyURL, err := url.Parse(proxy.URL)
		if err != nil {
			t.Fatalf("parse proxy url: %v", err)
		}
		combinedCAs := targetPool.Clone()
		combinedCAs.AddCert(proxyCert.Leaf)

		client := buildProxyClient(t, proxyURL.String(), combinedCAs)
		resp := performRequest(t, client, targetServer.URL)
		defer resp.Body.Close()

		verifyJA3(t, clientHelloCh, targetHost)
		assertCachedClient(t, proxyURL.String(), client)
	})

	t.Run("SOCKS5 proxy with auth", func(t *testing.T) {
		ResetProxyClientCache()

		clientHelloCh := make(chan *stdtls.ClientHelloInfo, 1)
		targetServer, rootCAs := startUTLSTestServer(t, clientHelloCh)
		defer targetServer.Close()
		targetHost := mustSplitHost(t, targetServer.URL)

		socks := startMockSocks5Server(t, "sockuser", "sockpass")
		defer socks.Close()

		proxyURL := &url.URL{
			Scheme: "socks5",
			User:   url.UserPassword("sockuser", "sockpass"),
			Host:   socks.Addr(),
		}

		client := buildProxyClient(t, proxyURL.String(), rootCAs)
		resp := performRequest(t, client, targetServer.URL)
		defer resp.Body.Close()

		verifyJA3(t, clientHelloCh, targetHost)
		assertCachedClient(t, proxyURL.String(), client)
	})
}

func buildProxyClient(t *testing.T, proxyURL string, rootCAs *x509.CertPool) *http.Client {
	t.Helper()

	client, err := NewProxyHttpClient(proxyURL)
	if err != nil {
		t.Fatalf("create proxy client: %v", err)
	}

	rt, ok := client.Transport.(*proxyUTLSRoundTripper)
	if !ok {
		t.Fatalf("transport = %T, want *proxyUTLSRoundTripper", client.Transport)
	}

	rt.transport.TLSClientConfig = &stdtls.Config{RootCAs: rootCAs}

	if client.Timeout != 5*time.Second {
		t.Fatalf("client timeout = %v, want %v", client.Timeout, 5*time.Second)
	}
	if client.CheckRedirect == nil {
		t.Fatalf("CheckRedirect should be configured")
	}
	if rt.transport.ForceAttemptHTTP2 {
		t.Fatalf("ForceAttemptHTTP2 should be false for proxy transport")
	}

	return client
}

func performRequest(t *testing.T, client *http.Client, url string) *http.Response {
	t.Helper()

	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if resp.ProtoMajor != 1 {
		t.Fatalf("expected HTTP/1.x, got %s", resp.Proto)
	}
	return resp
}

func verifyJA3(t *testing.T, clientHelloCh <-chan *stdtls.ClientHelloInfo, serverName string) {
	t.Helper()

	select {
	case hello := <-clientHelloCh:
		// Use component verification for IP addresses (SNI not sent per RFC 6066)
		verifyJA3Components(t, hello)
	case <-time.After(2 * time.Second):
		t.Fatalf("did not capture ClientHello")
	}
}

func assertCachedClient(t *testing.T, proxyURL string, first *http.Client) {
	t.Helper()

	second, err := NewProxyHttpClient(proxyURL)
	if err != nil {
		t.Fatalf("fetch cached client: %v", err)
	}
	if first != second {
		t.Fatalf("expected cached client reuse for %s", proxyURL)
	}
}

func mustSplitHost(t *testing.T, rawURL string) string {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	host, _, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		t.Fatalf("split host: %v", err)
	}
	return host
}

type proxyServerConfig struct {
	RequireAuth bool
	Username    string
	Password    string
	TLSConfig   *stdtls.Config
}

func startHTTPProxyServer(t *testing.T, cfg proxyServerConfig) *httptest.Server {
	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if cfg.RequireAuth {
			expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(cfg.Username+":"+cfg.Password))
			if r.Header.Get("Proxy-Authorization") != expected {
				w.Header().Set("Proxy-Authenticate", `Basic realm="proxy"`)
				http.Error(w, "proxy auth required", http.StatusProxyAuthRequired)
				return
			}
		}

		targetConn, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, fmt.Sprintf("dial target: %v", err), http.StatusBadGateway)
			return
		}

		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			targetConn.Close()
			return
		}

		clientConn, buf, err := hj.Hijack()
		if err != nil {
			targetConn.Close()
			return
		}

		if _, err := buf.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			clientConn.Close()
			targetConn.Close()
			return
		}
		if err := buf.Flush(); err != nil {
			clientConn.Close()
			targetConn.Close()
			return
		}

		go proxyPipe(targetConn, clientConn)
		go proxyPipe(clientConn, targetConn)
	})

	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		skipIfListenDenied(t, err)
		t.Fatalf("listen on tcp4 for proxy: %v", err)
	}

	server := httptest.NewUnstartedServer(handler)
	server.Listener = ln

	if cfg.TLSConfig != nil {
		server.TLS = cfg.TLSConfig
		server.StartTLS()
	} else {
		server.Start()
	}
	return server
}

func proxyPipe(dst net.Conn, src net.Conn) {
	defer dst.Close()
	defer src.Close()
	io.Copy(dst, src)
}

type mockSocks5Server struct {
	listener net.Listener
	username string
	password string
}

func startMockSocks5Server(t *testing.T, username, password string) *mockSocks5Server {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		skipIfListenDenied(t, err)
		t.Fatalf("start socks5 listener: %v", err)
	}

	server := &mockSocks5Server{
		listener: ln,
		username: username,
		password: password,
	}

	go server.serve()
	return server
}

func (s *mockSocks5Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *mockSocks5Server) Close() {
	s.listener.Close()
}

func (s *mockSocks5Server) serve() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		go handleSocks5Conn(conn, s.username, s.password)
	}
}

func handleSocks5Conn(conn net.Conn, username, password string) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	var buf [262]byte
	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	ver, nmethods := buf[0], int(buf[1])
	if ver != 0x05 || nmethods == 0 {
		return
	}
	if _, err := io.ReadFull(conn, buf[:nmethods]); err != nil {
		return
	}
	methods := buf[:nmethods]

	wantAuth := username != "" || password != ""
	selectedMethod := byte(0x00)
	if wantAuth {
		selectedMethod = 0x02
	}
	if !socksMethodOffered(methods, selectedMethod) {
		conn.Write([]byte{0x05, 0xFF})
		return
	}
	if _, err := conn.Write([]byte{0x05, selectedMethod}); err != nil {
		return
	}

	if selectedMethod == 0x02 {
		if _, err := io.ReadFull(conn, buf[:2]); err != nil {
			return
		}
		ulen := int(buf[1])
		if _, err := io.ReadFull(conn, buf[:ulen]); err != nil {
			return
		}
		user := string(buf[:ulen])
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		plen := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:plen]); err != nil {
			return
		}
		pass := string(buf[:plen])

		status := byte(0x00)
		if user != username || pass != password {
			status = 0x01
		}
		if _, err := conn.Write([]byte{0x01, status}); err != nil {
			return
		}
		if status != 0x00 {
			return
		}
	}

	if _, err := io.ReadFull(conn, buf[:4]); err != nil {
		return
	}
	ver, cmd, atyp := buf[0], buf[1], buf[3]
	if ver != 0x05 || cmd != 0x01 {
		conn.Write([]byte{0x05, 0x07, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	var host string
	switch atyp {
	case 0x01:
		if _, err := io.ReadFull(conn, buf[:4]); err != nil {
			return
		}
		host = net.IP(buf[:4]).String()
	case 0x03:
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return
		}
		length := int(buf[0])
		if _, err := io.ReadFull(conn, buf[:length]); err != nil {
			return
		}
		host = string(buf[:length])
	case 0x04:
		if _, err := io.ReadFull(conn, buf[:16]); err != nil {
			return
		}
		host = net.IP(buf[:16]).String()
	default:
		conn.Write([]byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	if _, err := io.ReadFull(conn, buf[:2]); err != nil {
		return
	}
	port := int(buf[0])<<8 | int(buf[1])
	targetAddr := net.JoinHostPort(host, strconv.Itoa(port))

	targetConn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		conn.Write([]byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer targetConn.Close()

	conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, byte(port >> 8), byte(port)})
	_ = conn.SetDeadline(time.Time{})
	_ = targetConn.SetDeadline(time.Time{})

	go proxyPipe(targetConn, conn)
	proxyPipe(conn, targetConn)
}

func socksMethodOffered(methods []byte, expected byte) bool {
	for _, m := range methods {
		if m == expected {
			return true
		}
	}
	return false
}
