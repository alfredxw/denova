package compat

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
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
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
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
	body := &sseHeartbeatFilteringBody{PipeReader: pr, source: source}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				_ = pw.CloseWithError(fmt.Errorf("providercompat SSE heartbeat filter panic: %v", recovered))
			}
			_ = body.closeSource()
		}()
		reader := bufio.NewReader(source)
		var event bytes.Buffer
		flushEvent := func(delimiter []byte) error {
			if event.Len() == 0 {
				return nil
			}
			filtered := filterSSEEvent(event.Bytes(), len(delimiter) > 0)
			if len(filtered) > 0 {
				if _, err := pw.Write(filtered); err != nil {
					return err
				}
				// The blank line is part of the SSE protocol: it dispatches the
				// buffered data event. Dropping it makes a valid provider stream
				// look like a successful response with zero frames.
				if len(delimiter) > 0 {
					if _, err := pw.Write(delimiter); err != nil {
						return err
					}
				}
			}
			event.Reset()
			return nil
		}
		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				if len(bytes.TrimSpace(line)) == 0 && bytes.HasSuffix(line, []byte{'\n'}) {
					if writeErr := flushEvent(normalizeSSEDelimiter(line)); writeErr != nil {
						_ = pw.CloseWithError(writeErr)
						return
					}
				} else if _, writeErr := event.Write(line); writeErr != nil {
					_ = pw.CloseWithError(writeErr)
					return
				}
			}
			if readErr != nil {
				if writeErr := flushEvent(nil); writeErr != nil {
					_ = pw.CloseWithError(writeErr)
					return
				}
				if readErr == io.EOF {
					_ = pw.Close()
					return
				}
				_ = pw.CloseWithError(readErr)
				return
			}
		}
	}()
	return body
}

type sseHeartbeatFilteringBody struct {
	*io.PipeReader
	source    io.Closer
	closeOnce sync.Once
	closeErr  error
}

func (b *sseHeartbeatFilteringBody) Close() error {
	pipeErr := b.PipeReader.Close()
	sourceErr := b.closeSource()
	if pipeErr != nil {
		return pipeErr
	}
	return sourceErr
}

func (b *sseHeartbeatFilteringBody) closeSource() error {
	b.closeOnce.Do(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				b.closeErr = fmt.Errorf("providercompat SSE source close panic: %v", recovered)
			}
		}()
		b.closeErr = b.source.Close()
	})
	return b.closeErr
}

func filterSSEEvent(event []byte, delimited bool) []byte {
	var filtered bytes.Buffer
	var data bytes.Buffer
	eventType := ""
	hasData := false
	for _, line := range bytes.SplitAfter(event, []byte{'\n'}) {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			continue
		}
		name, value, found := bytes.Cut(trimmed, []byte{':'})
		if !found {
			continue
		}
		value = bytes.TrimPrefix(value, []byte{' '})
		switch strings.ToLower(string(name)) {
		case "event":
			eventType = strings.ToLower(strings.TrimSpace(string(value)))
			_, _ = filtered.Write(line)
		case "data":
			hasData = true
			_, _ = data.Write(value)
			_ = data.WriteByte('\n')
			_, _ = filtered.Write(line)
		}
	}
	if isSSEHeartbeatValue(eventType) || isSSEHeartbeatValue(strings.TrimSpace(data.String())) {
		return nil
	}
	if !hasData {
		// A blank line dispatches an SSE event. The OpenAI SDK tries to
		// decode even metadata-only events as JSON, so omit those boundaries.
		// Keep an unterminated raw response intact so callers can still report
		// a provider error body instead of silently replacing it.
		if delimited {
			return nil
		}
		return event
	}
	return filtered.Bytes()
}

func isSSEHeartbeatValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ping", "heartbeat", "keep-alive", "keepalive":
		return true
	default:
		return false
	}
}

func normalizeSSEDelimiter(line []byte) []byte {
	if bytes.HasSuffix(line, []byte{'\r', '\n'}) {
		return []byte{'\r', '\n'}
	}
	return []byte{'\n'}
}
