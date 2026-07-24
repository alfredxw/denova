package webaccess

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var blockedPublicFetchPrefixes = []netip.Prefix{
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

type netIPResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

type publicDialer struct {
	resolver netIPResolver
	dialer   *net.Dialer
}

// publicFetchPolicyError marks a destination rejected before any network
// request is sent. Callers must not forward these URLs to hosted or browser
// fallbacks because doing so would bypass Denova's public-network boundary.
type publicFetchPolicyError struct {
	message string
}

func (fetchError *publicFetchPolicyError) Error() string {
	return fetchError.message
}

func isPublicFetchPolicyError(err error) bool {
	var policyError *publicFetchPolicyError
	return errors.As(err, &policyError)
}

func newUnboundedHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{}).DialContext
	transport.TLSHandshakeTimeout = 0
	transport.ResponseHeaderTimeout = 0
	transport.ExpectContinueTimeout = 0
	return &http.Client{Transport: transport}
}

func newPublicHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Public-page fetching bypasses environment proxies because a proxy can
	// obscure the actual destination and invalidate the dial-time SSRF check.
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

func (dialer *publicDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("parse fetch destination %q: %w", address, err)
	}
	addresses, err := dialer.resolve(ctx, network, host)
	if err != nil {
		return nil, err
	}
	var dialErrors []error
	for _, address := range addresses {
		connection, dialErr := dialer.dialer.DialContext(ctx, network, net.JoinHostPort(address.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		dialErrors = append(dialErrors, dialErr)
	}
	if len(dialErrors) == 0 {
		return nil, fmt.Errorf("fetch destination %q resolved to no usable addresses", host)
	}
	return nil, fmt.Errorf("dial public fetch destination %q: %w", host, errors.Join(dialErrors...))
}

func (dialer *publicDialer) resolve(ctx context.Context, network, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, fmt.Errorf("fetch destination host is empty")
	}
	var addresses []netip.Addr
	if parsed, err := netip.ParseAddr(host); err == nil {
		addresses = []netip.Addr{parsed}
	} else {
		resolved, resolveErr := dialer.resolver.LookupNetIP(ctx, "ip", host)
		if resolveErr != nil {
			return nil, fmt.Errorf("resolve fetch destination %q: %w", host, resolveErr)
		}
		addresses = resolved
	}
	usable := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if !isPublicAddress(address) {
			return nil, &publicFetchPolicyError{message: fmt.Sprintf("fetch destination %q resolves to blocked address %s", host, address)}
		}
		if network == "tcp4" && !address.Is4() {
			continue
		}
		if network == "tcp6" && !address.Is6() {
			continue
		}
		usable = append(usable, address)
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
	for _, prefix := range blockedPublicFetchPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}
