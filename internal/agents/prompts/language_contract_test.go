package prompts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

var latinWordPattern = regexp.MustCompile(`[A-Za-z]{3,}`)

func TestModelFacingSourceRejectsBilingualSplitLiterals(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	paths := []string{
		"agent/permission.go",
		"internal/agents/prompts",
		"internal/agents/chat/context.go",
		"internal/agents/chat/explicit_skills.go",
		"internal/agents/agentprofile",
		"internal/agents/conversation",
		"internal/agents/context",
		"internal/agents/toolruntime",
		"internal/agents/tools",
		"internal/agents/configresource",
		"internal/agents/modeltask",
		"internal/agents/skillassembly",
		"internal/agents/interactive",
		"internal/app/configmanager",
		"internal/app/interactive/conversation.go",
		"internal/app/interactive/conversation_history.go",
		"internal/app/interactive/story_runtime.go",
		"internal/app/interactive/lore_context.go",
		"internal/app/interactive/resident_lore.go",
		"internal/interactive/actor_state_runtime_context.go",
		"internal/interactive/branch_plan.go",
		"internal/interactive/history_search.go",
		"internal/interactive/state_schema_adaptation.go",
		"internal/interactive/state_schema_initialization.go",
		"internal/interactive/state_schema_policy.go",
		"internal/interactive/state_schema_proposal.go",
		"internal/interactive/state_schema_proposal_batch.go",
		"internal/interactive/teller/builtins.go",
		"internal/automation/run_user_message.go",
		"internal/book/novel_import.go",
		"internal/book/lore/index.go",
		"internal/book/lore/lore_catalog.go",
		"internal/book/lore/markdown.go",
		"internal/book/lore/store_read.go",
		"internal/image/asset/lore.go",
		"internal/app/image/agent.go",
		"internal/app/image/interactive.go",
	}
	for _, relative := range paths {
		absolute := filepath.Join(repoRoot, relative)
		matches, err := modelFacingGoFiles(absolute)
		if err != nil {
			t.Fatalf("enumerate %s: %v", relative, err)
		}
		for _, filename := range matches {
			assertNoBilingualSplitStringLiterals(t, filename)
		}
	}
}

func TestSystemPromptTruncationMarkerIsEnglishOnly(t *testing.T) {
	if strings.Contains(systemPromptTruncationMarker, " / ") || containsHan(systemPromptTruncationMarker) {
		t.Fatalf("system prompt truncation marker must be English-only: %q", systemPromptTruncationMarker)
	}
}

func TestBuiltinSkillInstructionsAreEnglishOnly(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	err := filepath.WalkDir(filepath.Join(repoRoot, "skills"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if containsHan(string(content)) {
			t.Errorf("built-in Skill instruction contains Han characters: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func modelFacingGoFiles(path string) ([]string, error) {
	if filepath.Ext(path) == ".go" {
		return []string{path}, nil
	}
	matches, err := filepath.Glob(filepath.Join(path, "*.go"))
	if err != nil {
		return nil, err
	}
	result := matches[:0]
	for _, match := range matches {
		if !strings.HasSuffix(match, "_test.go") {
			result = append(result, match)
		}
	}
	return result, nil
}

func assertNoBilingualSplitStringLiterals(t *testing.T, filename string) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), filename, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", filename, err)
	}
	ast.Inspect(parsed, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil || !strings.Contains(value, " / ") || !containsHan(value) || !latinWordPattern.MatchString(value) {
			return true
		}
		t.Errorf("model-facing string literal repeats English and Chinese with a slash separator in %s: %q", filename, value)
		return true
	})
}

func containsHan(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0
}
