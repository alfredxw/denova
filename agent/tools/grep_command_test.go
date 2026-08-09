package tools

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCompileGrepCommandSupportsNativeSearchSyntax(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{
		"agent", "internal", "docs/Guide With Spaces", "chapters/v00001-卷一-天都篇",
		"chapters/v00002-卷二-灵州篇", "chapters/v00008-卷八-天下篇",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(directory)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	workspace := mustOpenTestWorkspace(t, root)
	plan, err := workspace.compileGrepCommand(`rg -SiC2 -tgo -g '!**/*_test.go' 'OpenWorkspace|GrepRequest' -- internal agent`)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.stages) != 1 {
		t.Fatalf("compiled grep stages = %#v", plan.stages)
	}
	stage := plan.stages[0]
	if stage.mode != grepOutputContent || !plan.groupsContext() || strings.Join(stage.paths, ",") != "agent,internal" {
		t.Fatalf("compiled grep plan = %#v", plan)
	}
	args := grepArguments(stage, false)
	for _, expected := range []string{
		"-SiC2", "-tgo", "--no-config", "--no-follow", "--no-pre", "--no-search-zip", "--sort=path",
		"--path-separator=/", "--glob=!.git/**", "--with-filename", "--line-number",
	} {
		if !containsTestString(args, expected) {
			t.Fatalf("compiled grep args missing %q: %v", expected, args)
		}
	}
	if args[len(args)-2] != "agent" || args[len(args)-1] != "internal" {
		t.Fatalf("compiled grep targets are not normalized: %v", args)
	}
	nativePaths, err := workspace.compileGrepCommand(`rg -n "封侯|男爵|灵州城主" chapters/v00008-卷八-天下篇`)
	if err != nil || len(nativePaths.stages[0].paths) != 1 || nativePaths.stages[0].paths[0] != "chapters/v00008-卷八-天下篇" {
		t.Fatalf("native positional grep paths = %#v, %v", nativePaths, err)
	}

	spaced, err := workspace.compileGrepCommand(`rg -F -e '-draft' -- 'docs/Guide With Spaces'`)
	if err != nil || len(spaced.stages[0].paths) != 1 || spaced.stages[0].paths[0] != "docs/Guide With Spaces" || !spaced.stages[0].hasRegexp {
		t.Fatalf("quoted grep command = %#v, %v", spaced, err)
	}
	files, err := workspace.compileGrepCommand(`rg -l -e TODO -e FIXME -- agent`)
	if err != nil || files.stages[0].mode != grepOutputFiles {
		t.Fatalf("files grep command = %#v, %v", files, err)
	}
	nativeRegexpPaths, err := workspace.compileGrepCommand(`rg -l -e TODO -e FIXME agent internal`)
	if err != nil || nativeRegexpPaths.stages[0].mode != grepOutputFiles || strings.Join(nativeRegexpPaths.stages[0].paths, ",") != "agent,internal" {
		t.Fatalf("native regexp grep paths = %#v, %v", nativeRegexpPaths, err)
	}
	count, err := workspace.compileGrepCommand(`rg --count-matches dragon -- agent`)
	if err != nil || count.stages[0].mode != grepOutputCount {
		t.Fatalf("count grep command = %#v, %v", count, err)
	}
	contextEnabled, err := workspace.compileGrepCommand(`rg -A0 -B0 -C2 dragon`)
	if err != nil || !contextEnabled.groupsContext() {
		t.Fatalf("last -C should enable context grouping: %#v, %v", contextEnabled, err)
	}
	contextDisabled, err := workspace.compileGrepCommand(`rg -C2 -A0 -B0 dragon`)
	if err != nil || contextDisabled.groupsContext() {
		t.Fatalf("last -A/-B should disable context grouping: %#v, %v", contextDisabled, err)
	}
	pipeline, err := workspace.compileGrepCommand(`rg -n -m 8 -e 化蕴 -e 最年轻 -- chapters/v00001-卷一-天都篇 chapters/v00002-卷二-灵州篇 | rg -n "岳小婵|小婵"`)
	if err != nil || len(pipeline.stages) != 2 {
		t.Fatalf("literal rg pipeline = %#v, %v", pipeline, err)
	}
	if strings.Join(pipeline.stages[0].paths, ",") != "chapters/v00001-卷一-天都篇,chapters/v00002-卷二-灵州篇" || strings.Join(pipeline.stages[1].paths, ",") != "-" {
		t.Fatalf("pipeline sources = %#v", pipeline)
	}
	filterArgs := grepArguments(pipeline.stages[1], true)
	if containsTestString(filterArgs, "--with-filename") || !containsTestString(filterArgs, "-n") {
		t.Fatalf("pipeline filter args = %v", filterArgs)
	}
}

func TestCompileGrepCommandRejectsShellAuthorityAndUnsafeRipgrepFlags(t *testing.T) {
	workspace := mustOpenTestWorkspace(t, t.TempDir())
	tests := []struct {
		name    string
		command string
		code    string
	}{
		{name: "wrong executable", command: `grep TODO`, code: "invalid_command"},
		{name: "external pipeline", command: `rg TODO | head -20`, code: "invalid_command"},
		{name: "pipeline path", command: `rg TODO | rg FIXME agent`, code: "pipeline_path"},
		{name: "stderr pipeline", command: `rg TODO |& rg FIXME`, code: "shell_syntax"},
		{name: "second command", command: `rg TODO; echo unsafe`, code: "shell_syntax"},
		{name: "conditional command", command: `rg TODO && echo unsafe`, code: "shell_syntax"},
		{name: "background", command: `rg TODO &`, code: "shell_syntax"},
		{name: "redirect", command: `rg TODO > result.txt`, code: "shell_syntax"},
		{name: "substitution", command: `rg "$(pwd)"`, code: "shell_syntax"},
		{name: "variable", command: `rg "$PATTERN"`, code: "shell_syntax"},
		{name: "assignment", command: `MODE=x rg TODO`, code: "shell_syntax"},
		{name: "preprocessor", command: `rg --pre cat TODO`, code: "unsafe_flag"},
		{name: "compressed programs", command: `rg -z TODO`, code: "unsafe_flag"},
		{name: "hostname program", command: `rg --hostname-bin=sh TODO`, code: "unsafe_flag"},
		{name: "follow links", command: `rg -L TODO`, code: "unsafe_flag"},
		{name: "config", command: `rg --config TODO`, code: "unsafe_flag"},
		{name: "pattern file", command: `rg -f patterns.txt`, code: "unsafe_flag"},
		{name: "raw json", command: `rg --json TODO`, code: "unsafe_flag"},
		{name: "passthru", command: `rg --passthru TODO`, code: "unsafe_flag"},
		{name: "unknown flag", command: `rg --future-flag TODO`, code: "unsupported_flag"},
		{name: "missing native path", command: `rg TODO agent`, code: "path_not_found"},
		{name: "parent path", command: `rg TODO -- ../outside`, code: "unsafe_path"},
		{name: "absolute path", command: `rg TODO -- /tmp`, code: "unsafe_path"},
		{name: "windows path", command: `rg TODO -- 'C:\\outside'`, code: "unsafe_path"},
		{name: "glob target", command: `rg TODO -- '*.go'`, code: "path_glob"},
		{name: "missing pattern", command: `rg -i`, code: "invalid_pattern"},
		{name: "conflicting modes", command: `rg -l -c TODO`, code: "conflicting_flags"},
		{name: "negative context", command: `rg -C -1 TODO`, code: "invalid_flag_value"},
		{name: "color output", command: `rg --color=always TODO`, code: "invalid_flag_value"},
		{name: "result sort", command: `rg --sort=modified TODO`, code: "invalid_flag_value"},
		{name: "too many pipeline stages", command: strings.Repeat(`rg TODO | `, maxGrepPipelineStages) + `rg TODO`, code: "invalid_command"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := workspace.compileGrepCommand(test.command)
			if err == nil || !strings.Contains(err.Error(), test.code) {
				t.Fatalf("compile %q error = %v, want %s", test.command, err, test.code)
			}
		})
	}
}

func TestCompileGrepCommandReturnsPartialMissingPathWarnings(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	workspace := mustOpenTestWorkspace(t, root)
	plan, err := workspace.compileGrepCommand(`rg TODO -- missing existing missing-again`)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.stages[0].paths) != 1 || plan.stages[0].paths[0] != "existing" || len(plan.warnings) != 2 {
		t.Fatalf("partial path plan = %#v", plan)
	}
	if _, err := workspace.compileGrepCommand(`rg TODO -- missing`); err == nil {
		t.Fatal("grep accepted a sole missing path")
	}
}

func TestCompileGrepCommandRejectsEscapingSymlinkPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows tests")
	}
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	workspace := mustOpenTestWorkspace(t, root)
	if _, err := workspace.compileGrepCommand(`rg TODO -- outside`); err == nil {
		t.Fatal("grep accepted a path whose symlink target escapes the workspace")
	}
	if _, err := workspace.compileGrepCommand(`rg TODO -- . outside`); err == nil {
		t.Fatal("grep treated an escaping symlink as a partially missing path")
	}
}

func TestLocalWorkspaceGrepSupportsLiteralRipgrepPipeline(t *testing.T) {
	if _, err := exec.LookPath("rg"); err != nil {
		t.Skip("ripgrep is unavailable")
	}
	root := t.TempDir()
	mustWriteTestFile(t, root, "chapters/v00001-卷一-天都篇/one.md", "岳小婵是最年轻的弟子\n化蕴深厚\n")
	mustWriteTestFile(t, root, "chapters/v00002-卷二-灵州篇/two.md", "小婵尚未化蕴\n最年轻的是别人\n")
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root: root,
		Limits: WorkspaceLimits{
			DefaultDirectoryItems: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := `rg -n -m 8 -e 化蕴 -e 最年轻 -- chapters/v00001-卷一-天都篇 chapters/v00002-卷二-灵州篇 | rg -n "岳小婵|小婵"`
	first, err := workspace.Grep(context.Background(), GrepRequest{Command: command})
	if err != nil || len(first.Entries) != 1 || !first.Truncated || first.NextCursor == "" {
		t.Fatalf("first pipeline page = %#v, %v", first, err)
	}
	second, err := workspace.Grep(context.Background(), GrepRequest{Command: command, Cursor: first.NextCursor})
	if err != nil || len(second.Entries) != 1 {
		t.Fatalf("second pipeline page = %#v, %v", second, err)
	}
	entries := append(append([]string(nil), first.Entries...), second.Entries...)
	if !strings.Contains(entries[0], "岳小婵是最年轻的弟子") || !strings.Contains(entries[1], "小婵尚未化蕴") {
		t.Fatalf("pipeline entries = %#v", entries)
	}
	if strings.Contains(strings.Join(entries, "\n"), "化蕴深厚") || strings.Contains(strings.Join(entries, "\n"), "最年轻的是别人") {
		t.Fatalf("pipeline did not filter first-stage matches: %#v", entries)
	}
}

func TestGrepCursorRejectsChangedPrecedingResults(t *testing.T) {
	root := t.TempDir()
	mustWriteTestFile(t, root, "one.md", "dragon one\n")
	mustWriteTestFile(t, root, "two.md", "dragon two\n")
	workspace, err := OpenWorkspaceWithOptions(WorkspaceOptions{
		Root: root,
		Limits: WorkspaceLimits{
			DefaultDirectoryItems: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := workspace.Grep(context.Background(), GrepRequest{Command: "rg dragon"})
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	mustWriteTestFile(t, root, "one.md", "changed dragon one\n")
	_, err = workspace.Grep(context.Background(), GrepRequest{Command: "rg dragon", Cursor: first.NextCursor})
	if err == nil || !strings.Contains(err.Error(), "cursor is stale") {
		t.Fatalf("changed prefix cursor error = %v", err)
	}
}

func TestNormalizeGrepEntryPreservesMatchedBackslashes(t *testing.T) {
	input := ".\\chapter.md:1:const path = `C:\\fiction\\chapter.md`"
	want := "chapter.md:1:const path = `C:\\fiction\\chapter.md`"
	if got := normalizeGrepEntry(input); got != want {
		t.Fatalf("normalizeGrepEntry() = %q, want %q", got, want)
	}
}
