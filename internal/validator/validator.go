package validator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
)

var (
	ErrInvalidScheme            = errors.New("invalid scheme: only http and https are allowed")
	ErrEmptyHost                = errors.New("upstream host cannot be empty")
	ErrInvalidPort              = errors.New("upstream port must be between 1 and 65535")
	ErrMetadataBlocked          = errors.New("security violation: cloud metadata endpoint is permanently prohibited")
	ErrLinkLocalBlocked         = errors.New("security violation: link-local IP addresses are permanently prohibited")
	ErrLocalhostDisallowed      = errors.New("security policy: loopback / localhost is not permitted for this upstream")
	ErrPrivateNetworkDisallowed = errors.New("security policy: private RFC 1918 / unique-local networks are not permitted for this upstream")
	ErrNoIPResolved             = errors.New("no IP addresses resolved for upstream host")
)

var (
	// Metadata IP (AWS, GCP, Azure, DigitalOcean)
	metadataIP = net.ParseIP("169.254.169.254")

	// Disallowed IP CIDRs
	linkLocalCIDRs = []*net.IPNet{
		mustParseCIDR("169.254.0.0/16"),
		mustParseCIDR("fe80::/10"),
	}

	loopbackCIDRs = []*net.IPNet{
		mustParseCIDR("127.0.0.0/8"),
		mustParseCIDR("::1/128"),
	}

	privateCIDRs = []*net.IPNet{
		mustParseCIDR("10.0.0.0/8"),
		mustParseCIDR("172.16.0.0/12"),
		mustParseCIDR("192.168.0.0/16"),
		mustParseCIDR("100.64.0.0/10"), // Shared Address Space / Carrier Grade NAT
		mustParseCIDR("fc00::/7"),      // IPv6 Unique Local
	}

	multicastBroadcastCIDRs = []*net.IPNet{
		mustParseCIDR("224.0.0.0/4"),
		mustParseCIDR("255.255.255.255/32"),
		mustParseCIDR("ff00::/8"),
	}
)

func mustParseCIDR(s string) *net.IPNet {
	_, cidr, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("failed to parse CIDR %q: %v", s, err))
	}
	return cidr
}

// DestinationValidator enforces SSRF policies and validates upstream targets.
type DestinationValidator struct {
	Resolver *net.Resolver
}

// NewDestinationValidator creates a new validator with custom or default resolver.
func NewDestinationValidator(resolver *net.Resolver) *DestinationValidator {
	if resolver == nil {
		resolver = &net.Resolver{
			PreferGo: true,
		}
	}
	return &DestinationValidator{
		Resolver: resolver,
	}
}

// ValidateTarget verifies an upstream URL against SSRF rules.
func (v *DestinationValidator) ValidateTarget(ctx context.Context, targetURL string, allowLocalhost, allowPrivate bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return nil, fmt.Errorf("malformed URL: %w", err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidScheme
	}

	host := u.Hostname()
	if host == "" {
		return nil, ErrEmptyHost
	}

	portStr := u.Port()
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			return nil, ErrInvalidPort
		}
	}

	// Resolve host IPs
	ips, err := v.Resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns resolution failed for %q: %w", host, err)
	}

	if len(ips) == 0 {
		return nil, ErrNoIPResolved
	}

	for _, ipAddr := range ips {
		if err := CheckIPAllowed(ipAddr.IP, allowLocalhost, allowPrivate); err != nil {
			return nil, fmt.Errorf("ip %s failed validation: %w", ipAddr.IP.String(), err)
		}
	}

	return u, nil
}

// CheckIPAllowed checks an individual IP against SSRF CIDRs and policies.
func CheckIPAllowed(ip net.IP, allowLocalhost, allowPrivate bool) error {
	// Normalize IPv4-mapped IPv6 addresses (::ffff:127.0.0.1 -> 127.0.0.1)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	// 1. Cloud Metadata protection (permanent block)
	if ip.Equal(metadataIP) {
		return ErrMetadataBlocked
	}

	// 2. Link-local protection (permanent block)
	for _, cidr := range linkLocalCIDRs {
		if cidr.Contains(ip) {
			return ErrLinkLocalBlocked
		}
	}

	// 3. Multicast / Broadcast protection
	for _, cidr := range multicastBroadcastCIDRs {
		if cidr.Contains(ip) {
			return errors.New("security violation: multicast or broadcast IP is not allowed")
		}
	}

	// 4. Loopback / Localhost validation
	for _, cidr := range loopbackCIDRs {
		if cidr.Contains(ip) {
			if !allowLocalhost {
				return ErrLocalhostDisallowed
			}
			return nil
		}
	}

	// 5. Private network validation
	for _, cidr := range privateCIDRs {
		if cidr.Contains(ip) {
			if !allowPrivate {
				return ErrPrivateNetworkDisallowed
			}
			return nil
		}
	}

	return nil
}

// SafeDialer creates a net.Dialer that prevents DNS Rebinding attacks by re-checking
// the destination IP at TCP connect time inside Control.
func SafeDialer(upstream *models.Upstream, timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("invalid socket address %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip != nil {
				if err := CheckIPAllowed(ip, upstream.AllowLocalhost, upstream.AllowPrivateNetwork); err != nil {
					return fmt.Errorf("dns rebinding / SSRF blocked at socket connection: %w", err)
				}
			}
			return nil
		},
	}
}
