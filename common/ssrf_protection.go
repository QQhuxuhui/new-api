package common

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

const ssrfDNSLookupTimeout = 30 * time.Second

// SSRFProtection SSRF防护配置
type SSRFProtection struct {
	AllowPrivateIp         bool
	DomainFilterMode       bool     // true: 白名单, false: 黑名单
	DomainList             []string // domain format, e.g. example.com, *.example.com
	IpFilterMode           bool     // true: 白名单, false: 黑名单
	IpList                 []string // CIDR or single IP
	AllowedPorts           []int    // 允许的端口范围
	ApplyIPFilterForDomain bool     // 对域名启用IP过滤
}

// DefaultSSRFProtection 默认SSRF防护配置
var DefaultSSRFProtection = &SSRFProtection{
	AllowPrivateIp:   false,
	DomainFilterMode: true,
	DomainList:       []string{},
	IpFilterMode:     true,
	IpList:           []string{},
	AllowedPorts:     []int{},
}

var specialPurposeIPPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.31.196.0/24"),
	netip.MustParsePrefix("192.52.193.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("192.175.48.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/128"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("100:0:0:1::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2620:4f:8000::/48"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("ff00::/8"),
}

var allocatedPublicIPv6Prefixes = []netip.Prefix{
	netip.MustParsePrefix("2001:200::/23"),
	netip.MustParsePrefix("2001:400::/23"),
	netip.MustParsePrefix("2001:600::/23"),
	netip.MustParsePrefix("2001:800::/22"),
	netip.MustParsePrefix("2001:c00::/23"),
	netip.MustParsePrefix("2001:e00::/23"),
	netip.MustParsePrefix("2001:1200::/23"),
	netip.MustParsePrefix("2001:1400::/22"),
	netip.MustParsePrefix("2001:1800::/23"),
	netip.MustParsePrefix("2001:1a00::/23"),
	netip.MustParsePrefix("2001:1c00::/22"),
	netip.MustParsePrefix("2001:2000::/19"),
	netip.MustParsePrefix("2001:4000::/23"),
	netip.MustParsePrefix("2001:4200::/23"),
	netip.MustParsePrefix("2001:4400::/23"),
	netip.MustParsePrefix("2001:4600::/23"),
	netip.MustParsePrefix("2001:4800::/23"),
	netip.MustParsePrefix("2001:4a00::/23"),
	netip.MustParsePrefix("2001:4c00::/23"),
	netip.MustParsePrefix("2001:5000::/20"),
	netip.MustParsePrefix("2001:8000::/19"),
	netip.MustParsePrefix("2001:a000::/20"),
	netip.MustParsePrefix("2001:b000::/20"),
	netip.MustParsePrefix("2003::/18"),
	netip.MustParsePrefix("2400::/12"),
	netip.MustParsePrefix("2410::/12"),
	netip.MustParsePrefix("2600::/12"),
	netip.MustParsePrefix("2610::/23"),
	netip.MustParsePrefix("2620::/23"),
	netip.MustParsePrefix("2630::/12"),
	netip.MustParsePrefix("2800::/12"),
	netip.MustParsePrefix("2a00::/12"),
	netip.MustParsePrefix("2a10::/12"),
	netip.MustParsePrefix("2c00::/12"),
}

func isAllocatedPublicIPv6(address netip.Addr) bool {
	for _, prefix := range allocatedPublicIPv6Prefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

// isPrivateIP 检查IP是否为私有或特殊用途地址
func isPrivateIP(ip net.IP) bool {
	address, ok := netip.AddrFromSlice(ip)
	if !ok {
		return true
	}
	address = address.Unmap()
	if address.Is6() && !isAllocatedPublicIPv6(address) {
		return true
	}

	for _, prefix := range specialPurposeIPPrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func normalizeDomain(domain string) (string, error) {
	domain = strings.TrimRight(strings.TrimSpace(domain), ".")
	if domain == "" {
		return "", fmt.Errorf("invalid host")
	}

	asciiDomain, err := idna.Lookup.ToASCII(domain)
	if err != nil {
		return "", fmt.Errorf("invalid host %q: %v", domain, err)
	}
	asciiDomain = strings.ToLower(strings.TrimRight(asciiDomain, "."))
	if asciiDomain == "" {
		return "", fmt.Errorf("invalid host")
	}
	return asciiDomain, nil
}

// parsePortRanges 解析端口范围配置
// 支持格式: "80", "443", "8000-9000"
func parsePortRanges(portConfigs []string) ([]int, error) {
	var ports []int

	for _, config := range portConfigs {
		config = strings.TrimSpace(config)
		if config == "" {
			continue
		}

		if strings.Contains(config, "-") {
			// 处理端口范围 "8000-9000"
			parts := strings.Split(config, "-")
			if len(parts) != 2 {
				return nil, fmt.Errorf("invalid port range format: %s", config)
			}

			startPort, err := strconv.Atoi(strings.TrimSpace(parts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid start port in range %s: %v", config, err)
			}

			endPort, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid end port in range %s: %v", config, err)
			}

			if startPort > endPort {
				return nil, fmt.Errorf("invalid port range %s: start port cannot be greater than end port", config)
			}

			if startPort < 1 || startPort > 65535 || endPort < 1 || endPort > 65535 {
				return nil, fmt.Errorf("port range %s contains invalid port numbers (must be 1-65535)", config)
			}

			// 添加范围内的所有端口
			for port := startPort; port <= endPort; port++ {
				ports = append(ports, port)
			}
		} else {
			// 处理单个端口 "80"
			port, err := strconv.Atoi(config)
			if err != nil {
				return nil, fmt.Errorf("invalid port number: %s", config)
			}

			if port < 1 || port > 65535 {
				return nil, fmt.Errorf("invalid port number %d (must be 1-65535)", port)
			}

			ports = append(ports, port)
		}
	}

	return ports, nil
}

// isAllowedPort 检查端口是否被允许
func (p *SSRFProtection) isAllowedPort(port int) bool {
	if len(p.AllowedPorts) == 0 {
		return true // 如果没有配置端口限制，则允许所有端口
	}

	for _, allowedPort := range p.AllowedPorts {
		if port == allowedPort {
			return true
		}
	}
	return false
}

// isDomainWhitelisted 检查域名是否在白名单中
func isDomainListed(domain string, list []string) bool {
	if len(list) == 0 {
		return false
	}

	domain, err := normalizeDomain(domain)
	if err != nil {
		return false
	}
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		wildcard := strings.HasPrefix(item, "*.")
		if wildcard {
			item = strings.TrimPrefix(item, "*.")
		}
		item, err = normalizeDomain(item)
		if err != nil {
			continue
		}
		if domain == item {
			return true
		}
		if wildcard && strings.HasSuffix(domain, "."+item) {
			return true
		}
	}
	return false
}

func (p *SSRFProtection) isDomainAllowed(domain string) bool {
	listed := isDomainListed(domain, p.DomainList)
	if p.DomainFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

// isIPWhitelisted 检查IP是否在白名单中

func isIPListed(ip net.IP, list []string) bool {
	if len(list) == 0 {
		return false
	}

	for _, whitelistCIDR := range list {
		whitelistCIDR = strings.TrimSpace(whitelistCIDR)
		_, network, err := net.ParseCIDR(whitelistCIDR)
		if err != nil {
			// 尝试作为单个IP处理
			if whitelistIP := net.ParseIP(whitelistCIDR); whitelistIP != nil {
				if ip.Equal(whitelistIP) {
					return true
				}
			}
			continue
		}

		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// IsIPAccessAllowed 检查IP是否允许访问
func (p *SSRFProtection) IsIPAccessAllowed(ip net.IP) bool {
	// 私有IP限制
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return false
	}

	listed := isIPListed(ip, p.IpList)
	if p.IpFilterMode { // 白名单
		return listed
	}
	// 黑名单
	return !listed
}

// NewSSRFProtectionFromFetchSetting builds an SSRFProtection from persisted fetch_setting values.
func NewSSRFProtectionFromFetchSetting(allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) (*SSRFProtection, error) {
	allowedPortInts, err := parsePortRanges(allowedPorts)
	if err != nil {
		return nil, fmt.Errorf("request reject - invalid port configuration: %v", err)
	}

	return &SSRFProtection{
		AllowPrivateIp:         allowPrivateIp,
		DomainFilterMode:       domainFilterMode,
		DomainList:             domainList,
		IpFilterMode:           ipFilterMode,
		IpList:                 ipList,
		AllowedPorts:           allowedPortInts,
		ApplyIPFilterForDomain: applyIPFilterForDomain,
	}, nil
}

func (p *SSRFProtection) ipAccessError(host string, ip net.IP) error {
	if host != "" {
		if isPrivateIP(ip) && !p.AllowPrivateIp {
			return fmt.Errorf("private IP address not allowed: %s resolves to %s", host, ip.String())
		}
		if p.IpFilterMode {
			return fmt.Errorf("ip not in whitelist: %s resolves to %s", host, ip.String())
		}
		return fmt.Errorf("ip in blacklist: %s resolves to %s", host, ip.String())
	}

	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return fmt.Errorf("private IP address not allowed: %s", ip.String())
	}
	if p.IpFilterMode {
		return fmt.Errorf("ip not in whitelist: %s", ip.String())
	}
	return fmt.Errorf("ip in blacklist: %s", ip.String())
}

// ValidateNetworkTarget validates the host and port before dialing.
func (p *SSRFProtection) ValidateNetworkTarget(host string, port int) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("invalid host")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if !p.isAllowedPort(port) {
		return fmt.Errorf("port %d is not allowed", port)
	}

	if ip := net.ParseIP(host); ip != nil {
		if !p.IsIPAccessAllowed(ip) {
			return p.ipAccessError("", ip)
		}
		return nil
	}

	normalizedHost, err := normalizeDomain(host)
	if err != nil {
		return err
	}
	if !p.isDomainAllowed(normalizedHost) {
		if p.DomainFilterMode {
			return fmt.Errorf("domain not in whitelist: %s", normalizedHost)
		}
		return fmt.Errorf("domain in blacklist: %s", normalizedHost)
	}
	return nil
}

// ValidateResolvedIP validates a domain's resolved IP immediately before dialing it.
func (p *SSRFProtection) ValidateResolvedIP(host string, ip net.IP) error {
	if isPrivateIP(ip) && !p.AllowPrivateIp {
		return p.ipAccessError(host, ip)
	}
	if p.ApplyIPFilterForDomain && !p.IsIPAccessAllowed(ip) {
		return p.ipAccessError(host, ip)
	}
	return nil
}

func (p *SSRFProtection) validateURLTarget(urlStr string) (string, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return "", fmt.Errorf("invalid URL format: %v", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported protocol: %s (only http/https allowed)", u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("invalid host")
	}
	portStr := u.Port()
	if portStr == "" {
		portStr = "80"
		if u.Scheme == "https" {
			portStr = "443"
		}
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("invalid port: %s", portStr)
	}

	if err := p.ValidateNetworkTarget(host, port); err != nil {
		return "", err
	}
	return host, nil
}

func (p *SSRFProtection) ValidateURLTarget(urlStr string) error {
	_, err := p.validateURLTarget(urlStr)
	return err
}

// ValidateURL 验证URL是否安全
func (p *SSRFProtection) ValidateURL(urlStr string) error {
	host, err := p.validateURLTarget(urlStr)
	if err != nil {
		return err
	}
	if net.ParseIP(host) != nil {
		return nil
	}

	host, err = normalizeDomain(host)
	if err != nil {
		return err
	}
	lookupCtx, cancel := context.WithTimeout(context.Background(), ssrfDNSLookupTimeout)
	defer cancel()
	ipAddresses, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %v", host, err)
	}
	for _, ipAddress := range ipAddresses {
		if err := p.ValidateResolvedIP(host, ipAddress.IP); err != nil {
			return err
		}
	}
	return nil
}

// ValidateURLWithFetchSetting 使用FetchSetting配置验证URL
func ValidateURLWithFetchSetting(urlStr string, enableSSRFProtection, allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) error {
	if !enableSSRFProtection {
		return nil
	}
	protection, err := NewSSRFProtectionFromFetchSetting(allowPrivateIp, domainFilterMode, ipFilterMode, domainList, ipList, allowedPorts, applyIPFilterForDomain)
	if err != nil {
		return err
	}
	return protection.ValidateURL(urlStr)
}

func ValidateURLTargetWithFetchSetting(urlStr string, enableSSRFProtection, allowPrivateIp bool, domainFilterMode bool, ipFilterMode bool, domainList, ipList, allowedPorts []string, applyIPFilterForDomain bool) error {
	if !enableSSRFProtection {
		return nil
	}
	protection, err := NewSSRFProtectionFromFetchSetting(allowPrivateIp, domainFilterMode, ipFilterMode, domainList, ipList, allowedPorts, applyIPFilterForDomain)
	if err != nil {
		return err
	}
	return protection.ValidateURLTarget(urlStr)
}
