package publicnet_test

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"denova/internal/publicnet"
)

func TestValidateHostRejectsIPv4HiddenInTransitionAddresses(t *testing.T) {
	tests := []struct {
		name    string
		address netip.Addr
	}{
		{name: "nat64 well-known metadata", address: nat64WellKnownAddress(netip.MustParseAddr("169.254.169.254"))},
		{name: "nat64 local-use loopback", address: nat64LocalUseAddress(netip.MustParseAddr("127.0.0.1"))},
		{name: "6to4 RFC1918", address: sixToFourAddress(netip.MustParseAddr("10.0.0.1"))},
		{name: "Teredo metadata client", address: teredoAddress(netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("169.254.169.254"))},
		{name: "Teredo loopback server", address: teredoAddress(netip.MustParseAddr("127.0.0.1"), netip.MustParseAddr("1.1.1.1"))},
		{name: "Teredo RFC1918 client", address: teredoAddress(netip.MustParseAddr("8.8.4.4"), netip.MustParseAddr("192.168.1.10"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := publicnet.ValidateHost(context.Background(), test.address.String())
			if err == nil || !publicnet.IsPolicyError(err) || !strings.Contains(err.Error(), "blocked address") {
				t.Fatalf("transition address %s was accepted: %v", test.address, err)
			}
		})
	}
}

func TestHTTPClientRejectsIPv4HiddenInTransitionAddress(t *testing.T) {
	response, err := publicnet.NewHTTPClient().Get("http://[64:ff9b::a9fe:a9fe]/")
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil || !publicnet.IsPolicyError(err) || !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("public HTTP client accepted NAT64 metadata address: response=%v err=%v", response, err)
	}
}

func TestValidateHostAcceptsPublicIPv4TransitionMappings(t *testing.T) {
	tests := []struct {
		name    string
		address netip.Addr
	}{
		{name: "nat64 well-known", address: nat64WellKnownAddress(netip.MustParseAddr("8.8.8.8"))},
		{name: "nat64 local-use", address: nat64LocalUseAddress(netip.MustParseAddr("1.1.1.1"))},
		{name: "6to4", address: sixToFourAddress(netip.MustParseAddr("8.8.4.4"))},
		{name: "Teredo", address: teredoAddress(netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1"))},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := publicnet.ValidateHost(context.Background(), test.address.String()); err != nil {
				t.Fatalf("public transition mapping %s was rejected: %v", test.address, err)
			}
		})
	}
}

func TestValidateHostRejectsMalformedNAT64LocalUseEncoding(t *testing.T) {
	address := nat64LocalUseAddress(netip.MustParseAddr("8.8.8.8"))
	bytes := address.As16()
	bytes[8] = 1
	malformed := netip.AddrFrom16(bytes)
	err := publicnet.ValidateHost(context.Background(), malformed.String())
	if err == nil || !publicnet.IsPolicyError(err) {
		t.Fatalf("malformed NAT64 local-use address %s was accepted: %v", malformed, err)
	}
}

func TestValidateHostAppliesPublicAddressPolicy(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{address: "8.8.8.8", allowed: true},
		{address: "2606:4700:4700::1111", allowed: true},
		{address: "127.0.0.1"},
		{address: "10.0.0.1"},
		{address: "169.254.169.254"},
		{address: "100.64.0.1"},
		{address: "192.0.2.1"},
		{address: "::1"},
		{address: "fc00::1"},
		{address: "2001:db8::1"},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			err := publicnet.ValidateHost(context.Background(), test.address)
			if (err == nil) != test.allowed {
				t.Fatalf("ValidateHost(%s) error = %v, allowed = %v", test.address, err, test.allowed)
			}
		})
	}
}

func nat64WellKnownAddress(ipv4 netip.Addr) netip.Addr {
	bytes := netip.MustParsePrefix("64:ff9b::/96").Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[12:], value[:])
	return netip.AddrFrom16(bytes)
}

func nat64LocalUseAddress(ipv4 netip.Addr) netip.Addr {
	bytes := netip.MustParsePrefix("64:ff9b:1::/48").Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[6:8], value[:2])
	bytes[8] = 0
	copy(bytes[9:11], value[2:])
	return netip.AddrFrom16(bytes)
}

func sixToFourAddress(ipv4 netip.Addr) netip.Addr {
	bytes := netip.MustParsePrefix("2002::/16").Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[2:6], value[:])
	return netip.AddrFrom16(bytes)
}

func teredoAddress(serverIPv4, clientIPv4 netip.Addr) netip.Addr {
	bytes := netip.MustParsePrefix("2001::/32").Addr().As16()
	server := serverIPv4.Unmap().As4()
	client := clientIPv4.Unmap().As4()
	copy(bytes[4:8], server[:])
	bytes[8], bytes[9] = 0, 0
	bytes[10], bytes[11] = 0xed, 0xcb
	for index := range client {
		bytes[12+index] = ^client[index]
	}
	return netip.AddrFrom16(bytes)
}
