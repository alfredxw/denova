package hostruntime

import (
	"reflect"
	"slices"
	"testing"
)

func TestProxyEnvironmentPrecedenceAndCase(t *testing.T) {
	system := []string{"HTTP_PROXY=http://system:7890", "HTTPS_PROXY=http://system:7890", "NO_PROXY=localhost"}
	inherited := []string{"PATH=tools", "CODEX_HOME=shared", "OPENAI_API_KEY=fixture", "HTTPS_PROXY=http://upper:9000", "https_proxy=http://lower:9001", "no_proxy="}
	before := slices.Clone(inherited)
	for _, platform := range []string{"windows", "darwin", "linux"} {
		t.Run(platform, func(t *testing.T) {
			want := map[string]string{"PATH": "tools", "CODEX_HOME": "shared", "OPENAI_API_KEY": "fixture", "HTTP_PROXY": "http://system:7890", "HTTPS_PROXY": "http://lower:9001", "NO_PROXY": ""}
			if platform != "windows" {
				want["http_proxy"], want["https_proxy"], want["no_proxy"] = want["HTTP_PROXY"], want["HTTPS_PROXY"], ""
			}
			if got := mergeProxyEnvironment(platform, system, inherited); !reflect.DeepEqual(got, sortedEnvironment(want)) {
				t.Fatalf("merged environment = %v, want %v", got, sortedEnvironment(want))
			}
			got := mergeProxyEnvironment(platform, system, inherited, []string{"https_proxy=http://explicit:9002", "HTTP_PROXY="})
			if environmentValue(got, "HTTPS_PROXY") != "http://explicit:9002" || environmentValue(got, "HTTP_PROXY") != "" {
				t.Fatalf("explicit overrides did not win: %v", got)
			}
		})
	}
	if !reflect.DeepEqual(inherited, before) {
		t.Fatal("proxy merge changed the inherited environment")
	}
}

func TestWindowsSystemProxy(t *testing.T) {
	for _, test := range []struct {
		name           string
		enabled        uint64
		server, bypass string
		want           map[string]string
	}{
		{"disabled", 0, "127.0.0.1:7890", "", nil},
		{"unset", 1, "", "", nil},
		{"shared endpoint", 1, "127.0.0.1:7890", "<local>;*.example.com;localhost", map[string]string{
			"HTTP_PROXY": "http://127.0.0.1:7890", "HTTPS_PROXY": "http://127.0.0.1:7890", "NO_PROXY": "localhost,127.0.0.1,::1,.local,.example.com",
		}},
		{"per protocol", 1, "http=proxy:80;https=[::1]:7890;socks=localhost:1080", "*", map[string]string{
			"HTTP_PROXY": "http://proxy:80", "HTTPS_PROXY": "http://[::1]:7890", "ALL_PROXY": "socks5://localhost:1080", "NO_PROXY": "localhost,127.0.0.1,::1,*",
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := environmentMap(windowsProxyEnvironment(test.enabled, test.server, test.bypass))
			if len(got) == 0 && len(test.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("proxy environment = %v, want %v", got, test.want)
			}
		})
	}
}

func TestMacSystemProxy(t *testing.T) {
	output := `<dictionary> {
  ExceptionsList : <array> {
    0 : *.example.com
    1 : localhost
  }
  HTTPEnable : 1
  HTTPProxy : 127.0.0.1
  HTTPPort : 7890
  HTTPSEnable : 0
  HTTPSProxy : ignored
  HTTPSPort : 9000
  SOCKSEnable : 1
  SOCKSProxy : ::1
  SOCKSPort : 1080
}`
	want := map[string]string{"HTTP_PROXY": "http://127.0.0.1:7890", "ALL_PROXY": "socks5://[::1]:1080", "NO_PROXY": "localhost,127.0.0.1,::1,.example.com"}
	if got := environmentMap(macProxyEnvironment(output)); !reflect.DeepEqual(got, want) {
		t.Fatalf("proxy environment = %v, want %v", got, want)
	}
	if got := macProxyEnvironment("<dictionary> {\n}\n"); len(got) != 0 {
		t.Fatalf("empty system configuration produced %v", got)
	}
}
