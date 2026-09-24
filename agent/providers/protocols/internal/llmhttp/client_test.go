package llmhttp

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/alfredxw/denova/agent/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestClientChecksWireBytesWithoutChangingCaller(t *testing.T) {
	calls := 0
	configured := &http.Client{Timeout: 5 * time.Second, Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(r.Body)
		_ = r.Body.Close()
		if err != nil || len(body) != 6 {
			t.Fatalf("transport body changed: %q, %v", body, err)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
	})}
	client := Client(configured)
	if client == configured || client.Timeout != configured.Timeout {
		t.Fatal("caller client was replaced or mutated")
	}
	for _, tc := range []struct {
		host     string
		size     int64
		rejected bool
	}{
		{"api.anthropic.com", 6, false}, {"api.anthropic.com", -1, false},
		{"api.anthropic.com", (32 << 20) + 1, true}, {"api.openai.com", (512 << 20) + 1, true},
		{"gateway.example", (32 << 20) + 1, false},
	} {
		request, _ := http.NewRequest(http.MethodPost, "https://"+tc.host+"/v1/messages", strings.NewReader("images"))
		request.ContentLength = tc.size
		before := calls
		response, err := client.Do(request)
		if tc.rejected {
			var apiError *providers.APIError
			if !errors.As(err, &apiError) || apiError.StatusCode != 413 || calls != before {
				t.Fatalf("wire limit did not reject before I/O: %v", err)
			}
		} else {
			if err != nil || calls != before+1 {
				t.Fatalf("valid request rejected: %v", err)
			}
			_ = response.Body.Close()
		}
	}
}
