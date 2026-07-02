package providercompat

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWrapHTTPClientFiltersSSEHeartbeatLines(t *testing.T) {
	client := WrapHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body strings.Builder
		for range 301 {
			body.WriteString("\n")
			body.WriteString(": ping\n")
			body.WriteString("event: ping\n")
			body.WriteString("id: heartbeat\n")
			body.WriteString("retry: 1000\n")
			body.WriteString("ping\n")
			body.WriteString("keep-alive\n")
		}
		body.WriteString("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body.String())),
			Request:    req,
		}, nil
	})})

	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n"; got != want {
		t.Fatalf("filtered stream = %q, want %q", got, want)
	}
}

func TestWrapHTTPClientKeepsUnknownSSELinesForErrorVisibility(t *testing.T) {
	client := WrapHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("upstream overloaded\n")),
			Request:    req,
		}, nil
	})})

	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), "upstream overloaded\n"; got != want {
		t.Fatalf("filtered stream = %q, want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
