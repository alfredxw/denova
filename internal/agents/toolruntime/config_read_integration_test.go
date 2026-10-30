package toolruntime

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"denova/config"
	"denova/internal/agents/configresource"
	agentrun "denova/internal/agents/run"
	"denova/internal/agents/toolartifact"
	"denova/internal/interactive/teller"

	"github.com/alfredxw/denova/agent"
	agentmiddleware "github.com/alfredxw/denova/agent/engine/middleware"
	agentschema "github.com/alfredxw/denova/agent/schema"
	sdktool "github.com/alfredxw/denova/agent/tool"
	publictools "github.com/alfredxw/denova/agent/tool/builtin"
	publicresult "github.com/alfredxw/denova/agent/tool/result"
)

func TestConfigReadLargeResultSurvivesNativeProcessingAndFileRead(t *testing.T) {
	const limit = 128 << 10
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	_, err := teller.NewLibrary(cfg.NovaDir).Create(teller.Definition{
		ID: "large", Name: "Large narrative style",
		Slots: []teller.PromptSlot{
			{ID: "active", Target: "system", Enabled: true, Content: strings.Repeat("叙述🌌\"\\\n", 12000)},
			{ID: "disabled", Target: "system", Content: "Preserve this disabled rule."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definitions, err := configresource.NewTools(cfg, limit)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := sdktool.StaticTools(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	const arguments = `{"operation":"get","resource":"narrative_style","ids":["large"]}`
	for _, available := range []bool{true, false} {
		t.Run(fmt.Sprintf("artifact_store_%t", available), func(t *testing.T) {
			model := &oneToolThenFinalModel{toolName: "config_read", arguments: arguments}
			definition := agent.Definition{
				Name: "large-config-read", Model: model, Tools: tools,
				Middlewares: []agentmiddleware.Middleware{NewOrchestratorMiddleware(OrchestratorConfig{
					AgentKind: config.AgentKindGeneral, Workspace: cfg.Workspace, ToolResultMaxBytes: limit,
				})},
				ResultProcessor: publicresult.Standard(publicresult.Policy{MaxBytes: limit}),
			}
			store, err := toolartifact.NewStateStore(t.TempDir(), "config-read")
			if err != nil {
				t.Fatal(err)
			}
			if available {
				definition.Artifacts, err = sdktool.IdentifyToolArtifactStorage(store, agentschema.CapabilityIdentity{Kind: "test.config_artifacts", Version: 1})
				if err != nil {
					t.Fatal(err)
				}
			}
			outcome, runErr, results := runPublicToolLifecycle(t, definition, agentrun.NewObserver(nil, ""))
			if !available {
				if runErr == nil || outcome.Status != agentschema.ResultFailed || model.calls.Load() != 1 {
					t.Fatalf("missing artifact storage did not stop the run: status=%s error=%v", outcome.Status, runErr)
				}
				return
			}
			if runErr != nil || outcome.Status != agentschema.ResultCompleted {
				t.Fatalf("config read failed: status=%s error=%v", outcome.Status, runErr)
			}
			result := results["config_read"]
			if result.IsError() || len(result.ModelContent) > limit || !result.Metadata.ModelTruncated || len(result.Artifacts) != 1 || !result.Artifacts[0].Complete {
				t.Fatalf("invalid large-result projection: bytes=%d metadata=%+v artifacts=%+v", len(result.ModelContent), result.Metadata, result.Artifacts)
			}
			path, err := store.ResolveToolArtifactPath(t.Context(), result.Artifacts[0].ReadablePath)
			if err != nil {
				t.Fatal(err)
			}
			complete, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			original, err := definitions[0].Tool.Run(t.Context(), arguments)
			if err != nil || string(complete) != original.ModelContent {
				t.Fatalf("artifact changed the complete config result: %v", err)
			}
			workspace, err := publictools.OpenWorkspaceWithOptions(publictools.WorkspaceOptions{
				Root: cfg.Workspace, Limits: publictools.WorkspaceLimits{MaxResultBytes: 32 << 10},
			})
			if err != nil {
				t.Fatal(err)
			}
			adapter, err := publictools.LocalFileAdapter(workspace)
			if err != nil {
				t.Fatal(err)
			}
			read, err := publictools.Read([]publictools.ReadAdapter{adapter}, publictools.WithMaxResultBytes(32<<10))
			if err != nil {
				t.Fatal(err)
			}
			var recovered strings.Builder
			byteOffset := 0
			for {
				args, _ := json.Marshal(map[string]any{"path": path, "offset": 1, "byte_offset": byteOffset, "limit": 1})
				page, err := read.Tool.Run(t.Context(), string(args))
				if err != nil {
					t.Fatal(err)
				}
				var envelope struct {
					Limits struct {
						Truncated      bool `json:"truncated"`
						NextOffset     int  `json:"next_offset"`
						NextByteOffset int  `json:"next_byte_offset"`
					} `json:"limits"`
				}
				if err := json.Unmarshal(page.Details, &envelope); err != nil {
					t.Fatal(err)
				}
				_, content, found := strings.Cut(page.ModelContent, "\n1\t")
				if !found || !utf8.ValidString(content) || len(page.ModelContent) > 32<<10 {
					t.Fatal("invalid bounded UTF-8 file read")
				}
				recovered.WriteString(content)
				if !envelope.Limits.Truncated {
					break
				}
				if envelope.Limits.NextOffset != 1 || envelope.Limits.NextByteOffset <= byteOffset {
					t.Fatal("long-line continuation did not advance")
				}
				byteOffset = envelope.Limits.NextByteOffset
			}
			if byteOffset == 0 || recovered.String() != string(complete) {
				t.Fatal("file continuation lost or duplicated config bytes")
			}
		})
	}
}
