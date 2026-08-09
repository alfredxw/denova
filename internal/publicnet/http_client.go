// Package publicnet owns Denova's public-Internet destination policy.
// Callers use the same validation and dial-time enforcement so redirects,
// DNS rebinding, and IPv4 addresses embedded in IPv6 transports cannot bypass
// the private-network boundary.
package publicnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
}

var (
	nat64WellKnownPrefix = netip.MustParsePrefix("64:ff9b::/96")
	nat64LocalUsePrefix  = netip.MustParsePrefix("64:ff9b:1::/48")
	sixToFourPrefix      = netip.MustParsePrefix("2002::/16")
	teredoPrefix         = netip.MustParsePrefix("2001::/32")
)

type netIPResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type publicDialer struct {
	resolver netIPResolver
	dialer   *net.Dialer
}

type policyError struct {
	message string
}

func (policyErr *policyError) Error() string {
	return policyErr.message
}

// IsPolicyError reports whether err was caused by a destination rejected by
// the public-network policy. Callers must not retry such a URL through another
// transport because doing so would bypass the same security boundary.
func IsPolicyError(err error) bool {
	var policyErr *policyError
	return errors.As(err, &policyErr)
}

// NewHTTPClient returns an unbounded client that only dials public Internet
// destinations. Proxy lookup is disabled because a proxy obscures the actual
// destination from the dial-time policy check.
func NewHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&publicDialer{
		resolver: net.DefaultResolver,
		dialer:   &net.Dialer{},
	}).DialContext
	transport.TLSHandshakeTimeout = 0
	transport.ResponseHeaderTimeout = 0
	transport.ExpectContinueTimeout = 0
	return &http.Client{Transport: transport}
}

// ValidateHost resolves host and rejects any address outside the public
// Internet. Network clients must still use NewHTTPClient so every connection,
// including redirects and subresources, repeats the check at dial time.
func ValidateHost(ctx context.Context, host string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := resolvePublicAddresses(ctx, net.DefaultResolver, "ip", host)
	return err
}

func (dialer *publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse public destination %q: %w", address, err)
	}
	addresses, err := resolvePublicAddresses(ctx, dialer.resolver, network, host)
	if err != nil {
		return nil, err
	}
	var dialErrors []error
	for _, resolved := range addresses {
		connection, dialErr := dialer.dialer.DialContext(ctx, network, net.JoinHostPort(resolved.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, dialErr)
	}
	if len(dialErrors) == 0 {
		return nil, fmt.Errorf("public destination %q resolved to no usable addresses", host)
	}
	return nil, fmt.Errorf("dial public destination %q: %w", host, errors.Join(dialErrors...))
}

func resolvePublicAddresses(ctx context.Context, resolver netIPResolver, network, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, errors.New("public destination host is empty")
	}
	var addresses []netip.Addr
	if parsed, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{parsed}
	} else {
		resolved, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve public destination %q: %w", host, err)
		}
		addresses = resolved
	}
	usable := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !isPublicAddress(address) {
			return nil, &policyError{message: fmt.Sprintf("public destination %q resolves to blocked address %s", host, address)}
		}
		if network == "tcp4" && !address.Is4() {
			continue
		}
		if network == "tcp6" && !address.Is6() {
			continue
		}
		usable = append(usable, address)
	}
	if len(usable) == 0 {
		return nil, fmt.Errorf("public destination %q resolved to no public addresses", host)
	}
	return usable, nil
}

func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() ||
		address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsLinkLocalMulticast() ||
		address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	embedded, transition, valid := transitionIPv4Addresses(address)
	if transition {
		if !valid {
			return false
		}
		for _, ipv4 := range embedded {
			if !isPublicAddress(ipv4) {
				return false
			}
		}
	}
	return true
}

// transitionIPv4Addresses extracts every IPv4 endpoint that determines the
// destination of an automatic IPv6 translation or tunnel address. matched is
// true for a recognized transition prefix; valid is false for a malformed
// encoding, which callers reject rather than treating as ordinary IPv6.
func transitionIPv4Addresses(address netip.Addr) (embedded []netip.Addr, matched, valid bool) {
	if !address.Is6() || address.Is4In6() {
		return nil, false, false
	}
	bytes := address.As16()
	switch {
	case nat64WellKnownPrefix.Contains(address):
		return []netip.Addr{ipv4FromBytes(bytes[12], bytes[13], bytes[14], bytes[15])}, true, true
	case nat64LocalUsePrefix.Contains(address):
		// RFC 6052's /48 layout places the high 16 IPv4 bits at 48..63,
		// reserves bits 64..71 as the zero "u" octet, and places the low
		// 16 bits at 72..87. RFC 8215 reserves this /48 for local translators.
		if bytes[8] != 0 {
			return nil, true, false
		}
		return []netip.Addr{ipv4FromBytes(bytes[6], bytes[7], bytes[9], bytes[10])}, true, true
	case sixToFourPrefix.Contains(address):
		return []netip.Addr{ipv4FromBytes(bytes[2], bytes[3], bytes[4], bytes[5])}, true, true
	case teredoPrefix.Contains(address):
		// Teredo carries the server IPv4 directly at bits 32..63 and the
		// client IPv4, XOR-obfuscated with all ones, at bits 96..127.
		server := ipv4FromBytes(bytes[4], bytes[5], bytes[6], bytes[7])
		client := ipv4FromBytes(^bytes[12], ^bytes[13], ^bytes[14], ^bytes[15])
		return []netip.Addr{server, client}, true, true
	default:
		return nil, false, false
	}
}

func ipv4FromBytes(first, second, third, fourth byte) netip.Addr {
	return netip.AddrFrom4([4]byte{first, second, third, fourth})
}
