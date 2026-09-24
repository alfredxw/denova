package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"denova/internal/agents/toolartifact"
	agent "github.com/alfredxw/denova/agent"
	"github.com/alfredxw/denova/agent/providers"
	"github.com/alfredxw/denova/agent/toolresult"
	agenttools "github.com/alfredxw/denova/agent/tools"
)

func TestReadImagesCaptureImmutableCopiesAcrossFormats(t *testing.T) {
	for _, format := range []string{"png", "jpeg", "gif", "webp"} {
		t.Run(format, func(t *testing.T) {
			var data bytes.Buffer
			picture := image.NewNRGBA(image.Rect(0, 0, 31, 47))
			var err error
			switch format {
			case "png":
				err = png.Encode(&data, picture)
			case "jpeg":
				err = jpeg.Encode(&data, picture, nil)
			case "gif":
				err = gif.Encode(&data, picture, nil)
			case "webp":
				decoded, decodeErr := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4TBEAAAAvAAAAAAfQ//73v/+BiOh/AAA=")
				err = decodeErr
				data.Write(decoded)
			}
			if err != nil {
				t.Fatal(err)
			}
			workspaceRoot, stateRoot := t.TempDir(), t.TempDir()
			// Detect bytes even when the source has no extension.
			source := filepath.Join(workspaceRoot, "reference")
			if err := os.WriteFile(source, data.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			definition, ctx := imageReadDefinition(t, workspaceRoot, stateRoot)
			ctx = agent.ContextWithToolCall(ctx, "image-call", "read")
			result, err := definition.Tool.Run(ctx, `{"path":"reference"}`)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Attachments) != 1 || len(result.Artifacts) != 1 || !strings.Contains(result.ModelContent, `"kind":"local_image"`) {
				t.Fatalf("image was not returned natively: %+v", result)
			}
			attachment := result.Attachments[0]
			if !fs.ValidPath(attachment.Path) || filepath.IsAbs(attachment.Path) || attachment.Path == "reference" || attachment.MediaType != "image/"+format {
				t.Fatalf("snapshot identity is not portable: %+v", attachment)
			}
			// Same execution is idempotent; another read captures the then-current bytes.
			retry, err := definition.Tool.Run(ctx, `{"path":"reference"}`)
			if err != nil || !reflect.DeepEqual(retry.Attachments, result.Attachments) {
				t.Fatalf("retry changed image snapshot: %v", err)
			}
			if err := os.Remove(source); err != nil {
				t.Fatal(err)
			}
			original, err := agent.ReadAttachmentImage(attachment)
			if err != nil || !bytes.Equal(original, data.Bytes()) {
				t.Fatalf("source deletion changed captured pixels: %v", err)
			}
			processed, err := toolresult.Standard(toolresult.Policy{MaxBytes: 512}).Process(ctx, agent.ToolResultProcessRequest{
				ToolName: "read", Arguments: `{"path":"reference"}`, ProviderCallID: "image-call", BatchSize: 1,
				Definition: agent.ToolDefinitionSnapshot{Descriptor: definition.Descriptor}, Result: result,
			})
			if err != nil {
				t.Fatal(err)
			}
			message := agent.ToolMessage(processed, "image-call", agent.WithToolName("read"))
			if !reflect.DeepEqual(message.Attachments, result.Attachments) || !reflect.DeepEqual(message.EffectiveToolResult().Attachments, result.Attachments) {
				t.Fatal("tool projection dropped the native image")
			}
			encoded, err := json.Marshal(message)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(encoded, []byte(stateRoot)) || bytes.Contains(encoded, []byte("base64")) {
				t.Fatal("host paths or binary data entered the journal")
			}
			var restored agent.Message
			if err := json.Unmarshal(encoded, &restored); err != nil {
				t.Fatal(err)
			}
			if restored.Attachments[0].RuntimePath != "" || restored.Attachments[0].Path != attachment.Path {
				t.Fatal("image journal round trip changed portable identity")
			}
			estimator := agent.InputEstimator{ImageTokens: func(int, int) int { return 123 }}
			size, err := estimator.Estimate([]*agent.Message{message, message.Clone()}, nil)
			if err != nil || size.Tokens != agent.EstimateRequestTextTokens([]*agent.Message{message, message.Clone()}, nil)+246 || providers.NativeImageCount([]*agent.Message{message, message.Clone()}) != 2 {
				t.Fatalf("repeated tool images were not included in visual accounting: %+v %v", size, err)
			}
			if err := os.WriteFile(attachment.RuntimePath, []byte("changed"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := agent.ReadAttachmentImage(attachment); err == nil {
				t.Fatal("snapshot corruption was silently accepted")
			}
		})
	}
}

func TestReadImageFailuresDoNotPublishSnapshots(t *testing.T) {
	for _, scenario := range []string{"invalid", "oversized", "cancelled", "no_store"} {
		t.Run(scenario, func(t *testing.T) {
			workspaceRoot, stateRoot := t.TempDir(), t.TempDir()
			var data bytes.Buffer
			if err := png.Encode(&data, image.NewNRGBA(image.Rect(0, 0, 1, 1))); err != nil {
				t.Fatal(err)
			}
			if scenario == "invalid" {
				data.Truncate(12)
			}
			source := filepath.Join(workspaceRoot, "reference.png")
			if err := os.WriteFile(source, data.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			if scenario == "oversized" {
				if err := os.Truncate(source, 20<<20+1); err != nil {
					t.Fatal(err)
				}
			}
			definition, ctx := imageReadDefinition(t, workspaceRoot, stateRoot)
			if scenario == "no_store" {
				ctx = t.Context()
			}
			if scenario == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if _, err := definition.Tool.Run(ctx, `{"path":"reference.png"}`); err == nil {
				t.Fatal("invalid image read unexpectedly succeeded")
			}
			entries, err := os.ReadDir(stateRoot)
			if err != nil || len(entries) != 0 {
				t.Fatalf("failed image read published state: %v %v", entries, err)
			}
		})
	}
}

func imageReadDefinition(t *testing.T, workspaceRoot, stateRoot string) (agent.ToolDefinition, context.Context) {
	t.Helper()
	workspace, err := agenttools.OpenWorkspaceWithOptions(agenttools.WorkspaceOptions{Root: workspaceRoot})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := agenttools.LocalFileAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Read([]agenttools.ReadAdapter{adapter}, agenttools.WithMaxResultBytes(256))
	if err != nil {
		t.Fatal(err)
	}
	store, err := toolartifact.NewStateStore(stateRoot, "image-session")
	if err != nil {
		t.Fatal(err)
	}
	return definition, agent.ContextWithToolArtifactBackend(t.Context(), store)
}
