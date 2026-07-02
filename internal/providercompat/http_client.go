package providercompat

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// WrapHTTPClient returns an HTTP client that filters SSE heartbeat-only lines
// before the OpenAI-compatible SDK reads the stream. Some providers keep long
// reasoning requests alive with empty/comment/event metadata lines; go-openai
// treats too many of those lines as a broken stream before real data arrives.
func WrapHTTPClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	clone := *client
	if _, ok := clone.Transport.(*sseHeartbeatFilterTransport); ok {
		return &clone
	}
	base := clone.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	clone.Transport = &sseHeartbeatFilterTransport{base: base}
	return &clone
}

type sseHeartbeatFilterTransport struct {
	base http.RoundTripper
}

func (t *sseHeartbeatFilterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	if !shouldFilterSSEResponse(req, resp) {
		return resp, nil
	}
	resp.Body = newSSEHeartbeatFilteringBody(resp.Body)
	return resp, nil
}

func shouldFilterSSEResponse(req *http.Request, resp *http.Response) bool {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return true
	}
	return strings.Contains(strings.ToLower(req.Header.Get("Accept")), "text/event-stream")
}

func newSSEHeartbeatFilteringBody(source io.ReadCloser) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = pw.CloseWithError(fmt.Errorf("providercompat SSE heartbeat filter panic: %v", recovered))
			}
			_ = source.Close()
		}()
		reader := bufio.NewReader(source)
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 && !isSSEHeartbeatLine(line) {
				if _, writeErr := pw.Write(line); writeErr != nil {
					_ = pw.CloseWithError(writeErr)
					return
				}
			}
			if readErr != nil {
				if readErr == io.EOF {
					_ = pw.Close()
					return
				}
				_ = pw.CloseWithError(readErr)
				return
			}
		}
	}()
	return &sseHeartbeatFilteringBody{PipeReader: pr, source: source}
}

type sseHeartbeatFilteringBody struct {
	*io.PipeReader
	source io.Closer
}

func (b *sseHeartbeatFilteringBody) Close() error {
	pipeErr := b.PipeReader.Close()
	sourceErr := b.source.Close()
	if pipeErr != nil {
		return pipeErr
	}
	return sourceErr
}

func isSSEHeartbeatLine(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return true
	}
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte(":")) {
		return true
	}
	for _, prefix := range [][]byte{
		[]byte("event:"),
		[]byte("id:"),
		[]byte("retry:"),
	} {
		if bytes.HasPrefix(lower, prefix) {
			return true
		}
	}
	return bytes.Equal(lower, []byte("ping")) || bytes.Equal(lower, []byte("keep-alive"))
}
