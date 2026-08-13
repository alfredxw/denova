package agent

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type testCombinedContextSource struct {
	identity  CapabilityIdentity
	fragments []ContextFragment
	err       error
}

func (source testCombinedContextSource) Identity() CapabilityIdentity { return source.identity }

func (source testCombinedContextSource) Materialize(context.Context, ContextRequest) ([]ContextFragment, error) {
	return append([]ContextFragment(nil), source.fragments...), source.err
}

func TestCombineContextSourcesPreservesOrderAndIdentity(t *testing.T) {
	first := testCombinedContextSource{
		identity:  CapabilityIdentity{Kind: "context.first", Version: 1},
		fragments: []ContextFragment{{Resource: "first"}},
	}
	second := testCombinedContextSource{
		identity:  CapabilityIdentity{Kind: "context.second", Version: 2, ConfigHash: "config"},
		fragments: []ContextFragment{{Resource: "second"}},
	}

	combined, err := CombineContextSources(nil, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if identity := combined.Identity(); identity.Kind != "context.combined" || identity.Version != 1 || identity.ConfigHash == "" {
		t.Fatalf("unexpected combined identity: %#v", identity)
	}
	fragments, err := combined.Materialize(context.Background(), ContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{fragments[0].Resource, fragments[1].Resource}; !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("combined context order = %#v", got)
	}

	reversed, err := CombineContextSources(second, first)
	if err != nil {
		t.Fatal(err)
	}
	if reversed.Identity().ConfigHash == combined.Identity().ConfigHash {
		t.Fatal("combined identity must include source order")
	}
}

func TestCombineContextSourcesHandlesEmptySingleAndFailure(t *testing.T) {
	empty, err := CombineContextSources(nil)
	if err != nil || empty != nil {
		t.Fatalf("empty combination = %#v, %v", empty, err)
	}
	one := testCombinedContextSource{identity: CapabilityIdentity{Kind: "context.one", Version: 1}}
	single, err := CombineContextSources(one)
	if err != nil || single.Identity() != one.Identity() {
		t.Fatalf("single combination = %#v, %v", single, err)
	}
	_, err = CombineContextSources(testCombinedContextSource{})
	if err == nil || !strings.Contains(err.Error(), "capability identity is incomplete") {
		t.Fatalf("incomplete identity error = %v", err)
	}

	failed, err := CombineContextSources(
		one,
		testCombinedContextSource{identity: CapabilityIdentity{Kind: "context.failed", Version: 1}, err: errors.New("boom")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := failed.Materialize(context.Background(), ContextRequest{}); err == nil ||
		!strings.Contains(err.Error(), "context.failed") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("materialization error = %v", err)
	}
}
