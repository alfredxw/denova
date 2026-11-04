package claude

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"denova/internal/agents/runtime/external"

	agentschema "github.com/alfredxw/denova/agent/schema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolBridge is private to one accepted attempt. Its bearer credential and URL
// exist only in the disposable launch directory, never in a Project journal.
type toolBridge struct {
	url, token string
	server     *http.Server
	mcp        *mcp.Server
	mu         sync.Mutex
	closed     bool
	workers    sync.WaitGroup
	active     int
	steering   chan struct{}
	superseded map[string]bool
	err        error
}

// Hold new callbacks until the accepted guidance is in the product journal.
// A running tool can own a pending question or a mutation and uses the product
// interrupt path so its cancellation and recovery stay paired.
func (b *toolBridge) holdToolsForSteer() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.active != 0 {
		return false
	}
	b.steering = make(chan struct{})
	return true
}

func (b *toolBridge) acceptSteer(previousCalls map[string]bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.superseded == nil {
		b.superseded = make(map[string]bool, len(previousCalls))
	}
	for id := range previousCalls {
		b.superseded[id] = true
	}
	close(b.steering)
	b.steering = nil
}

func (b *toolBridge) admitTool(ctx context.Context, id string) bool {
	for {
		b.mu.Lock()
		if b.closed || ctx.Err() != nil || b.superseded[id] {
			b.mu.Unlock()
			return false
		}
		gate := b.steering
		if gate == nil {
			b.active++
			b.workers.Add(1)
			b.mu.Unlock()
			return true
		}
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-gate:
		}
	}
}

func startBridge(ctx context.Context, tools []external.Tool, host external.Host, cancel context.CancelFunc) (*toolBridge, error) {
	b := &toolBridge{token: rand.Text(), mcp: mcp.NewServer(&mcp.Implementation{Name: "denova", Version: "1"}, nil)}
	names := map[string]bool{}
	for _, tool := range tools {
		if tool.Name == "" || names[tool.Name] || !json.Valid(tool.Schema) {
			return nil, errors.New("invalid or duplicate host tool definition")
		}
		names[tool.Name] = true
		b.mcp.AddTool(&mcp.Tool{Name: tool.Name, Description: tool.Description, InputSchema: tool.Schema}, func(callCtx context.Context, req *mcp.CallToolRequest) (result *mcp.CallToolResult, err error) {
			// Claude supplies the model's tool-use ID in MCP metadata. Unlike
			// transport request IDs it survives an MCP reconnect, so a retried
			// delivery cannot repeat an already committed business effect.
			id, _ := req.Params.Meta["claudecode/toolUseId"].(string)
			// Request cancellation and run cancellation both reach the original
			// Host. A result is returned only after Host durably settles the tool.
			callCtx, stop := context.WithCancel(callCtx)
			defer stop()
			unlink := context.AfterFunc(ctx, stop)
			defer unlink()
			if !b.admitTool(callCtx, id) {
				return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: "Tool call was superseded or cancelled."}}}, nil
			}
			defer func() {
				b.mu.Lock()
				b.active--
				if value := recover(); value != nil {
					slog.ErrorContext(ctx, "[external-runtime] Claude host tool panicked", "tool", tool.Name, "panic", value)
					err = errors.New("host tool panicked")
				}
				if err != nil {
					b.err = errors.Join(b.err, err)
					cancel()
				}
				b.mu.Unlock()
				b.workers.Done()
			}()
			if id == "" {
				return nil, errors.New("host tool request lacks identity")
			}
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
		// Host accepts at most 4 MiB tool arguments; allow envelope overhead.
		r.Body = http.MaxBytesReader(w, r.Body, 5<<20)
		handler.ServeHTTP(w, r)
	})}
	go func() {
		defer func() {
			if v := recover(); v != nil {
				slog.ErrorContext(ctx, "[external-runtime] Claude tool server panicked", "panic", v)
				cancel()
			}
		}()
		if err := b.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.ErrorContext(ctx, "[external-runtime] Claude tool server failed", "error", err)
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

func (b *toolBridge) config() []byte {
	body, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"denova": map[string]any{"type": "http", "url": b.url, "headers": map[string]string{"Authorization": fmt.Sprintf("Bearer %s", b.token)}}}})
	return body
}
