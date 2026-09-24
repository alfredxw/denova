package hostruntime

import (
	"context"
	"log/slog"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"
)

// WithSystemProxy fills GUI-launch proxy defaults from the host OS. Explicit
// process values win, including empty values that disable a system default.
// It neither changes the parent environment nor persists machine-local paths.
func WithSystemProxy(ctx context.Context, environment []string) []string {
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	defaults, err := systemProxyEnvironment(probe)
	if err != nil {
		slog.WarnContext(ctx, "[hostruntime] system proxy lookup failed; using inherited environment", "error", err)
	}
	if len(defaults) > 0 {
		// Proxy URLs can contain credentials. Log only the source, never values.
		slog.InfoContext(ctx, "[hostruntime] applying system proxy defaults; inherited proxy values take precedence")
	}
	return mergeProxyEnvironment(runtime.GOOS, defaults, environment)
}

// Proxy keys are case-insensitive across layers. POSIX children receive both
// spellings; Windows receives one spelling to avoid ambiguous duplicate keys.
func mergeProxyEnvironment(platform string, sources ...[]string) []string {
	merged := make(map[string]string)
	for _, source := range sources {
		proxies := make(map[string]string)
		for _, entry := range source {
			key, value, ok := strings.Cut(entry, "=")
			if !ok || key == "" {
				continue
			}
			canonical := strings.ToUpper(key)
			switch canonical {
			case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
				// Match clients that prefer lowercase when both are present.
				if _, exists := proxies[canonical]; !exists || key == strings.ToLower(key) {
					proxies[canonical] = value
				}
			default:
				merged[key] = value
			}
		}
		for key, value := range proxies {
			merged[key] = value
			if platform != "windows" {
				merged[strings.ToLower(key)] = value
			}
		}
	}
	return sortedEnvironment(merged)
}

func windowsProxyEnvironment(enabled uint64, server, bypass string) []string {
	if enabled != 1 || strings.TrimSpace(server) == "" {
		return nil
	}
	proxies := make(map[string]string)
	if strings.Contains(server, "=") {
		for _, entry := range strings.Split(server, ";") {
			kind, value, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(kind)) {
			case "http":
				proxies["HTTP_PROXY"] = proxyURL(value, "http")
			case "https":
				proxies["HTTPS_PROXY"] = proxyURL(value, "http")
			case "socks":
				proxies["ALL_PROXY"] = proxyURL(value, "socks5")
			}
		}
	} else {
		proxies["HTTP_PROXY"] = proxyURL(server, "http")
		proxies["HTTPS_PROXY"] = proxies["HTTP_PROXY"]
	}
	return systemProxyValues(proxies, strings.FieldsFunc(bypass, func(r rune) bool { return r == ';' || r == ',' }))
}

func macProxyEnvironment(output string) []string {
	values := make(map[string]string)
	var bypass []string
	inExceptions := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ExceptionsList :") {
			inExceptions = true
			continue
		}
		if inExceptions && line == "}" {
			inExceptions = false
			continue
		}
		key, value, ok := strings.Cut(line, " : ")
		if !ok {
			continue
		}
		if inExceptions {
			bypass = append(bypass, value)
		} else {
			values[key] = value
		}
	}
	proxies := make(map[string]string)
	for _, entry := range []struct{ prefix, key, scheme string }{{"HTTP", "HTTP_PROXY", "http"}, {"HTTPS", "HTTPS_PROXY", "http"}, {"SOCKS", "ALL_PROXY", "socks5"}} {
		if values[entry.prefix+"Enable"] == "1" && values[entry.prefix+"Proxy"] != "" && values[entry.prefix+"Port"] != "" {
			proxies[entry.key] = proxyURL(net.JoinHostPort(strings.Trim(values[entry.prefix+"Proxy"], "[]"), values[entry.prefix+"Port"]), entry.scheme)
		}
	}
	return systemProxyValues(proxies, bypass)
}

func proxyURL(value, scheme string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = scheme + "://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return value
}

func systemProxyValues(proxies map[string]string, bypass []string) []string {
	for key, value := range proxies {
		if value == "" {
			delete(proxies, key)
		}
	}
	if len(proxies) == 0 {
		return nil
	}
	// Local app-server and model fixtures must stay reachable without a proxy.
	items := []string{"localhost", "127.0.0.1", "::1"}
	seen := map[string]bool{"localhost": true, "127.0.0.1": true, "::1": true}
	for _, item := range bypass {
		item = strings.TrimSpace(item)
		if item == "<local>" {
			item = ".local"
		}
		if strings.HasPrefix(item, "*.") {
			item = strings.TrimPrefix(item, "*")
		}
		if item != "" && !seen[item] {
			seen[item] = true
			items = append(items, item)
		}
	}
	proxies["NO_PROXY"] = strings.Join(items, ",")
	return sortedEnvironment(proxies)
}
