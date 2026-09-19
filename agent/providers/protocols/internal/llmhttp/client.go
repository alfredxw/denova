// Package llmhttp owns the default transport policy shared by model protocol
// adapters.
package llmhttp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/alfredxw/denova/agent/providers"
)

// Client preserves caller transport settings without mutating its client.
// The final SDK-serialized body is checked against known endpoint limits,
// including native media and protocol overrides. Custom endpoints own theirs.
// The fallback has no
// overall or response-header timeout; model execution is bounded only by the
// caller context, as required by Denova's long-running Agent contract.
func Client(configured *http.Client) *http.Client {
	client := &http.Client{}
	if configured != nil {
		*client = *configured
	}
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	client.Transport = requestLimitTransport{next: transport}
	return client
}

type requestLimitTransport struct{ next http.RoundTripper }

func (transport requestLimitTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var limit int64
	switch strings.ToLower(request.URL.Hostname()) {
	case "api.anthropic.com":
		limit = 32 << 20
	case "api.openai.com":
		limit = 512 << 20
	}
	if limit == 0 || request.Body == nil {
		return transport.next.RoundTrip(request)
	}
	size := request.ContentLength
	if size <= 0 {
		data, err := io.ReadAll(io.LimitReader(request.Body, limit+1))
		_ = request.Body.Close()
		if err != nil {
			return nil, err
		}
		request = request.Clone(request.Context())
		request.Body = io.NopCloser(bytes.NewReader(data))
		size = int64(len(data))
	}
	if size > limit {
		_ = request.Body.Close()
		return nil, &providers.APIError{StatusCode: http.StatusRequestEntityTooLarge, Kind: "request_too_large", Message: fmt.Sprintf("serialized model request exceeds endpoint byte limit: bytes=%d limit=%d", size, limit)}
	}
	return transport.next.RoundTrip(request)
}
