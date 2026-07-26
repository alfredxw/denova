package browser

import (
	"context"
	"net/netip"
	"strings"
	"testing"
)

type staticNetIPResolver struct {
	addresses []netip.Addr
}

func (resolver staticNetIPResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return append([]netip.Addr(nil), resolver.addresses...), nil
}

func TestResolvePublicAddressesRejectsIPv4HiddenInTransitionAddresses(t *testing.T) {
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
			_, err := resolvePublicAddresses(context.Background(), staticNetIPResolver{
				addresses: []netip.Addr{test.address},
			}, "ip", "attacker.example")
			if err == nil || !strings.Contains(err.Error(), "blocked address") {
				t.Fatalf("transition address %s was accepted: %v", test.address, err)
			}
		})
	}
}

func TestPublicAddressPolicyAcceptsPublicIPv4TransitionMappings(t *testing.T) {
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
			if !isPublicAddress(test.address) {
				t.Fatalf("public transition mapping %s was rejected", test.address)
			}
		})
	}
}

func TestPublicAddressPolicyRejectsMalformedNAT64LocalUseEncoding(t *testing.T) {
	address := nat64LocalUseAddress(netip.MustParseAddr("8.8.8.8"))
	bytes := address.As16()
	bytes[8] = 1 // RFC 6052 reserves this as the zero "u" octet.
	malformed := netip.AddrFrom16(bytes)
	if isPublicAddress(malformed) {
		t.Fatalf("malformed NAT64 local-use address %s was accepted", malformed)
	}
}

func nat64WellKnownAddress(ipv4 netip.Addr) netip.Addr {
	bytes := nat64WellKnownPrefix.Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[12:], value[:])
	return netip.AddrFrom16(bytes)
}

func nat64LocalUseAddress(ipv4 netip.Addr) netip.Addr {
	bytes := nat64LocalUsePrefix.Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[6:8], value[:2])
	bytes[8] = 0
	copy(bytes[9:11], value[2:])
	return netip.AddrFrom16(bytes)
}

func sixToFourAddress(ipv4 netip.Addr) netip.Addr {
	bytes := sixToFourPrefix.Addr().As16()
	value := ipv4.Unmap().As4()
	copy(bytes[2:6], value[:])
	return netip.AddrFrom16(bytes)
}

func teredoAddress(serverIPv4, clientIPv4 netip.Addr) netip.Addr {
	bytes := teredoPrefix.Addr().As16()
	server := serverIPv4.Unmap().As4()
	client := clientIPv4.Unmap().As4()
	copy(bytes[4:8], server[:])
	bytes[8], bytes[9] = 0, 0         // flags
	bytes[10], bytes[11] = 0xed, 0xcb // an arbitrary obfuscated UDP port
	for index := range client {
		bytes[12+index] = ^client[index]
	}
	return netip.AddrFrom16(bytes)
}
