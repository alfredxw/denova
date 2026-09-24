package agentruntime

import (
	"context"
	"errors"
	"testing"

	"denova/config"
	"denova/internal/agents/runtime/external"
)

func TestAPILeasesIsolateRoutingAndNeverProbeCLIAccount(t *testing.T) {
	engines := NewEngines()
	defer engines.Close()
	var routes []config.ResolvedModelSettings
	var connections []*testConnection
	engines.apiFactory = func(_ context.Context, _ config.RuntimeID, route config.ResolvedModelSettings) (external.Connection, error) {
		routes = append(routes, route)
		connection := &testConnection{state: external.ConnectionState{Status: "auth_required"}}
		connections = append(connections, connection)
		return connection, nil
	}
	selection := config.RuntimeSelection{Kind: config.RuntimeCodex, Codex: &config.CodexRuntimeSettings{ProfileID: "api"}}
	cfg := config.Config{ModelProfiles: []config.ModelProfileSettings{{ID: "api", EndpointID: "endpoint", Model: "not-in-cli-catalog"}}, ModelEndpoints: []config.ModelEndpointSettings{{ID: "endpoint", Provider: "openai", BaseURL: "http://localhost:9999/v1", APIKey: "first"}}}
	first, releaseFirst, err := engines.Acquire(t.Context(), selection, cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ModelEndpoints[0].APIKey = "second"
	second, releaseSecond, err := engines.Acquire(t.Context(), selection, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || routes[0].APIKey != "first" || routes[1].APIKey != "second" {
		t.Fatal("operations shared mutable routing")
	}
	for _, connection := range connections {
		if connection.probes != 0 || connection.models != 0 {
			t.Fatal("API selection required a CLI account or catalog")
		}
	}
	releaseFirst()
	releaseFirst()
	if connections[0].closes != 1 || connections[1].closes != 0 {
		t.Fatal("release closed another operation")
	}
	if err := engines.Close(); err != nil {
		t.Fatal(err)
	}
	releaseSecond()
	if connections[1].closes != 1 {
		t.Fatal("shutdown did not release exactly once")
	}
	if _, _, err := engines.Acquire(t.Context(), selection, cfg); !errors.Is(err, ErrEngineNotReady) {
		t.Fatal("closed engine admitted an API operation")
	}
}
