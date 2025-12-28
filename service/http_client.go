package service

import (
	"bufio"
	"context"
	stdtls "crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/system_setting"
	utls "github.com/refraction-networking/utls"

	"golang.org/x/net/proxy"
)

var (
	httpClient      *http.Client
	proxyClientLock sync.Mutex
	proxyClients    = make(map[string]*http.Client)
)

type utlsRoundTripper struct {
	transport *http.Transport
}

func newUTLSRoundTripper() *utlsRoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.ForceAttemptHTTP2 = false
	base.DialTLSContext = makeUTLSDialer(base)

	return &utlsRoundTripper{
		transport: base,
	}
}

func makeUTLSDialer(t *http.Transport) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialContext := t.DialContext
		if dialContext == nil {
			dialContext = (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext
		}

		rawConn, err := dialContext(ctx, network, addr)
		if err != nil {
			return nil, err
		}

		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			rawConn.Close()
			return nil, err
		}

		return performUTLSHandshake(ctx, rawConn, host, t.TLSClientConfig)
	}
}

func cloneTLSConfigForUTLS(base *stdtls.Config, serverName string) *utls.Config {
	if base == nil {
		return &utls.Config{ServerName: serverName}
	}

	// Manual conversion since utls.Config and stdtls.Config are not directly castable
	cfg := &utls.Config{
		ServerName:         base.ServerName,
		InsecureSkipVerify: base.InsecureSkipVerify,
		MinVersion:         base.MinVersion,
		MaxVersion:         base.MaxVersion,
		RootCAs:            base.RootCAs,
	}
	if cfg.ServerName == "" {
		cfg.ServerName = serverName
	}
	return cfg
}

func cloneStandardTLSConfig(base *stdtls.Config, serverName string) *stdtls.Config {
	if base == nil {
		return &stdtls.Config{ServerName: serverName}
	}

	cfg := base.Clone()
	if cfg.ServerName == "" {
		cfg.ServerName = serverName
	}
	return cfg
}

type proxyUTLSRoundTripper struct {
	transport *http.Transport
	proxyURL  *url.URL
}

func newProxyUTLSRoundTripper(proxyURL *url.URL) (*proxyUTLSRoundTripper, error) {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.ForceAttemptHTTP2 = false

	dialContext := base.DialContext
	if dialContext == nil {
		dialContext = (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext
	}

	base.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialViaProxy(ctx, network, addr, proxyURL, dialContext, base.TLSClientConfig, false)
	}
	base.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dialViaProxy(ctx, network, addr, proxyURL, dialContext, base.TLSClientConfig, true)
	}

	return &proxyUTLSRoundTripper{
		transport: base,
		proxyURL:  proxyURL,
	}, nil
}

func dialViaProxy(ctx context.Context, network, addr string, proxyURL *url.URL, dialContext func(ctx context.Context, network, addr string) (net.Conn, error), tlsConfig *stdtls.Config, withTLS bool) (net.Conn, error) {
	switch proxyURL.Scheme {
	case "http", "https":
		return dialViaHTTPProxy(ctx, network, addr, proxyURL, dialContext, tlsConfig, withTLS)
	case "socks5", "socks5h":
		return dialViaSOCKS5Proxy(ctx, network, addr, proxyURL, dialContext, tlsConfig, withTLS)
	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", proxyURL.Scheme)
	}
}

func dialViaHTTPProxy(ctx context.Context, network, addr string, proxyURL *url.URL, dialContext func(ctx context.Context, network, addr string) (net.Conn, error), tlsConfig *stdtls.Config, withTLS bool) (net.Conn, error) {
	proxyAddr := ensurePort(proxyURL.Host, proxyURL.Scheme)
	conn, err := dialContext(ctx, network, proxyAddr)
	if err != nil {
		return nil, err
	}

	if proxyURL.Scheme == "https" {
		host, _, splitErr := net.SplitHostPort(proxyAddr)
		if splitErr != nil {
			conn.Close()
			return nil, splitErr
		}
		tlsConn := stdtls.Client(conn, cloneStandardTLSConfig(tlsConfig, host))
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			tlsConn.Close()
			return nil, err
		}
		conn = tlsConn
	}

	if err := performHTTPConnect(ctx, conn, proxyURL, addr); err != nil {
		conn.Close()
		return nil, err
	}

	if !withTLS {
		return conn, nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	tlsConn, err := performUTLSHandshake(ctx, conn, host, tlsConfig)
	if err != nil {
		return nil, err
	}
	return tlsConn, nil
}

func dialViaSOCKS5Proxy(ctx context.Context, network, addr string, proxyURL *url.URL, dialContext func(ctx context.Context, network, addr string) (net.Conn, error), tlsConfig *stdtls.Config, withTLS bool) (net.Conn, error) {
	var auth *proxy.Auth
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		auth = &proxy.Auth{
			User:     proxyURL.User.Username(),
			Password: password,
		}
	}

	forward := &contextDialerAdapter{dialContext: dialContext}
	dialer, err := proxy.SOCKS5("tcp", ensurePort(proxyURL.Host, proxyURL.Scheme), auth, forward)
	if err != nil {
		return nil, err
	}

	var conn net.Conn
	if ctxDialer, ok := dialer.(proxy.ContextDialer); ok {
		conn, err = ctxDialer.DialContext(ctx, network, addr)
	} else {
		conn, err = dialer.Dial(network, addr)
	}
	if err != nil {
		return nil, err
	}

	if !withTLS {
		return conn, nil
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		conn.Close()
		return nil, err
	}

	tlsConn, err := performUTLSHandshake(ctx, conn, host, tlsConfig)
	if err != nil {
		return nil, err
	}
	return tlsConn, nil
}

type contextDialerAdapter struct {
	dialContext func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (d *contextDialerAdapter) Dial(network, addr string) (net.Conn, error) {
	return d.dialContext(context.Background(), network, addr)
}

func (d *contextDialerAdapter) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	return d.dialContext(ctx, network, addr)
}

func performHTTPConnect(ctx context.Context, conn net.Conn, proxyURL *url.URL, targetAddr string) error {
	req := &http.Request{
		Method: "CONNECT",
		URL:    &url.URL{Opaque: targetAddr},
		Host:   targetAddr,
		Header: make(http.Header),
	}

	if proxyURL.User != nil {
		username := proxyURL.User.Username()
		password, _ := proxyURL.User.Password()
		token := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
		req.Header.Set("Proxy-Authorization", "Basic "+token)
	}

	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
		defer conn.SetDeadline(time.Time{})
	}

	if err := req.Write(conn); err != nil {
		return err
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy connect failed: %s", resp.Status)
	}

	return nil
}

func ensurePort(host, scheme string) string {
	if strings.Contains(host, ":") {
		return host
	}

	switch scheme {
	case "https":
		return net.JoinHostPort(host, "443")
	case "socks5", "socks5h":
		return net.JoinHostPort(host, "1080")
	default:
		return net.JoinHostPort(host, "80")
	}
}

func performUTLSHandshake(ctx context.Context, rawConn net.Conn, host string, baseConfig *stdtls.Config) (net.Conn, error) {
	clientConfig := cloneTLSConfigForUTLS(baseConfig, host)
	uconn := utls.UClient(rawConn, clientConfig, utls.HelloCustom)

	spec := CloneNodeJS22ClientHelloSpec(host)
	if err := uconn.ApplyPreset(spec); err != nil {
		rawConn.Close()
		return nil, err
	}

	if err := uconn.HandshakeContext(ctx); err != nil {
		uconn.Close()
		return nil, err
	}

	return uconn, nil
}

func (r *utlsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.transport.RoundTrip(req)
}

func (r *utlsRoundTripper) CloseIdleConnections() {
	r.transport.CloseIdleConnections()
}

func (r *proxyUTLSRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r.transport.RoundTrip(req)
}

func (r *proxyUTLSRoundTripper) CloseIdleConnections() {
	r.transport.CloseIdleConnections()
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	fetchSetting := system_setting.GetFetchSetting()
	urlStr := req.URL.String()
	if err := common.ValidateURLWithFetchSetting(urlStr, fetchSetting.EnableSSRFProtection, fetchSetting.AllowPrivateIp, fetchSetting.DomainFilterMode, fetchSetting.IpFilterMode, fetchSetting.DomainList, fetchSetting.IpList, fetchSetting.AllowedPorts, fetchSetting.ApplyIPFilterForDomain); err != nil {
		return fmt.Errorf("redirect to %s blocked: %v", urlStr, err)
	}
	if len(via) >= 10 {
		return fmt.Errorf("stopped after 10 redirects")
	}
	return nil
}

func InitHttpClient() {
	client := &http.Client{
		Transport:     newUTLSRoundTripper(),
		CheckRedirect: checkRedirect,
	}

	if common.RelayTimeout != 0 {
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
	}

	httpClient = client
}

func GetHttpClient() *http.Client {
	if httpClient == nil {
		InitHttpClient()
	}
	return httpClient
}

// ResetProxyClientCache 清空代理客户端缓存，确保下次使用时重新初始化
func ResetProxyClientCache() {
	proxyClientLock.Lock()
	defer proxyClientLock.Unlock()
	for _, client := range proxyClients {
		if closer, ok := client.Transport.(interface{ CloseIdleConnections() }); ok && closer != nil {
			closer.CloseIdleConnections()
		}
	}
	proxyClients = make(map[string]*http.Client)
}

// NewProxyHttpClient 创建支持代理的 HTTP 客户端
func NewProxyHttpClient(proxyURL string) (*http.Client, error) {
	if proxyURL == "" {
		return GetHttpClient(), nil
	}

	proxyClientLock.Lock()
	if client, ok := proxyClients[proxyURL]; ok {
		proxyClientLock.Unlock()
		return client, nil
	}
	proxyClientLock.Unlock()

	parsedURL, err := url.Parse(proxyURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Scheme {
	case "http", "https", "socks5", "socks5h":
		transport, err := newProxyUTLSRoundTripper(parsedURL)
		if err != nil {
			return nil, err
		}

		client := &http.Client{
			Transport:     transport,
			CheckRedirect: checkRedirect,
		}
		client.Timeout = time.Duration(common.RelayTimeout) * time.Second
		proxyClientLock.Lock()
		proxyClients[proxyURL] = client
		proxyClientLock.Unlock()
		return client, nil

	default:
		return nil, fmt.Errorf("unsupported proxy scheme: %s, must be http, https, socks5 or socks5h", parsedURL.Scheme)
	}
}
