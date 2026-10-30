package external

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/agents/configresource"
	agentrun "denova/internal/agents/run"
	externaljournal "denova/internal/agents/runtime/external/journal"
	"denova/internal/agents/session"
	"denova/internal/interactive/teller"
)

func TestExternalConfigReadArtifactCanBeReadCompletely(t *testing.T) {
	service, request, workspace := operationFixture(t)
	const limit = 128 << 10
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: workspace}
	_, err := teller.NewLibrary(cfg.NovaDir).Create(teller.Definition{
		ID: "large", Name: "Large narrative style",
		Slots: []teller.PromptSlot{{ID: "rule", Target: "system", Enabled: true, Content: strings.Repeat("叙述🌌\"\\\n", 12000)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := configresource.NewTools(cfg, limit)
	if err != nil {
		t.Fatal(err)
	}
	const arguments = `{"operation":"get","resource":"narrative_style","ids":["large"]}`
	original, err := definitions[0].Tool.Run(t.Context(), arguments)
	if err != nil {
		t.Fatal(err)
	}
	request.Definitions = append(request.Definitions, definitions...)
	request.ToolPolicy.ToolSettings[config.AgentToolConfigRead] = true
	request.ToolPolicy.ToolResultMaxBytes = limit
	privatePath := filepath.Join(t.TempDir(), "unrelated.txt")
	if err := os.WriteFile(privatePath, []byte("Unrelated file."), 0o600); err != nil {
		t.Fatal(err)
	}
	var path string
	request.Adapter = adapterFunc(func(ctx context.Context, _ Input, host Host) (Result, error) {
		args, _ := json.Marshal(map[string]any{"path": privatePath})
		denied, err := host.CallTool(ctx, ToolCall{ID: "unrelated", Name: "read", Arguments: args})
		if err != nil || denied.Success {
			return Result{}, fmt.Errorf("artifact access widened unrelated file permissions: %v", err)
		}
		if path == "" {
			result, err := host.CallTool(ctx, ToolCall{ID: "large-config", Name: "config_read", Arguments: json.RawMessage(arguments)})
			if err != nil || !result.Success || len(result.Text) > limit {
				return Result{}, fmt.Errorf("large config result failed: success=%t bytes=%d error=%v", result.Success, len(result.Text), err)
			}
			_, suffix, found := strings.Cut(result.Text, "complete=true; artifact=")
			var complete bool
			path, _, complete = strings.Cut(suffix, "]")
			if !found || !complete || filepath.IsAbs(path) {
				return Result{}, fmt.Errorf("portable complete result artifact was not returned")
			}
		}
		var recovered strings.Builder
		byteOffset := 0
		for {
			args, _ := json.Marshal(map[string]any{"path": path, "offset": 1, "byte_offset": byteOffset, "limit": 1})
			page, err := host.CallTool(ctx, ToolCall{ID: fmt.Sprintf("read-%d", byteOffset), Name: "read", Arguments: args})
			if err != nil || !page.Success || len(page.Text) > limit {
				return Result{}, fmt.Errorf("artifact read failed: success=%t bytes=%d error=%v", page.Success, len(page.Text), err)
			}
			header, body, found := strings.Cut(page.Text, "\n")
			var envelope struct {
				Status string `json:"status"`
				Limits struct {
					Truncated      bool `json:"truncated"`
					NextOffset     int  `json:"next_offset"`
					NextByteOffset int  `json:"next_byte_offset"`
				} `json:"limits"`
			}
			if err := json.Unmarshal([]byte(header), &envelope); err != nil || !found || !strings.HasPrefix(body, "1\t") {
				return Result{}, fmt.Errorf("artifact was not readable: status=%s error=%v", envelope.Status, err)
			}
			recovered.WriteString(strings.TrimPrefix(body, "1\t"))
			if !envelope.Limits.Truncated {
				break
			}
			if envelope.Limits.NextOffset != 1 || envelope.Limits.NextByteOffset <= byteOffset {
				return Result{}, fmt.Errorf("artifact read cursor did not advance")
			}
			byteOffset = envelope.Limits.NextByteOffset
		}
		if byteOffset == 0 || recovered.String() != original.ModelContent {
			return Result{}, fmt.Errorf("artifact reread lost config bytes: got=%d want=%d", recovered.Len(), len(original.ModelContent))
		}
		return Result{Text: "Read the complete configuration."}, nil
	})
	operation, err := service.Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("large config operation failed: status=%s error=%v", outcome.Status, outcome.Error)
	}
	if err := request.Session.ReadExternal(t.Context(), func(state session.ExternalState) error {
		for _, tool := range state.Projection.Operations[operation.id].Tools {
			if tool.Name != "read" {
				continue
			}
			record, err := state.Read(tool.Started)
			if err != nil {
				return err
			}
			var started externaljournal.StartedTool
			if err := json.Unmarshal(record.Data, &started); err != nil {
				return err
			}
			var args struct{ Path string }
			if err := json.Unmarshal(started.Arguments, &args); err != nil {
				return err
			}
			if args.Path != privatePath && args.Path != path {
				return fmt.Errorf("canonical artifact read path was replaced: %s", args.Path)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Reopen the canonical session in another store. The same result reference
	// must resolve there without depending on the original host path or process.
	moved := t.TempDir()
	if err := os.CopyFS(moved, os.DirFS(request.AttachmentRoot)); err != nil {
		t.Fatal(err)
	}
	reopened, err := session.NewStore(moved)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	request.Session, err = reopened.Get(request.Session.ID)
	if err != nil {
		t.Fatal(err)
	}
	request.AttachmentRoot = moved
	request.CommandID, request.Fingerprint, request.Metadata.MessageID = "reread", "reread", "reread-input"
	history, err := PrepareHistory(t.Context(), request.Session)
	if err != nil {
		t.Fatal(err)
	}
	request.PreparedCursor = history.Cursor
	operation, err = (&Service{}).Start(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := operation.Wait(t.Context()); outcome.Status != agentrun.OutcomeCompleted {
		t.Fatalf("moved artifact reread failed: status=%s error=%v", outcome.Status, outcome.Error)
	}
}
