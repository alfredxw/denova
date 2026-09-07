package api

import (
	"encoding/json"
	"net/url"
	"testing"

	"denova/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestRemoteAccessAdvertisesBrowserEndpoint(t *testing.T) {
	_, h, _ := testRemoteAccess(t)
	for _, test := range []struct {
		name    string
		headers []ut.Header
		origin  string
	}{
		{"packaged backend", nil, config.LANHTTPURL(8080)},
		{"Vite frontend", []ut.Header{{Key: "X-Forwarded-Host", Value: "localhost:5173"}, {Key: "X-Forwarded-Proto", Value: "http"}}, config.LANHTTPURL(5173)},
		{"custom frontend port", []ut.Header{{Key: "X-Forwarded-Host", Value: "[::1]:15173"}, {Key: "X-Forwarded-Proto", Value: "http"}}, config.LANHTTPURL(15173)},
		{"HTTPS proxy", []ut.Header{{Key: "X-Forwarded-Host", Value: "localhost"}, {Key: "X-Forwarded-Proto", Value: "https"}}, "https://" + config.LANAddress()},
		{"invalid forwarded port", []ut.Header{{Key: "X-Forwarded-Host", Value: "localhost:99999"}}, config.LANHTTPURL(8080)},
	} {
		t.Run(test.name, func(t *testing.T) {
			status := ut.PerformRequest(h.Engine, "GET", "http://localhost:8080/api/auth/status", nil, test.headers...)
			var result struct {
				Local         bool   `json:"local"`
				Authenticated bool   `json:"authenticated"`
				LANURL        string `json:"lan_url"`
			}
			if err := json.Unmarshal(status.Body.Bytes(), &result); err != nil || status.Code != 200 || !result.Local || !result.Authenticated || result.LANURL != test.origin+"/" {
				t.Fatalf("status = %d %s, want LAN URL %s/", status.Code, status.Body.String(), test.origin)
			}
			link := ut.PerformRequest(h.Engine, "POST", "http://localhost:8080/api/auth/link", nil, test.headers...)
			var pairing struct {
				URL string `json:"url"`
			}
			if err := json.Unmarshal(link.Body.Bytes(), &pairing); err != nil || link.Code != 200 {
				t.Fatalf("link = %d %s", link.Code, link.Body.String())
			}
			parsed, err := url.Parse(pairing.URL)
			if err != nil {
				t.Fatal(err)
			}
			fragment, _ := url.ParseQuery(parsed.Fragment)
			if fragment.Get("pair") == "" {
				t.Fatalf("missing pairing token: %s", pairing.URL)
			}
			parsed.Fragment = ""
			if parsed.String() != result.LANURL {
				t.Fatalf("pairing endpoint = %s, want %s", parsed, result.LANURL)
			}
		})
	}
}
