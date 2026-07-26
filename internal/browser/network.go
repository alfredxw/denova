package browser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"
)

const maxBrowserURLRunes = 4096

var blockedPublicPrefixes = []netip.Prefix{
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

// ValidatePublicURL rejects credentials, non-HTTP schemes, and destinations
// that currently resolve outside the public Internet. RodDriver repeats this
// boundary at dial time for every top-level and subresource request.
func ValidatePublicURL(ctx context.Context, raw string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("browser URL is required")
	}
	if utf8.RuneCountInString(raw) > maxBrowserURLRunes {
		return "", fmt.Errorf("browser URL exceeds %d characters", maxBrowserURLRunes)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse browser URL: %w", err)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("browser URL must use http or https")
	}
	if parsed.Hostname() == "" {
		return "", errors.New("browser URL must include a host")
	}
	if parsed.User != nil {
		return "", errors.New("browser URL must not include user credentials")
	}
	if _, err := resolvePublicAddresses(ctx, net.DefaultResolver, "ip", parsed.Hostname()); err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func newPublicHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = (&publicDialer{resolver: net.DefaultResolver, dialer: &net.Dialer{}}).DialContext
	transport.TLSHandshakeTimeout = 0
	transport.ResponseHeaderTimeout = 0
	transport.ExpectContinueTimeout = 0
	return &http.Client{Transport: transport}
}

func (dialer *publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse browser destination %q: %w", address, err)
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
		return nil, fmt.Errorf("browser destination %q resolved to no usable addresses", host)
	}
	return nil, fmt.Errorf("dial public browser destination %q: %w", host, errors.Join(dialErrors...))
}

func resolvePublicAddresses(ctx context.Context, resolver netIPResolver, network, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, errors.New("browser destination host is empty")
	}
	var addresses []netip.Addr
	if parsed, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{parsed}
	} else {
		resolved, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve browser destination %q: %w", host, err)
		}
		addresses = resolved
	}
	usable := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !isPublicAddress(address) {
			return nil, fmt.Errorf("browser destination %q resolves to blocked address %s", host, address)
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
		return nil, fmt.Errorf("browser destination %q resolved to no public addresses", host)
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
	for _, prefix := range blockedPublicPrefixes {
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
			// Apply the same policy used for direct IPv4 destinations. This rejects
			// loopback, link-local metadata endpoints, RFC 1918, documentation,
			// benchmark, multicast, and reserved ranges hidden in IPv6 transports.
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
// encoding, which callers must reject rather than treat as ordinary IPv6.
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
		// A 6to4 route is 2002:V4ADDR::/48 (RFC 3056).
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
