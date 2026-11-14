package antigravity

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolResult struct {
	done   chan struct{}
	result *mcp.CallToolResult
	err    error
	name   string
	args   json.RawMessage
}

// toolBridge belongs to one process attempt. Antigravity does not publish a
// provider tool-use ID in MCP metadata. Session + JSON-RPC request ID identify a
// delivery here; repeated HTTP deliveries return the already settled outcome.
type toolBridge struct {
	url, token string
	server     *http.Server
	mcp        *mcp.Server
	ready      chan struct{}
	mu         sync.Mutex
	closed     bool
	workers    sync.WaitGroup
	results    map[string]*toolResult
	err        error
}

func startBridge(ctx context.Context, tools []external.Tool, host external.Host, cancel context.CancelFunc) (*toolBridge, error) {
	b := &toolBridge{token: rand.Text(), mcp: mcp.NewServer(&mcp.Implementation{Name: "denova", Version: "1"}, nil), ready: make(chan struct{}), results: map[string]*toolResult{}}
	names := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || names[tool.Name] || !json.Valid(tool.Schema) {
			return nil, errors.New("invalid or duplicate host tool definition")
		}
		names[tool.Name] = true
		b.mcp.AddTool(&mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema}, func(callCtx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
			select {
			case <-b.ready:
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-callCtx.Done():
				return nil, callCtx.Err()
			}
			if req.Extra == nil || req.Extra.Header.Get("X-Denova-Request-ID") == "" {
				return nil, errors.New("Antigravity host tool request lacks identity")
			}
			id := req.Session.ID() + ":" + req.Extra.Header.Get("X-Denova-Request-ID")
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				return nil, errors.New("tool bridge is closed")
			}
			if prior := b.results[id]; prior != nil {
				b.mu.Unlock()
				if prior.name != tool.Name || !bytes.Equal(prior.args, req.Params.Arguments) {
					return nil, errors.New("Antigravity reused a tool request identity with a different payload")
				}
				select {
				case <-prior.done:
					return prior.result, prior.err
				case <-callCtx.Done():
					return nil, callCtx.Err()
				}
			}
			settled := &toolResult{done: make(chan struct{}), name: tool.Name, args: bytes.Clone(req.Params.Arguments)}
			b.results[id] = settled
			b.workers.Add(1)
			b.mu.Unlock()
			defer b.workers.Done()
			defer func() {
				if value := recover(); value != nil {
					slog.ErrorContext(ctx, "Antigravity host tool panicked", "tool", tool.Name, "panic", value)
					err = errors.New("host tool panicked")
				}
				b.mu.Lock()
				settled.result, settled.err = result, err
				close(settled.done)
				b.err = errors.Join(b.err, err)
				b.mu.Unlock()
				if err != nil {
					cancel()
				}
			}()
			callCtx, stop := context.WithCancel(callCtx)
			defer stop()
			unlink := context.AfterFunc(ctx, stop)
			defer unlink()
			value, err := host.CallTool(callCtx, external.ToolCall{ID: id, Name: tool.Name, Arguments: req.Params.Arguments})
			if err != nil {
				return nil, err
			}
			result = &mcp.CallToolResult{IsError: !value.Success, Content: []mcp.Content{&mcp.TextContent{Text: value.Text}}}
			for _, attachment := range value.Images {
				data, err := agentschema.ReadAttachmentImage(attachment)
				if err != nil {
					return nil, err
				}
				result.Content = append(result.Content, &mcp.ImageContent{Data: data, MIMEType: attachment.MediaType})
			}
			return result, nil
		})
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b.url = "http://" + listener.Addr().String() + "/mcp"
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return b.mcp }, nil)
	b.server = &http.Server{ReadHeaderTimeout: infrastructureTimeout, BaseContext: func(net.Listener) context.Context { return ctx }, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mcp" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+b.token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
			if err != nil {
				http.Error(w, "Invalid MCP request", http.StatusBadRequest)
				return
			}
			var envelope struct {
				ID json.RawMessage `json:"id"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				http.Error(w, "Invalid MCP request", http.StatusBadRequest)
				return
			}
			r.Header.Set("X-Denova-Request-ID", string(envelope.ID))
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
		handler.ServeHTTP(w, r)
	})}
	go func() {
		defer func() {
			if value := recover(); value != nil {
				slog.ErrorContext(ctx, "Antigravity tool server panicked", "panic", value)
				cancel()
			}
		}()
		if err := b.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "Antigravity tool server failed", "error", err)
			cancel()
		}
	}()
	return b, nil
}

func (b *toolBridge) close() error {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	_ = b.server.Close()
	for session := range b.mcp.Sessions() {
		_ = session.Close()
	}
	b.workers.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.err
}

func (b *toolBridge) config() map[string]any {
	return map[string]any{"name": "denova", "serverUrl": b.url, "headers": map[string]string{"Authorization": "Bearer " + b.token}}
}
