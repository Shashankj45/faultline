package validator

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/faultline-proxy/faultline/pkg/models"
)

func TestCheckIPAllowed(t *testing.T) {
	tests := []struct {
		name           string
		ip             string
		allowLocalhost bool
		allowPrivate   bool
		wantErr        error
	}{
		{
			name:           "Metadata IP is always blocked",
			ip:             "169.254.169.254",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrMetadataBlocked,
		},
		{
			name:           "Link-local is always blocked",
			ip:             "169.254.10.20",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrLinkLocalBlocked,
		},
		{
			name:           "IPv4 loopback blocked by default",
			ip:             "127.0.0.1",
			allowLocalhost: false,
			allowPrivate:   true,
			wantErr:        ErrLocalhostDisallowed,
		},
		{
			name:           "IPv4 loopback allowed when explicit",
			ip:             "127.0.0.1",
			allowLocalhost: true,
			allowPrivate:   false,
			wantErr:        nil,
		},
		{
			name:           "IPv6 loopback blocked by default",
			ip:             "::1",
			allowLocalhost: false,
			allowPrivate:   true,
			wantErr:        ErrLocalhostDisallowed,
		},
		{
			name:           "Private RFC1918 10.x blocked by default",
			ip:             "10.0.1.50",
			allowLocalhost: true,
			allowPrivate:   false,
			wantErr:        ErrPrivateNetworkDisallowed,
		},
		{
			name:           "Private RFC1918 192.168.x allowed when explicit",
			ip:             "192.168.1.100",
			allowLocalhost: false,
			allowPrivate:   true,
			wantErr:        nil,
		},
		{
			name:           "Public IPv4 allowed",
			ip:             "93.184.216.34", // example.com
			allowLocalhost: false,
			allowPrivate:   false,
			wantErr:        nil,
		},
		{
			name:           "Unspecified IPv4 is always blocked",
			ip:             "0.0.0.0",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrUnspecifiedIPBlocked,
		},
		{
			name:           "Unspecified IPv6 is always blocked",
			ip:             "::",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrUnspecifiedIPBlocked,
		},
		{
			name:           "Azure Wireserver Metadata is always blocked",
			ip:             "168.63.129.16",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrMetadataBlocked,
		},
		{
			name:           "AWS IMDSv6 Metadata is always blocked",
			ip:             "fd00:ec2::254",
			allowLocalhost: true,
			allowPrivate:   true,
			wantErr:        ErrMetadataBlocked,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse IP %s", tt.ip)
			}
			err := CheckIPAllowed(ip, tt.allowLocalhost, tt.allowPrivate)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("expected error %v, got %v", tt.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidateTargetSchemes(t *testing.T) {
	v := NewDestinationValidator(nil)
	ctx := context.Background()

	invalidURLs := []string{
		"ftp://example.com",
		"file:///etc/passwd",
		"gopher://example.com",
		"://bad-url",
	}

	for _, raw := range invalidURLs {
		_, err := v.ValidateTarget(ctx, raw, false, false)
		if err == nil {
			t.Fatalf("expected error for URL %q, got nil", raw)
		}
	}
}

func TestSafeDialerPreventsRebinding(t *testing.T) {
	upstream := &models.Upstream{
		AllowLocalhost:      false,
		AllowPrivateNetwork: false,
	}

	dialer := SafeDialer(upstream, 1*time.Second)
	if dialer.Control == nil {
		t.Fatal("expected dialer.Control function to be populated")
	}

	// Test control with metadata address
	err := dialer.Control("tcp", "169.254.169.254:80", nil)
	if err == nil {
		t.Fatal("expected dialer control to reject metadata address")
	}

	// Test control with loopback address
	err = dialer.Control("tcp", "127.0.0.1:8080", nil)
	if err == nil {
		t.Fatal("expected dialer control to reject loopback address")
	}

	// Test control with AWS IMDSv6 address
	err = dialer.Control("tcp", "[fd00:ec2::254]:80", nil)
	if err == nil {
		t.Fatal("expected dialer control to reject AWS IMDSv6 address")
	}

	// Test control fails closed when socket address contains an unresolved hostname instead of IP
	err = dialer.Control("tcp", "unresolved.internal.domain:80", nil)
	if err == nil {
		t.Fatal("expected dialer control to fail closed on non-IP host")
	}
}
