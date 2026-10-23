package grok

import (
	"context"
	"errors"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestInterruptedCheckPreservesReadinessAndClosedCheckDoesNot(t *testing.T) {
	client := fixtureClient(t, "models")
	client.state = external.ConnectionState{Status: "ready"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state, err := client.Check(ctx)
	if !errors.Is(err, context.Canceled) || state.Status != "ready" || client.Status() != state {
		t.Fatalf("interrupted probe lost confirmed state: %+v %+v %v", state, client.Status(), err)
	}
	_ = client.Close()
	state, err = client.Check(t.Context())
	if err == nil || state.Status != "unavailable" || client.Status() != state {
		t.Fatalf("closed probe retained readiness: %+v %+v %v", state, client.Status(), err)
	}
}

// The readiness check reads the catalog anyway, so one probe must return both.
func TestProbeAnswersReadinessAndCatalogTogether(t *testing.T) {
	client := fixtureClient(t, "models")
	state, models, err := client.Probe(t.Context())
	if err != nil || state.Status != "ready" || len(models.Items) != 2 || models.DefaultID != "grok-4.7" {
		t.Fatalf("probe: %+v %#v %v", state, models, err)
	}
}

// A CLI that reports no account must map to an actionable login state instead of
// a generic connection failure, otherwise the UI reports the wrong cause.
func TestSignedOutProbeReportsAuthenticationRequired(t *testing.T) {
	client := fixtureClient(t, "signed-out")
	state, _, err := client.Probe(t.Context())
	if err != nil || state.Status != "auth_required" || client.Status().Status != "auth_required" {
		t.Fatalf("signed-out probe: %+v %+v %v", state, client.Status(), err)
	}
}

// A catalog the CLI cannot describe means no model can be selected, so the
// failure must replace a previously confirmed readiness.
func TestCatalogFailureReplacesPreviouslyReadyState(t *testing.T) {
	client := fixtureClient(t, "no-catalog")
	client.state = external.ConnectionState{Status: "ready"}
	state, err := client.Check(t.Context())
	if err == nil || state.Status != "unavailable" || client.Status() != state {
		t.Fatalf("catalog failure retained readiness: %+v %+v %v", state, client.Status(), err)
	}
}
