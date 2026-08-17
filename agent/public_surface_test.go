package agent_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"
)

func TestPublicSurfaceDoesNotExposeLowLevelExecutionEngine(t *testing.T) {
	forbidden := map[string]struct{}{
		"Loop": {}, "LoopConfig": {}, "NewLoop": {},
		"Runner": {}, "RunnerConfig": {}, "NewRunner": {}, "Runnable": {},
		"AgentInput": {}, "AgentEvent": {}, "AgentOutput": {}, "AgentAction": {}, "AgentRunOption": {},
		"AsyncIterator": {}, "AsyncGenerator": {}, "NewPair": {}, "MessageVariant": {}, "RunStep": {},
		"AgentCancelOption": {}, "AgentCancelFunc": {}, "AgentCancelInfo": {}, "CancelHandle": {},
		"CancelMode": {}, "CancelError": {}, "StreamCanceledError": {}, "InterruptError": {},
		"WithCancel": {}, "WithAgentCancelMode": {}, "WithAgentCancelTimeout": {}, "IsInterruptError": {},
		"ErrCancelTimeout": {}, "ErrExecutionEnded": {}, "ErrStreamCanceled": {},
		"Engine": {}, "Harness": {}, "BindingRef": {}, "Runtime": {},
		"Journal": {}, "JournalStore": {}, "EventSink": {}, "Host": {},
	}

	files := token.NewFileSet()
	packages, err := parser.ParseDir(files, ".", func(info os.FileInfo) bool {
		return info.Name() != "public_surface_test.go"
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	parsed, ok := packages["agent"]
	if !ok {
		t.Fatal("agent package was not parsed")
	}
	for _, file := range parsed.Files {
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.FuncDecl:
				if declaration.Recv == nil {
					if _, denied := forbidden[declaration.Name.Name]; denied {
						t.Errorf("low-level execution function %q is public at %s", declaration.Name.Name, files.Position(declaration.Pos()))
					}
				}
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if _, denied := forbidden[specification.Name.Name]; denied {
							t.Errorf("low-level execution type %q is public at %s", specification.Name.Name, files.Position(specification.Pos()))
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if _, denied := forbidden[name.Name]; denied {
								t.Errorf("low-level execution value %q is public at %s", name.Name, files.Position(name.Pos()))
							}
						}
					}
				}
			}
		}
	}
}

func TestLegacyPublicRuntimePackageIsRemoved(t *testing.T) {
	if _, err := os.Stat("runtime"); !os.IsNotExist(err) {
		t.Fatalf("legacy public runtime directory still exists: %v", err)
	}
}
