package webaccess

import (
	"net/netip"
	"testing"
)

func TestIsPublicAddress(t *testing.T) {
	tests := []struct {
		address string
		public  bool
	}{
		{address: "8.8.8.8", public: true},
		{address: "2606:4700:4700::1111", public: true},
		{address: "127.0.0.1", public: false},
		{address: "10.0.0.1", public: false},
		{address: "169.254.169.254", public: false},
		{address: "100.64.0.1", public: false},
		{address: "192.0.2.1", public: false},
		{address: "::1", public: false},
		{address: "fc00::1", public: false},
		{address: "2001:db8::1", public: false},
	}
	for _, test := range tests {
		t.Run(test.address, func(t *testing.T) {
			if got := isPublicAddress(netip.MustParseAddr(test.address)); got != test.public {
				t.Fatalf("isPublicAddress(%s) = %v, want %v", test.address, got, test.public)
			}
		})
	}
}
