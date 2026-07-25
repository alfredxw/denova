package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestMain(m *testing.M) {
	if os.Getenv("DENOVA_TEST_RIPGREP_HELPER") == "1" {
		for _, arg := range os.Args[1:] {
			if arg == "--no-config" {
				fmt.Fprint(os.Stdout, os.Getenv("DENOVA_TEST_RIPGREP_OUTPUT"))
				os.Exit(0)
			}
		}
		fmt.Fprintln(os.Stderr, "missing --no-config")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestLocalWorkspaceStandardTools(t *testing.T) {
	root := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := workspace.Write(ctx, "chapters/one.md", "alpha\nbeta\n"); err != nil {
		t.Fatal(err)
	}
	read, err := workspace.Read(ctx, ReadRequest{Path: "chapters/one.md", Offset: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if read.Content != "beta\n" || read.Path != "chapters/one.md" {
		t.Fatalf("read = %#v", read)
	}
	if _, err := workspace.Edit(ctx, "chapters/one.md", []TextEdit{{OldString: "beta", NewString: "gamma"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "chapters", "one.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "alpha\ngamma\n" {
		t.Fatalf("content = %q", data)
	}
	entries, err := workspace.List(ctx, "chapters")
	if err != nil || len(entries) != 1 || entries[0] != "chapters/one.md" {
		t.Fatalf("list = %v, %v", entries, err)
	}
	matches, err := workspace.Glob(ctx, ".", "**/*.md")
	if err != nil || len(matches) != 1 || matches[0] != "chapters/one.md" {
		t.Fatalf("glob = %v, %v", matches, err)
	}
}

func TestLocalWorkspaceRejectsTraversalAndEscapingSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Write(context.Background(), "../escape.txt", "no"); err == nil {
		t.Fatal("expected parent traversal rejection")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Write(context.Background(), "outside/escape.txt", "no"); err == nil {
		t.Fatal("expected os.Root symlink escape rejection")
	}
}

func TestOpenWorkspaceCanonicalizesAndRejectsInvalidRoots(t *testing.T) {
	root := t.TempDir()
	workspace, err := OpenWorkspace(filepath.Join(root, "."))
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Root() != filepath.Clean(canonical) {
		t.Fatalf("root = %q, want %q", workspace.Root(), canonical)
	}
	if _, err := OpenWorkspace(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing workspace should be rejected")
	}
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, []byte("text"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenWorkspace(file); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("file workspace should be rejected, got %v", err)
	}
}

func TestOpenWorkspaceWithOptionsRejectsNonExecutableRipgrep(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not use Unix executable permission bits")
	}
	ripgrepPath := filepath.Join(t.TempDir(), "rg")
	if err := os.WriteFile(ripgrepPath, []byte("not executable"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root:              t.TempDir(),
		RipgrepExecutable: ripgrepPath,
	})
	if err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("non-executable ripgrep error = %v", err)
	}
}

func TestReadFileDefinitionReturnsBoundedNumberedWindow(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "story.txt"), []byte("first\nsecond\nthird\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := ReadFile(workspace)
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), `{"file_path":"story.txt","offset":2,"limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, `"schema":"workspace_file.read.v2"`) ||
		!strings.Contains(result.ModelContent, `"file_path":"story.txt"`) ||
		!strings.Contains(result.ModelContent, "     2\tsecond") ||
		strings.Contains(result.ModelContent, "first") || strings.Contains(result.ModelContent, "third") {
		t.Fatalf("unexpected read_file result: %q", result.ModelContent)
	}
}

func TestLocalWorkspaceReadRejectsOversizedAndNonUTF8Windows(t *testing.T) {
	root := t.TempDir()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(strings.Repeat("x", defaultResultBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Read(context.Background(), ReadRequest{Path: "long.txt"}); err == nil || !strings.Contains(err.Error(), "selected read_file window exceeds") {
		t.Fatalf("oversized read should fail, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Read(context.Background(), ReadRequest{Path: "binary.txt"}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("non-UTF-8 read should fail, got %v", err)
	}
}

func TestLocalWorkspaceGrepModesAndHeadLimit(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is not installed")
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "chapters"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "chapters", "one.md"), []byte("opening\n-dragon wakes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(strings.Repeat("match\n", 10)), 0o644); err != nil {
		t.Fatal(err)
	}
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	content, err := workspace.Grep(context.Background(), GrepRequest{Pattern: "-dragon", Path: "chapters", OutputMode: "content"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(content, "\n") != "chapters/one.md:2:-dragon wakes" {
		t.Fatalf("content mode = %#v", content)
	}
	files, err := workspace.Grep(context.Background(), GrepRequest{Pattern: "-dragon", OutputMode: "files_with_matches"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(files, "\n") != "chapters/one.md" {
		t.Fatalf("files mode = %#v", files)
	}
	counts, err := workspace.Grep(context.Background(), GrepRequest{Pattern: "-dragon", OutputMode: "count"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(counts, "\n") != "chapters/one.md:1" {
		t.Fatalf("count mode = %#v", counts)
	}
	limited, err := workspace.Grep(context.Background(), GrepRequest{Pattern: "match", OutputMode: "content", HeadLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 3 || limited[2] != resultTruncatedMarker {
		t.Fatalf("head-limited content = %#v", limited)
	}
}

func TestLocalWorkspaceGrepUsesConfiguredRipgrepWithoutPATH(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DENOVA_TEST_RIPGREP_HELPER", "1")
	t.Setenv("DENOVA_TEST_RIPGREP_OUTPUT", "chapters/one.md\n")

	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root:              t.TempDir(),
		RipgrepExecutable: os.Args[0],
	})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := workspace.Grep(context.Background(), GrepRequest{
		Pattern:    "dragon",
		OutputMode: "files_with_matches",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(matches, "\n") != "chapters/one.md" {
		t.Fatalf("matches = %#v", matches)
	}
}

func TestExecuteStreamsProgressThroughAgentAndKeepsWorkspace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell assertion is Unix-specific")
	}
	workspace, err := OpenWorkspace(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewLocalCommandRunner(workspace)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Execute(runner)
	if err != nil {
		t.Fatal(err)
	}
	model := &shellTestModel{}
	native, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name: "shell-test", Model: model, Tools: []agent.ToolDefinition{definition},
	})
	if err != nil {
		t.Fatal(err)
	}
	events := agent.NewRunner(agent.RunnerConfig{Agent: native}).Query(context.Background(), "go")
	var result string
	for {
		event, ok := events.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			t.Fatal(event.Err)
		}
		if event.Output != nil && event.Output.MessageOutput != nil && event.Output.MessageOutput.Role == agent.ToolRole {
			result = event.Output.MessageOutput.Message.Content
		}
	}
	if result != "reusable" {
		t.Fatalf("result = %q", result)
	}
}

func TestExecutePublishesStableSchemaAndFailsClosedWithoutRunner(t *testing.T) {
	definition, err := Execute(nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "execute" || !strings.Contains(string(raw), `"command"`) {
		t.Fatalf("execute schema = %s", raw)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"command":"echo ok","background":true}`); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown execute argument should fail, got %v", err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"command":"echo unsafe"}`); err == nil || !strings.Contains(err.Error(), "capability is disabled") {
		t.Fatalf("disabled execute should fail closed, got %v", err)
	}
}

func TestReadBoundedAndDrainConsumesRemainder(t *testing.T) {
	source := &countingReader{reader: strings.NewReader(strings.Repeat("x", 64))}
	result, err := readBoundedAndDrain(source, 8)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "xxxxxxxx" || source.read != 64 {
		t.Fatalf("result=%q read=%d", result, source.read)
	}
}

type countingReader struct {
	reader *strings.Reader
	read   int
}

type shellTestModel struct{ calls int }

func (model *shellTestModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	model.calls++
	if model.calls == 1 {
		return agent.AssistantMessage("", []agent.ToolCall{{
			ID: "execute-1", Type: "function",
			Function: agent.FunctionCall{Name: "execute", Arguments: `{"command":"printf reusable"}`},
		}}), nil
	}
	return agent.AssistantMessage("done", nil), nil
}

func (model *shellTestModel) Stream(ctx context.Context, input []*agent.Message, options ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	message, err := model.Generate(ctx, input, options...)
	if err != nil {
		return nil, err
	}
	return agent.StreamReaderFromArray([]*agent.Message{message}), nil
}

func (reader *countingReader) Read(buffer []byte) (int, error) {
	count, err := reader.reader.Read(buffer)
	reader.read += count
	return count, err
}
