package agentruntime

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"denova/config"
	"denova/internal/agents/conversationconfig"
	"denova/internal/agents/external"
)

type testConnection struct {
	probes, models, closes int
	state                  external.ConnectionState
}

func (*testConnection) Version() string { return "test-engine-v1" }

func (connection *testConnection) Run(context.Context, external.Input, external.Host) (external.Result, error) {
	return external.Result{}, nil
}
func (connection *testConnection) Status() external.ConnectionState { return connection.state }
func (connection *testConnection) Check(context.Context) (external.ConnectionState, error) {
	connection.probes++
	return connection.state, nil
}
func (connection *testConnection) Models(context.Context) (external.Models, error) {
	connection.models++
	return external.Models{Items: []external.Model{{ID: "runtime-model", Efforts: []string{"medium", "high"}}}}, nil
}
func (connection *testConnection) Close() error {
	connection.closes++
	connection.state.Status = "unavailable"
	return nil
}

func TestEngineCatalogIsLazyAndConnectionLeaseProtectsAcceptedOperation(t *testing.T) {
	ctx := context.Background()
	engines := NewEngines()
	defer engines.Close()
	connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
	starts := 0
	engines.entries[config.RuntimeCodex].factory = func(context.Context) (external.Connection, error) { starts++; return connection, nil }
	catalog := engines.Catalog()
	if starts != 0 || catalog[1].Status != "unchecked" || !catalog[1].Capabilities.AskUser || catalog[1].Capabilities.InteractiveApproval {
		t.Fatalf("catalog probed or conflated capabilities: %#v", catalog)
	}
	if _, err := engines.Check(ctx, config.RuntimeNative); err != nil || starts != 0 {
		t.Fatalf("Native invoked an external engine: %v", err)
	}
	selection := config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "runtime-model", Effort: "high"}}
	adapter, release, err := engines.Acquire(ctx, selection, config.Config{})
	if err != nil || adapter != connection || starts != 1 {
		t.Fatalf("acquire failed: %v", err)
	}
	if _, err := engines.Check(ctx, config.RuntimeCodex); err != nil || starts != 1 || connection.closes != 0 {
		t.Fatalf("check replaced an accepted operation's connection: starts=%d closes=%d err=%v", starts, connection.closes, err)
	}
	connection.state.Status = "unavailable"
	if _, err := engines.Check(ctx, config.RuntimeCodex); err != nil || starts != 1 || connection.closes != 0 {
		t.Fatalf("check replaced an unavailable but leased connection: %v", err)
	}
	release()
	release()
	previous := connection
	connection = &testConnection{state: external.ConnectionState{Status: "auth_required"}}
	checked, err := engines.Check(ctx, config.RuntimeCodex)
	if err != nil || checked.Status != "auth_required" || starts != 2 || previous.closes != 1 {
		t.Fatalf("idle check did not reload local credentials: starts=%d closes=%d state=%#v err=%v", starts, previous.closes, checked, err)
	}
	if _, _, err := engines.Acquire(ctx, selection, config.Config{}); !errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("signed-out runtime executed: %v", err)
	}
	previous = connection
	connection = &testConnection{state: external.ConnectionState{Status: "ready"}}
	checked, err = engines.Check(ctx, config.RuntimeCodex)
	if err != nil || checked.Status != "ready" || starts != 3 || previous.closes != 1 {
		t.Fatalf("CLI login was not reloaded: starts=%d closes=%d state=%#v err=%v", starts, previous.closes, checked, err)
	}
	connection.state.Status = "ready"
	selection.Codex.Effort = "unsupported"
	if _, _, err := engines.Acquire(ctx, selection, config.Config{}); !errors.Is(err, ErrEngineModelUnavailable) {
		t.Fatalf("unavailable effort accepted: %v", err)
	}
	if _, err := engines.Models(ctx, config.RuntimeNative); !errors.Is(err, conversationconfig.ErrRuntimeCapabilityUnsupported) {
		t.Fatalf("Native profiles were exposed as engine models: %v", err)
	}
}

func TestEngineConfigurationSectionsRetainOwnerAcrossSelection(t *testing.T) {
	native, codex := EngineConfigurationSections(config.RuntimeNative), EngineConfigurationSections(config.RuntimeCodex)
	if len(native) != len(codex) {
		t.Fatal("selection deleted inactive configuration")
	}
	for index, section := range native {
		other := codex[index]
		if section.ID != other.ID || section.Owner != other.Owner {
			t.Fatal("selection changed field ownership")
		}
		if section.Owner == "shared" && !reflect.DeepEqual(section, other) {
			t.Fatal("shared settings changed with the runtime")
		}
		if section.Owner == "native" && other.State != "inactive" {
			t.Fatal("Native settings remained active")
		}
		if section.ID == "codex.execution_policy" && other.State != "editable" {
			t.Fatal("Codex execution permissions are not editable")
		}
	}
}
