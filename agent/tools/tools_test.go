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
	"sync/atomic"
	"testing"
	"time"

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

func TestReadRoutesLocalTextAndDirectoryWithAdapterSpecificArguments(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, root, "chapters/one.md", "first\nsecond\nthird\n")
	mustWriteTestFile(t, root, "chapters/.hidden", "secret")
	mustWriteTestFile(t, root, "chapters/nested/two.md", "two")
	workspace := mustOpenTestWorkspace(t, root)
	textAdapter, err := LocalTextAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	directoryAdapter, err := DirectoryAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Read([]ReadAdapter{textAdapter, directoryAdapter})
	if err != nil {
		t.Fatal(err)
	}
	info, err := definition.Tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := info.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "read" || len(parameters.AnyOf) != 2 {
		t.Fatalf("read schema = %#v", info)
	}

	fileResult, err := definition.Tool.Run(context.Background(), `{"path":"chapters/one.md","offset":2,"limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fileResult.ModelContent, `"kind":"local_text"`) ||
		!strings.Contains(fileResult.ModelContent, "     2\tsecond") ||
		strings.Contains(fileResult.ModelContent, "first") {
		t.Fatalf("file result = %q", fileResult.ModelContent)
	}

	directoryResult, err := definition.Tool.Run(context.Background(), `{"path":"chapters","depth":2,"limit":10}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(directoryResult.ModelContent, `"kind":"directory"`) ||
		!strings.Contains(directoryResult.ModelContent, "chapters/nested/two.md") ||
		strings.Contains(directoryResult.ModelContent, ".hidden") {
		t.Fatalf("directory result = %q", directoryResult.ModelContent)
	}

	if _, err := definition.Tool.Run(context.Background(), `{"path":"chapters","offset":1}`); err != nil {
		t.Fatalf("directory rejected harmless local_text-only parameter: %v", err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"chapters/one.md","depth":2}`); err != nil {
		t.Fatalf("local text rejected harmless directory-only parameter: %v", err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"https://example.com"}`); err == nil || !strings.Contains(err.Error(), "web_fetch") {
		t.Fatalf("read accepted HTTP resource: %v", err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"chapters/one.md","mystery":true}`); err != nil {
		t.Fatalf("read rejected harmless unknown parameter: %v", err)
	}
}

func TestReadRejectsAmbiguousAdapters(t *testing.T) {
	type input struct {
		Path string `json:"path"`
	}
	makeAdapter := func(name string) ReadAdapter {
		adapter, err := NewReadAdapter(name, func(context.Context, string) (bool, error) { return true, nil }, func(_ context.Context, in input) (ReadResult, error) {
			return ReadResult{Path: in.Path, Kind: name, Content: name}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	}
	definition, err := Read([]ReadAdapter{makeAdapter("one"), makeAdapter("two")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := definition.Tool.Run(context.Background(), `{"path":"resource://one"}`); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous adapters error = %v", err)
	}
}

func TestReadRejectsConflictingSharedAdapterParameters(t *testing.T) {
	type firstInput struct {
		Path  string `json:"path"`
		Limit int    `json:"limit,omitempty" jsonschema:"minimum=1"`
	}
	type secondInput struct {
		Path  string `json:"path"`
		Limit string `json:"limit,omitempty"`
	}
	first, err := NewReadAdapter("first", func(context.Context, string) (bool, error) { return true, nil }, func(context.Context, firstInput) (ReadResult, error) {
		return ReadResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewReadAdapter("second", func(context.Context, string) (bool, error) { return false, nil }, func(context.Context, secondInput) (ReadResult, error) {
		return ReadResult{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Read([]ReadAdapter{first, second}); err == nil || !strings.Contains(err.Error(), "conflicting contracts") {
		t.Fatalf("conflicting adapter parameters were accepted: %v", err)
	}
}

func TestReadUsesWorkspacePolicyAndReturnsContinuation(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, root, "chapter.md", "one\nfive\nnine\n")
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root: root, Limits: WorkspaceLimits{MaxResultBytes: 9, DefaultReadLines: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := LocalTextAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Read([]ReadAdapter{adapter})
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), `{"path":"chapter.md"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.ModelContent, `"status":"partial"`) ||
		!strings.Contains(result.ModelContent, `"next_offset":3`) ||
		!strings.Contains(result.ModelContent, "     2\tfive") ||
		strings.Contains(result.ModelContent, "nine") {
		t.Fatalf("bounded read result = %q", result.ModelContent)
	}
}

func TestReadProjectionBudgetsEnvelopeAndAdvancesOnlyVisibleLines(t *testing.T) {
	result, err := projectReadResult(ReadResult{
		Path: "chapter.md", Kind: "local_text", Offset: 10, Limit: 3,
		Content: strings.Repeat("a", 180) + "\n" + strings.Repeat("b", 180) + "\n" + strings.Repeat("c", 180) + "\n",
	}, 512)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ModelContent) > 512 {
		t.Fatalf("projected read has %d bytes", len(result.ModelContent))
	}
	parts := strings.Split(strings.TrimSuffix(result.ModelContent, "\n"), "\n")
	visible := len(parts) - 1
	if visible <= 0 || visible >= 3 {
		t.Fatalf("visible lines = %d, result=%q", visible, result.ModelContent)
	}
	var envelope readEnvelope
	if err := json.Unmarshal([]byte(parts[0]), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Limits.Returned != visible || envelope.Limits.NextOffset != 10+visible || !envelope.Limits.Truncated {
		t.Fatalf("read envelope = %#v, visible=%d", envelope, visible)
	}
}

func TestReadRejectsTraversalEscapingSymlinkOversizeAndBinary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	mustWriteTestFile(t, root, "long.txt", strings.Repeat("x", defaultResultBytes+1))
	if err := os.WriteFile(filepath.Join(root, "binary.txt"), []byte{0xff, 0xfe}, 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
			t.Fatal(err)
		}
	}
	workspace := mustOpenTestWorkspace(t, root)
	adapter, err := LocalTextAdapter(workspace)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Read([]ReadAdapter{adapter})
	if err != nil {
		t.Fatal(err)
	}
	for _, arguments := range []string{`{"path":"../escape.txt"}`, `{"path":"binary.txt"}`} {
		if _, err := definition.Tool.Run(context.Background(), arguments); err == nil {
			t.Fatalf("unsafe read succeeded: %s", arguments)
		}
	}
	first, err := definition.Tool.Run(context.Background(), `{"path":"long.txt"}`)
	if err != nil {
		t.Fatalf("long-line read should return a byte continuation: %v", err)
	}
	var firstEnvelope readEnvelope
	if err := json.Unmarshal(first.Details, &firstEnvelope); err != nil {
		t.Fatal(err)
	}
	if firstEnvelope.Limits.NextOffset != 1 || firstEnvelope.Limits.NextByteOffset <= 0 || !firstEnvelope.Limits.Truncated {
		t.Fatalf("long-line continuation = %#v", firstEnvelope.Limits)
	}
	continued, err := definition.Tool.Run(context.Background(), fmt.Sprintf(
		`{"path":"long.txt","offset":1,"byte_offset":%d}`, firstEnvelope.Limits.NextByteOffset,
	))
	if err != nil {
		t.Fatalf("continue long line: %v", err)
	}
	var continuedEnvelope readEnvelope
	if err := json.Unmarshal(continued.Details, &continuedEnvelope); err != nil {
		t.Fatal(err)
	}
	if continuedEnvelope.Limits.ByteOffset != firstEnvelope.Limits.NextByteOffset {
		t.Fatalf("continued byte offset = %#v", continuedEnvelope.Limits)
	}
	if runtime.GOOS != "windows" {
		if _, err := definition.Tool.Run(context.Background(), `{"path":"outside/escape.txt"}`); err == nil {
			t.Fatal("escaping symlink read succeeded")
		}
	}
}

type recordingMutationAdapter struct {
	write WriteRequest
	edit  EditRequest
}

func (adapter *recordingMutationAdapter) Write(_ context.Context, request WriteRequest) (agent.ToolResult, error) {
	adapter.write = request
	return agent.TextToolResult(`{"schema":"mutation.test"}`), nil
}

func (adapter *recordingMutationAdapter) Edit(_ context.Context, request EditRequest) (agent.ToolResult, error) {
	adapter.edit = request
	return agent.TextToolResult(`{"schema":"mutation.test"}`), nil
}

func TestWriteAndEditPublishSmallExactInterfaces(t *testing.T) {
	adapter := &recordingMutationAdapter{}
	writeDefinition, err := Write(adapter)
	if err != nil {
		t.Fatal(err)
	}
	editDefinition, err := Edit(adapter)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		definition agent.ToolDefinition
		name       string
		arguments  string
	}{
		{writeDefinition, "write", `{"path":"ideas.md","content":"new"}`},
		{editDefinition, "edit", `{"path":"ideas.md","old_string":"old","new_string":"new","replace_all":true}`},
	} {
		info, infoErr := test.definition.Tool.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name != test.name {
			t.Fatalf("tool name = %q, want %q", info.Name, test.name)
		}
		if _, runErr := test.definition.Tool.Run(context.Background(), test.arguments); runErr != nil {
			t.Fatal(runErr)
		}
	}
	if adapter.write.Path != "ideas.md" || adapter.write.Content != "new" {
		t.Fatalf("write request = %#v", adapter.write)
	}
	if adapter.edit.Path != "ideas.md" || adapter.edit.OldString != "old" || adapter.edit.NewString != "new" || !adapter.edit.ReplaceAll {
		t.Fatalf("edit request = %#v", adapter.edit)
	}
	if _, err := editDefinition.Tool.Run(context.Background(), `{"path":"ideas.md","old_string":"same","new_string":"same"}`); err == nil {
		t.Fatal("edit accepted a no-op")
	}
}

func TestLocalWorkspaceGlobAndGrep(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is unavailable")
	}
	root := t.TempDir()
	mustWriteTestFile(t, root, "chapters/one.md", "opening\n-dragon wakes\n")
	mustWriteTestFile(t, root, "chapters/two.md", "second dragon\n")
	mustWriteTestFile(t, root, "chapters/ignored.md", "ignored dragon\n")
	mustWriteTestFile(t, root, ".hidden.md", "hidden dragon\n")
	if err := os.MkdirAll(filepath.Join(root, "empty-visible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "empty-ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteTestFile(t, root, ".gitignore", "chapters/ignored.md\nempty-ignored/\n")
	workspace := mustOpenTestWorkspace(t, root)
	matches, err := workspace.Glob(context.Background(), GlobRequest{
		Paths: []string{"**/*.md", "empty-*"}, Hidden: true, Gitignore: true, Limit: 20,
	})
	if err != nil || strings.Contains(strings.Join(matches.Entries, "\n"), "ignored.md") ||
		strings.Contains(strings.Join(matches.Entries, "\n"), "empty-ignored") ||
		!containsTestString(matches.Entries, ".hidden.md") || !containsTestString(matches.Entries, "chapters/one.md") ||
		!containsTestString(matches.Entries, "empty-visible/") {
		t.Fatalf("glob = %#v, %v", matches, err)
	}
	ignored, err := workspace.Glob(context.Background(), GlobRequest{
		Paths: []string{"**/*.md"}, Hidden: true, Gitignore: false, Limit: 20,
	})
	if err != nil || !containsTestString(ignored.Entries, "chapters/ignored.md") {
		t.Fatalf("glob without gitignore = %#v, %v", ignored, err)
	}
	firstGlobPage, err := workspace.Glob(context.Background(), GlobRequest{
		Paths: []string{"**/*.md", "empty-*"}, Hidden: true, Gitignore: true, Limit: 1,
	})
	if err != nil || len(firstGlobPage.Entries) != 1 || !firstGlobPage.Truncated || firstGlobPage.NextCursor == "" {
		t.Fatalf("first glob page = %#v, %v", firstGlobPage, err)
	}
	secondGlobPage, err := workspace.Glob(context.Background(), GlobRequest{
		Paths: []string{"**/*.md", "empty-*"}, Hidden: true, Gitignore: true, Limit: 1, Cursor: firstGlobPage.NextCursor,
	})
	if err != nil || len(secondGlobPage.Entries) != 1 || secondGlobPage.Entries[0] == firstGlobPage.Entries[0] {
		t.Fatalf("second glob page = %#v, %v", secondGlobPage, err)
	}
	manyPaths := make([]string, 300)
	for index := range manyPaths {
		manyPaths[index] = "chapters/one.md"
	}
	if result, err := workspace.Glob(context.Background(), GlobRequest{Paths: manyPaths, Hidden: true, Gitignore: true}); err != nil || len(result.Entries) != 1 {
		t.Fatalf("glob should accept more than 256 requested paths: result=%#v err=%v", result, err)
	}
	content, err := workspace.Grep(context.Background(), GrepRequest{
		Pattern: "dragon", Paths: []string{"chapters/*.md"}, Mode: "content",
		CaseSensitive: true, Gitignore: true, Limit: 1,
	})
	if err != nil || len(content.Entries) != 1 || !content.Truncated || content.NextCursor == "" {
		t.Fatalf("first grep page = %#v, %v", content, err)
	}
	next, err := workspace.Grep(context.Background(), GrepRequest{
		Pattern: "dragon", Paths: []string{"chapters/*.md"}, Mode: "content",
		CaseSensitive: true, Gitignore: true, Limit: 1, Cursor: content.NextCursor,
	})
	if err != nil || len(next.Entries) != 1 || next.Entries[0] == content.Entries[0] {
		t.Fatalf("second grep page = %#v, %v", next, err)
	}
	if _, err := workspace.Grep(context.Background(), GrepRequest{
		Pattern: "different", Paths: []string{"chapters/*.md"}, Mode: "content",
		CaseSensitive: true, Gitignore: true, Limit: 1, Cursor: content.NextCursor,
	}); err == nil || !strings.Contains(err.Error(), "does not belong") {
		t.Fatalf("cursor was accepted for a different query: %v", err)
	}
}

func TestLocalWorkspaceGrepUsesConfiguredRipgrep(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DENOVA_TEST_RIPGREP_HELPER", "1")
	t.Setenv("DENOVA_TEST_RIPGREP_OUTPUT", "chapters/one.md\n")
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{Root: t.TempDir(), RipgrepExecutable: os.Args[0]})
	if err != nil {
		t.Fatal(err)
	}
	matches, err := workspace.Grep(context.Background(), GrepRequest{
		Pattern: "dragon", Paths: []string{"."}, Mode: "content",
		CaseSensitive: true, Gitignore: true, Limit: 10,
	})
	if err != nil || strings.Join(matches.Entries, "\n") != "chapters/one.md" {
		t.Fatalf("grep = %#v, %v", matches, err)
	}
}

func TestGrepCursorSupportsLargeContinuationOffsets(t *testing.T) {
	request := GrepRequest{
		Pattern: "dragon", Paths: []string{"."}, Mode: "content",
		CaseSensitive: true, Gitignore: true, Limit: 10,
	}
	cursor, err := encodeGrepCursor(50_000, request)
	if err != nil {
		t.Fatal(err)
	}
	offset, err := decodeGrepCursor(cursor, request)
	if err != nil || offset != 50_000 {
		t.Fatalf("large continuation cursor offset=%d err=%v", offset, err)
	}
}

func TestGrepUsesDeterministicPathOrdering(t *testing.T) {
	args := grepArguments(normalizeGrepRequest(GrepRequest{Pattern: "dragon"}), grepTarget{searchPath: "."})
	if !containsTestString(args, "--sort=path") {
		t.Fatalf("grep args are not deterministically sorted: %v", args)
	}
}

func TestSearchProjectionBudgetsEnvelopeAndRewritesCursor(t *testing.T) {
	request := normalizeGrepRequest(GrepRequest{Pattern: "dragon", Mode: "content", Limit: 3})
	result, err := searchToolResult("grep", SearchResult{Entries: []string{
		strings.Repeat("a", 180), strings.Repeat("b", 180), strings.Repeat("c", 180),
	}}, 600, func(returned int) (string, error) {
		return encodeGrepCursor(7+returned, request)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ModelContent) > 600 {
		t.Fatalf("projected search has %d bytes", len(result.ModelContent))
	}
	parts := strings.Split(result.ModelContent, "\n")
	visible := len(parts) - 1
	if visible <= 0 || visible >= 3 {
		t.Fatalf("visible entries = %d, result=%q", visible, result.ModelContent)
	}
	var envelope searchEnvelope
	if err := json.Unmarshal([]byte(parts[0]), &envelope); err != nil {
		t.Fatal(err)
	}
	offset, err := decodeGrepCursor(envelope.Limits.NextCursor, request)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Limits.Returned != visible || offset != 7+visible || !envelope.Limits.Truncated {
		t.Fatalf("search envelope = %#v offset=%d visible=%d", envelope, offset, visible)
	}
}

type recordingSearcher struct {
	glob GlobRequest
	grep GrepRequest
}

func (searcher *recordingSearcher) Glob(_ context.Context, request GlobRequest) (SearchResult, error) {
	searcher.glob = request
	return SearchResult{Entries: []string{"chapter.md"}}, nil
}

func (searcher *recordingSearcher) Grep(_ context.Context, request GrepRequest) (SearchResult, error) {
	searcher.grep = request
	return SearchResult{Entries: []string{"chapter.md:1:dragon"}}, nil
}

func TestSearchToolsPublishNewStrictInterfaces(t *testing.T) {
	searcher := &recordingSearcher{}
	glob, err := Glob(searcher)
	if err != nil {
		t.Fatal(err)
	}
	grep, err := Grep(searcher)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := glob.Tool.Run(context.Background(), `{"paths":["chapters/**/*.md"],"hidden":false,"gitignore":false,"limit":5}`); err != nil {
		t.Fatal(err)
	}
	if searcher.glob.Hidden || searcher.glob.Gitignore || searcher.glob.Limit != 5 {
		t.Fatalf("glob request = %#v", searcher.glob)
	}
	if _, err := grep.Tool.Run(context.Background(), `{"pattern":"dragon","paths":["chapters"],"mode":"files","case_sensitive":false,"gitignore":false,"context_after":2}`); err != nil {
		t.Fatal(err)
	}
	if searcher.grep.Mode != "files" || searcher.grep.CaseSensitive || searcher.grep.Gitignore || searcher.grep.ContextAfter != 2 {
		t.Fatalf("grep request = %#v", searcher.grep)
	}
	if _, err := glob.Tool.Run(context.Background(), `{"pattern":"legacy"}`); err == nil {
		t.Fatal("glob accepted the removed pattern/path interface")
	}
	if _, err := grep.Tool.Run(context.Background(), `{"pattern":"dragon","output_mode":"content"}`); err != nil {
		t.Fatalf("grep rejected harmless removed output_mode: %v", err)
	}
	if searcher.grep.Mode != "content" {
		t.Fatalf("grep default mode = %q", searcher.grep.Mode)
	}
}

func TestBashReturnsOutputExitMetadataAndUsesGuard(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash process assertion is Unix-specific")
	}
	workspace := mustOpenTestWorkspace(t, t.TempDir())
	var guarded atomic.Int32
	runner, err := NewLocalCommandRunner(CommandRunnerOptions{
		Workspace: workspace, Shell: ShellBash,
		Guard: func(ctx context.Context, run func() error) error {
			guarded.Add(1)
			return run()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := Bash(runner)
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), `{"command":"printf reusable"}`)
	if err != nil {
		t.Fatal(err)
	}
	if guarded.Load() != 1 || result.Status != agent.ToolResultSuccess ||
		!strings.Contains(result.ModelContent, "reusable") || !strings.Contains(result.ModelContent, `"exit_code":0`) {
		t.Fatalf("bash result = %#v guarded=%d", result, guarded.Load())
	}
	failed, err := definition.Tool.Run(context.Background(), `{"command":"printf diagnostic >&2; exit 7"}`)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != agent.ToolResultSuccess || !strings.Contains(failed.ModelContent, `"status":"failed"`) ||
		!strings.Contains(failed.ModelContent, `"exit_code":7`) || !strings.Contains(failed.ModelContent, "diagnostic") {
		t.Fatalf("failed bash result = %#v", failed)
	}
}

func TestBashStoresCompleteOutputArtifactWhileKeepingBoundedModelProjection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash process assertion is Unix-specific")
	}
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root: t.TempDir(), Limits: WorkspaceLimits{MaxResultBytes: 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewLocalCommandRunner(CommandRunnerOptions{Workspace: workspace, Shell: ShellBash})
	if err != nil {
		t.Fatal(err)
	}
	artifactStore := &memoryToolArtifactStore{}
	ctx := agent.ContextWithToolArtifactStore(context.Background(), artifactStore)
	result, err := runner.Run(ctx, CommandRequest{
		Command: `for ((i=0;i<200;i++)); do printf 'line-%03d-abcdefghijklmnopqrstuvwxyz\n' "$i"; done`,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OutputTruncated || result.Artifact == nil || result.ArtifactError != "" ||
		result.OutputBytes != int64(len(artifactStore.content.String())) || len(artifactStore.content.String()) <= len(result.Output) {
		t.Fatalf("command artifact result = %#v artifact_bytes=%d", result, len(artifactStore.content.String()))
	}
	if !strings.Contains(artifactStore.content.String(), "line-199-abcdefghijklmnopqrstuvwxyz") {
		t.Fatalf("complete artifact lost output tail: %q", artifactStore.content.String())
	}
	projected, err := commandToolResult(result, 1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(projected.ModelContent) > 1024 || len(projected.Artifacts) != 1 ||
		!strings.Contains(projected.ModelContent, `"artifact"`) || !strings.Contains(projected.ModelContent, processTruncatedMarker) {
		t.Fatalf("bounded process projection = %#v", projected)
	}
}

func TestProcessProjectionKeepsMandatoryMetadataWithinBudget(t *testing.T) {
	result, err := commandToolResult(CommandResult{
		Status: ProcessStatusFailed, Shell: ShellBash, Engine: "bash", ExitCode: 7,
		Output: strings.Repeat("diagnostic", 200), Cwd: ".",
	}, 512)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ModelContent) > 512 {
		t.Fatalf("projected process has %d bytes", len(result.ModelContent))
	}
	first, _, found := strings.Cut(result.ModelContent, "\n")
	if !found {
		t.Fatalf("missing projected output: %q", result.ModelContent)
	}
	var envelope processEnvelope
	if err := json.Unmarshal([]byte(first), &envelope); err != nil {
		t.Fatalf("metadata must be the complete first line: %v", err)
	}
	if envelope.ExitCode != 7 || envelope.Status != ProcessStatusFailed || !envelope.OutputTruncated {
		t.Fatalf("process envelope = %#v", envelope)
	}
}

func TestShellDescriptorDeclaresHostExternalEffects(t *testing.T) {
	definition, err := Bash(fakeCommandRunner{result: CommandResult{Shell: ShellBash, Engine: "bash"}})
	if err != nil {
		t.Fatal(err)
	}
	if definition.Descriptor.MutationScope != agent.ToolMutationExternal ||
		definition.Descriptor.PostCheck != agent.ToolPostCheckExternalReceipt ||
		definition.Descriptor.Recovery != agent.ToolRecoveryNonIdempotent {
		t.Fatalf("shell descriptor = %#v", definition.Descriptor)
	}
}

func TestBashSupportsCwdEnvTimeoutMergedOrderAndPTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash assertion is Unix-specific")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := mustOpenTestWorkspace(t, root)
	runner, err := NewLocalCommandRunner(CommandRunnerOptions{Workspace: workspace, Shell: ShellBash})
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), CommandRequest{
		Command: `printf '%s:%s:' "$NOVA_TEST_VALUE" "$PWD"; printf out; printf err >&2; printf tail`,
		Cwd:     "sub", Env: map[string]string{"NOVA_TEST_VALUE": "env-ok"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ProcessStatusSuccess || result.Cwd != "sub" ||
		!strings.Contains(result.Output, "env-ok:"+filepath.Join(workspace.Root(), "sub")) ||
		!strings.Contains(result.Output, "outerrtail") {
		t.Fatalf("rich bash result = %#v", result)
	}
	ptyResult, err := runner.Run(context.Background(), CommandRequest{Command: `test -t 1 && printf pty-ok`, PTY: true}, nil)
	if err != nil || ptyResult.Status != ProcessStatusSuccess || !strings.Contains(ptyResult.Output, "pty-ok") {
		t.Fatalf("PTY result = %#v err=%v", ptyResult, err)
	}
	started := time.Now()
	timedOut, err := runner.Run(context.Background(), CommandRequest{Command: "sleep 5", TimeoutSeconds: 1}, nil)
	if err != nil || timedOut.Status != ProcessStatusTimedOut || time.Since(started) > 3*time.Second {
		t.Fatalf("timeout result = %#v err=%v elapsed=%s", timedOut, err, time.Since(started))
	}
}

type fakeCommandRunner struct{ result CommandResult }

func (runner fakeCommandRunner) Run(context.Context, CommandRequest, func(string)) (CommandResult, error) {
	return runner.result, nil
}

type memoryToolArtifactStore struct{ content strings.Builder }

func (store *memoryToolArtifactStore) BeginToolArtifact(context.Context, agent.ToolArtifactRequest) (agent.ToolArtifactWriter, error) {
	store.content.Reset()
	return &memoryToolArtifactWriter{store: store}, nil
}

type memoryToolArtifactWriter struct {
	store    *memoryToolArtifactStore
	terminal bool
}

func (writer *memoryToolArtifactWriter) Write(data []byte) (int, error) {
	if writer.terminal {
		return 0, fmt.Errorf("artifact writer is closed")
	}
	return writer.store.content.Write(data)
}

func (writer *memoryToolArtifactWriter) Commit() (agent.ToolArtifactRef, error) {
	writer.terminal = true
	return agent.ToolArtifactRef{
		ID: "memory-artifact", URI: "memory://artifact", MIMEType: "text/plain; charset=utf-8",
		ByteSize: int64(writer.store.content.Len()), SHA256: strings.Repeat("0", 64),
	}, nil
}

func (writer *memoryToolArtifactWriter) Abort() error {
	writer.terminal = true
	return nil
}

func TestPlatformShellToolsHaveDistinctNames(t *testing.T) {
	bash, err := Bash(fakeCommandRunner{result: CommandResult{Shell: ShellBash, Engine: "bash"}})
	if err != nil {
		t.Fatal(err)
	}
	pwsh, err := Pwsh(fakeCommandRunner{result: CommandResult{Shell: ShellPwsh, Engine: "pwsh"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		definition agent.ToolDefinition
		name       string
	}{{bash, "bash"}, {pwsh, "pwsh"}} {
		info, infoErr := test.definition.Tool.Info(context.Background())
		if infoErr != nil || info.Name != test.name {
			t.Fatalf("shell info = %#v err=%v", info, infoErr)
		}
	}
	_, args := shellCommand(ShellPwsh, "powershell.exe", "Get-ChildItem")
	if !containsTestString(args, "-ExecutionPolicy") {
		t.Fatalf("Windows PowerShell fallback args = %v", args)
	}
}

func TestOpenWorkspaceCanonicalizesAndRejectsInvalidRoots(t *testing.T) {
	root := t.TempDir()
	workspace := mustOpenTestWorkspace(t, filepath.Join(root, "."))
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
	if _, err := OpenWorkspace(file); err == nil {
		t.Fatal("file workspace should be rejected")
	}
}

func TestWorkspaceFailsClosedWhenRootPathIsReplaced(t *testing.T) {
	container := t.TempDir()
	root := filepath.Join(container, "workspace")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := mustOpenTestWorkspace(t, root)
	if err := os.Rename(root, filepath.Join(container, "moved")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	opened, err := workspace.openRoot()
	if opened != nil {
		opened.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "identity changed") {
		t.Fatalf("replaced workspace root was accepted: %v", err)
	}
}

func TestWorkspaceDirectoryAndIgnoreScansEnforceAggregateBudgets(t *testing.T) {
	rootPath := t.TempDir()
	mustWriteTestFile(t, rootPath, "one.txt", "one")
	mustWriteTestFile(t, rootPath, ".gitignore", "*.tmp\n")
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if _, err := readWorkspaceDirectory(context.Background(), root, ".", &workspaceScanBudget{entries: maxWorkspaceScanEntries}); err == nil {
		t.Fatal("directory scan exceeded its aggregate entry budget")
	}
	if _, err := readWorkspaceIgnorePatterns(context.Background(), root, ".", nil, &workspaceIgnoreBudget{bytes: maxWorkspaceIgnoreBytes}); err == nil {
		t.Fatal("ignore scan exceeded its aggregate byte budget")
	}
}

func mustOpenTestWorkspace(t *testing.T, root string) *LocalWorkspace {
	t.Helper()
	workspace, err := OpenWorkspace(root)
	if err != nil {
		t.Fatal(err)
	}
	return workspace
}

func mustWriteTestFile(t *testing.T, root, relative, content string) {
	t.Helper()
	fullPath := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsTestString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
