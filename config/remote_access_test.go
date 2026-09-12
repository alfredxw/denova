package config

import (
	"net"
	"testing"
)

func TestSelectLANAddress(t *testing.T) {
	for _, test := range []struct {
		name string
		ips  []string
		want string
	}{
		{"proxy before WLAN", []string{"198.18.0.1", "192.168.3.22"}, "192.168.3.22"},
		{"benchmark range", []string{"198.19.255.254", "10.0.0.12"}, "10.0.0.12"},
		{"private before public", []string{"8.8.4.4", "172.16.1.2"}, "172.16.1.2"},
		{"unusable addresses", []string{"127.0.0.1", "0.0.0.0", "169.254.18.199", "224.0.0.1", "255.255.255.255", "::1", "fe80::1", "192.168.3.22"}, "192.168.3.22"},
		{"public interface fallback", []string{"198.18.0.1", "8.8.4.4"}, "8.8.4.4"},
		{"no usable interface", []string{"198.18.0.1", "198.19.0.1", "169.254.18.199"}, LANHTTPHost},
		{"no interfaces", nil, LANHTTPHost},
	} {
		t.Run(test.name, func(t *testing.T) {
			var addrs []net.Addr
			for _, address := range test.ips {
				addrs = append(addrs, &net.IPNet{IP: net.ParseIP(address)})
			}
			if got := selectLANAddress(addrs); got != test.want {
				t.Fatalf("selected LAN address = %q, want %q", got, test.want)
			}
		})
	}
}
