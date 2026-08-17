package compat

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
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
		body.WriteString("\n")
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
	if got, want := string(data), "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"; got != want {
		t.Fatalf("filtered stream = %q, want %q", got, want)
	}
}

func TestWrapHTTPClientPreservesSSEDispatchBoundaries(t *testing.T) {
	client := WrapHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := strings.Join([]string{
			": ping\n\n",
			`data: {"id":"stream-id","object":"chat.completion.chunk","created":1,"model":"test-model","choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}` + "\n\n",
			"data: [DONE]\n\n",
		}, "")
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader(body)),
			Request:    req,
		}, nil
	})})
	sdkClient := sdk.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL("https://example.invalid/v1"),
		option.WithHTTPClient(client),
	)
	stream := sdkClient.Chat.Completions.NewStreaming(context.Background(), sdk.ChatCompletionNewParams{
		Messages: []sdk.ChatCompletionMessageParamUnion{sdk.UserMessage("hello")},
		Model:    shared.ChatModel("test-model"),
	})
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("stream did not yield preserved event: %v", stream.Err())
	}
	chunk := stream.Current()
	if len(chunk.Choices) != 1 || chunk.Choices[0].Delta.Content != "ok" || chunk.Choices[0].FinishReason != "stop" {
		t.Fatalf("stream chunk = %#v", chunk)
	}
	if stream.Next() || stream.Err() != nil {
		t.Fatalf("stream terminal error = %v, want clean EOF", stream.Err())
	}
}

func TestWrapHTTPClientFiltersWholeHeartbeatAndMetadataEvents(t *testing.T) {
	body := strings.Join([]string{
		"event: ping\r\ndata: 1712345678\r\n\r\n",
		"event: provider-status\r\nid: status-1\r\nretry: 1000\r\n\r\n",
		"data: keep-alive\n\n",
		"event: message\r\nid: response-1\r\ndata: {\"ok\":true}\r\n \r\n",
	}, "")
	client := WrapHTTPClient(&http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"text/event-stream"}, "Content-Length": []string{"999"}},
			Body:          &fragmentedReadCloser{reader: strings.NewReader(body)},
			ContentLength: 999,
			Request:       req,
		}, nil
	})})

	req, err := http.NewRequest(http.MethodPost, "https://example.invalid/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.ContentLength != -1 || resp.Header.Get("Content-Length") != "" {
		t.Fatalf("filtered response length = (%d, %q), want (-1, empty)", resp.ContentLength, resp.Header.Get("Content-Length"))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	want := "event: message\r\ndata: {\"ok\":true}\r\n\r\n"
	if got := string(data); got != want {
		t.Fatalf("filtered stream = %q, want %q", got, want)
	}
}

func TestSSEHeartbeatFilteringBodyPropagatesSourceError(t *testing.T) {
	wantErr := errors.New("provider stream failed")
	source := &failingReadCloser{
		data:   []byte("data: {\"ok\":true}\n\n"),
		err:    wantErr,
		closed: make(chan struct{}),
	}
	body := newSSEHeartbeatFilteringBody(source)
	data, err := io.ReadAll(body)
	if !errors.Is(err, wantErr) {
		t.Fatalf("read error = %v, want %v", err, wantErr)
	}
	if got, want := string(data), "data: {\"ok\":true}\n\n"; got != want {
		t.Fatalf("filtered stream = %q, want %q", got, want)
	}
	if err := body.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	select {
	case <-source.closed:
	default:
		t.Fatal("source was not closed")
	}
}

func TestSSEHeartbeatFilteringBodyCloseUnblocksSource(t *testing.T) {
	sourceReader, sourceWriter := io.Pipe()
	body := newSSEHeartbeatFilteringBody(sourceReader)
	if err := body.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceWriter.Write([]byte("data: late\n\n")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("source write error = %v, want ErrClosedPipe", err)
	}
	_ = sourceWriter.Close()
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

type fragmentedReadCloser struct {
	reader *strings.Reader
}

func (r *fragmentedReadCloser) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.reader.Read(p)
}

func (*fragmentedReadCloser) Close() error { return nil }

type failingReadCloser struct {
	data   []byte
	err    error
	closed chan struct{}
}

func (r *failingReadCloser) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, nil
}

func (r *failingReadCloser) Close() error {
	select {
	case <-r.closed:
	default:
		close(r.closed)
	}
	return nil
}

var _ io.ReadCloser = (*fragmentedReadCloser)(nil)
var _ io.ReadCloser = (*failingReadCloser)(nil)
