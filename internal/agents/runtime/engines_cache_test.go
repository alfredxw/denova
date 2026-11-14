package agentruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"denova/config"
	"denova/internal/agents/runtime/external"
)

// catalogProbeConnection reports readiness and the catalog from one probe, like
// the CLI adapters whose readiness check is the catalog command.
type catalogProbeConnection struct {
	testConnection
	probes int
}

func (connection *catalogProbeConnection) Probe(context.Context) (external.ConnectionState, external.Models, error) {
	connection.probes++
	connection.state.Status = "ready"
	return external.ConnectionState{Status: "ready"}, external.Models{
		Items: []external.Model{{ID: "probe-model", Efforts: []string{"high"}}},
	}, nil
}

func TestCatalogProbeAnswersReadinessAndCatalogOnce(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	connection := &catalogProbeConnection{testConnection: testConnection{state: external.ConnectionState{Status: "unchecked"}}}
	entry := engines.entries[config.RuntimeGrok]
	entry.factory = func(context.Context) (external.Connection, error) { return connection, nil }
	models, err := engines.Models(t.Context(), config.RuntimeGrok)
	if err != nil || len(models.Items) != 1 || connection.probes != 1 {
		t.Fatalf("probe did not answer both questions: probes=%d models=%#v err=%v", connection.probes, models, err)
	}
	if connection.models != 0 {
		t.Fatalf("catalog was read twice: %d", connection.models)
	}
	// A ready engine is served from cache: neither readiness nor the catalog is
	// asked for again, so a probe's answer must also be recorded.
	for range 2 {
		cached, err := engines.Models(t.Context(), config.RuntimeGrok)
		if err != nil || len(cached.Items) != 1 || cached.Items[0].ID != "probe-model" {
			t.Fatalf("cached catalog lost the probe result: %#v err=%v", cached, err)
		}
	}
	if connection.probes != 1 || connection.models != 0 {
		t.Fatalf("cached catalog reprobed: probes=%d models=%d", connection.probes, connection.models)
	}
}

// flakyCatalogConnection fails its catalog command on demand while readiness
// stays confirmed, standing in for a CLI that is briefly unable to answer.
type flakyCatalogConnection struct {
	testConnection
	fail bool
}

func (connection *flakyCatalogConnection) Models(ctx context.Context) (external.Models, error) {
	if connection.fail {
		connection.models++
		return external.Models{}, errors.New("catalog command failed")
	}
	return connection.testConnection.Models(ctx)
}

func TestFailedCatalogRefreshServesLastKnownModels(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	connection := &flakyCatalogConnection{testConnection: testConnection{state: external.ConnectionState{Status: "ready"}}}
	entry := engines.entries[config.RuntimeClaude]
	entry.factory = func(context.Context) (external.Connection, error) { return connection, nil }
	first, err := engines.Models(t.Context(), config.RuntimeClaude)
	if err != nil || len(first.Items) != 1 {
		t.Fatalf("initial catalog: %#v %v", first, err)
	}
	entry.mu.Lock()
	entry.modelsExpiry = time.Unix(1, 0)
	entry.mu.Unlock()
	connection.fail = true
	// A ready engine that cannot answer right now keeps its last catalog instead
	// of turning into a connection error in the model menu.
	for range 2 {
		stale, err := engines.Models(t.Context(), config.RuntimeClaude)
		if err != nil || len(stale.Items) != len(first.Items) {
			t.Fatalf("stale catalog not served: %#v %v", stale, err)
		}
	}
}

func TestAcquireAndSelectionValidationReuseCachedCatalog(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
	starts := 0
	engines.entries[config.RuntimeCodex].factory = func(context.Context) (external.Connection, error) {
		starts++
		return connection, nil
	}
	selection := config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{Model: "runtime-model", Effort: "high"}}
	_, release, err := engines.Acquire(t.Context(), selection, config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := engines.ValidateSelection(t.Context(), selection); err != nil {
			t.Fatal(err)
		}
	}
	if starts != 1 || connection.probes != 0 || connection.models != 1 {
		t.Fatalf("selection change respawned the engine: starts=%d probes=%d models=%d", starts, connection.probes, connection.models)
	}
	_, secondRelease, err := engines.Acquire(t.Context(), selection, config.Config{})
	if err != nil || starts != 1 || connection.models != 1 {
		t.Fatalf("second admission respawned the engine: starts=%d models=%d err=%v", starts, connection.models, err)
	}
	release()
	secondRelease()
	engines.entries[config.RuntimeCodex].mu.Lock()
	engines.entries[config.RuntimeCodex].modelsExpiry = time.Unix(1, 0)
	engines.entries[config.RuntimeCodex].mu.Unlock()
	if err := engines.ValidateSelection(t.Context(), selection); err != nil || connection.models != 2 {
		t.Fatalf("expired catalog was not refreshed: models=%d err=%v", connection.models, err)
	}
}

func TestSignedOutEngineIsNotProbedOnEveryRead(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	connection := &testConnection{state: external.ConnectionState{Status: "auth_required"}}
	starts := 0
	engines.entries[config.RuntimeGrok].factory = func(context.Context) (external.Connection, error) {
		starts++
		return connection, nil
	}
	for range 3 {
		if _, err := engines.Models(t.Context(), config.RuntimeGrok); !errors.Is(err, ErrEngineNotReady) {
			t.Fatalf("signed-out engine served models: %v", err)
		}
	}
	// The negative result is remembered, so repeated menu reads reuse it.
	if starts != 1 {
		t.Fatalf("negative probe repeated: starts=%d", starts)
	}
	if _, err := engines.Check(t.Context(), config.RuntimeGrok); err != nil || starts != 2 {
		t.Fatalf("explicit check did not bypass the backoff: starts=%d err=%v", starts, err)
	}
}

func TestModelCacheExpiresAndCannotOutliveEngineClose(t *testing.T) {
	engines := NewEngines()
	connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
	engines.entries[config.RuntimeGrok].factory = func(context.Context) (external.Connection, error) { return connection, nil }
	for range 2 {
		if _, err := engines.Models(t.Context(), config.RuntimeGrok); err != nil {
			t.Fatal(err)
		}
	}
	if connection.models != 1 || connection.probes != 0 {
		t.Fatalf("cached catalog reprobed: models=%d probes=%d", connection.models, connection.probes)
	}
	engines.entries[config.RuntimeGrok].modelsExpiry = time.Unix(1, 0)
	if _, err := engines.Models(t.Context(), config.RuntimeGrok); err != nil || connection.models != 2 {
		t.Fatalf("expired catalog was not refreshed: models=%d err=%v", connection.models, err)
	}
	if err := engines.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := engines.Models(t.Context(), config.RuntimeGrok); !errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("closed engine returned cached models: %v", err)
	}
}

func TestExplicitCheckInvalidatesModelsWithoutReplacingLeasedConnection(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	connection := &testConnection{state: external.ConnectionState{Status: "ready"}}
	entry := engines.entries[config.RuntimeGrok]
	entry.factory = func(context.Context) (external.Connection, error) { return connection, nil }
	if _, err := engines.Models(t.Context(), config.RuntimeGrok); err != nil {
		t.Fatal(err)
	}
	entry.users = 1
	if _, err := engines.Check(t.Context(), config.RuntimeGrok); err != nil {
		t.Fatal(err)
	}
	if _, err := engines.Models(t.Context(), config.RuntimeGrok); err != nil || connection.models != 2 || connection.closes != 0 {
		t.Fatalf("leased check did not refresh catalog safely: models=%d closes=%d err=%v", connection.models, connection.closes, err)
	}
	connection.state = external.ConnectionState{Status: "auth_required"}
	if _, err := engines.Models(t.Context(), config.RuntimeGrok); !errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("signed-out engine returned cached models: %v", err)
	}
	entry.users = 0
}
