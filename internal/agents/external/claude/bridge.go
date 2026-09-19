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

	"denova/internal/agents/external"
	agent "github.com/alfredxw/denova/agent"
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
	err        error
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
			b.mu.Lock()
			if b.closed {
				b.mu.Unlock()
				return nil, errors.New("tool bridge is closed")
			}
			b.workers.Add(1)
			b.mu.Unlock()
			defer b.workers.Done()
			defer func() {
				if value := recover(); value != nil {
					slog.ErrorContext(ctx, "[external-runtime] Claude host tool panicked", "tool", tool.Name, "panic", value)
					err = errors.New("host tool panicked")
				}
				if err != nil {
					b.mu.Lock()
					b.err = errors.Join(b.err, err)
					b.mu.Unlock()
					cancel()
				}
			}()
			// Claude supplies the model's tool-use ID in MCP metadata. Unlike
			// transport request IDs it survives an MCP reconnect, so a retried
			// delivery cannot repeat an already committed business effect.
			id, _ := req.Params.Meta["claudecode/toolUseId"].(string)
			if id == "" {
				return nil, errors.New("host tool request lacks identity")
			}
			// Request cancellation and run cancellation both reach the original
			// Host. A result is returned only after Host durably settles the tool.
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
				data, err := agent.ReadAttachmentImage(attachment)
				if err != nil {
					return nil, err
				}
				result.Content = append(result.Content, &mcp.ImageContent{Data: data, MIMEType: attachment.MediaType})
			}
			return result, nil
		})
	}
	if !names["ask"] {
		return nil, errors.New("external runtime requires the ask host tool")
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
