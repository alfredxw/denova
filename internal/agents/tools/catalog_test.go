package tools

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestMain(m *testing.M) {
	if os.Getenv("DENOVA_TEST_CATALOG_RIPGREP_HELPER") == "1" {
		for _, arg := range os.Args[1:] {
			if arg == "--no-config" {
				fmt.Fprint(os.Stdout, "chapters/one.md\n")
				os.Exit(0)
			}
		}
		fmt.Fprintln(os.Stderr, "missing --no-config")
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func TestCatalogFilesystemUsesHostRipgrepExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DENOVA_TEST_CATALOG_RIPGREP_HELPER", "1")
	catalog := NewCatalog(
		&config.Config{Workspace: t.TempDir()},
		nil,
		RuntimeExecutables{Ripgrep: os.Args[0]},
	)
	filesystemTools, err := catalog.Filesystem(config.ResolvedAgentToolSettings{FileRead: true})
	if err != nil {
		t.Fatal(err)
	}
	var grep agent.InvokableTool
	for _, candidate := range filesystemTools {
		info, infoErr := candidate.Info(context.Background())
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if info.Name == "grep" {
			grep, _ = candidate.(agent.InvokableTool)
			break
		}
	}
	if grep == nil {
		t.Fatal("catalog did not expose invokable grep")
	}
	result, err := grep.InvokableRun(context.Background(), `{"pattern":"dragon"}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result) != "chapters/one.md" {
		t.Fatalf("grep result = %q", result)
	}
}
