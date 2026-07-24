package runtime

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

func testBinding(key string) BindingRef {
	return testBindingAt("/book", key)
}

func testBindingAt(workspace, key string) BindingRef {
	return BindingRef{
		Kind: "test", Profile: "default", Key: key,
		Labels: map[string]string{"workspace": workspace},
	}
}

func testGameBinding(workspace, story, branch string) BindingRef {
	return BindingRef{
		Kind: "test-game", Profile: "default", Key: story + ":" + branch,
		Labels: map[string]string{"workspace": workspace, "story": story, "branch": branch},
	}
}

func TestBindingRefValidationIsOpenAndBounded(t *testing.T) {
	t.Parallel()

	valid := BindingRef{
		Kind: "custom-host", Profile: "experimental", Key: "run-1",
		Labels: map[string]string{"tenant": "creator-1"},
	}
	if err := ValidateBindingRef(valid); err != nil {
		t.Fatalf("open application taxonomy rejected: %v", err)
	}

	tooManyLabels := valid.Clone()
	tooManyLabels.Labels = make(map[string]string, maxBindingLabels+1)
	for index := 0; index <= maxBindingLabels; index++ {
		tooManyLabels.Labels["label-"+strconv.Itoa(index)] = "value"
	}
	for _, invalid := range []BindingRef{
		{Kind: "", Key: "run"},
		{Kind: "custom", Key: " run "},
		{Kind: "custom", Profile: strings.Repeat("p", maxBindingProfileBytes+1), Key: "run"},
		{Kind: "custom", Key: "run", Labels: map[string]string{"tenant": strings.Repeat("v", maxBindingLabelValueBytes+1)}},
		tooManyLabels,
	} {
		if err := ValidateBindingRef(invalid); !errors.Is(err, ErrInvalidBinding) {
			t.Fatalf("ValidateBindingRef(%#v) error = %v, want ErrInvalidBinding", invalid, err)
		}
	}
}

func TestRuntimeOwnsBindingLabelsAcrossAdapterBoundaries(t *testing.T) {
	t.Parallel()

	labels := map[string]string{"workspace": "/book"}
	ref, err := BindingReference(BindingRef{Kind: "custom", Key: "run", Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	labels["workspace"] = "/mutated-caller"
	if ref.Label("workspace") != "/book" {
		t.Fatalf("BindingReference retained caller-owned labels: %#v", ref.Labels)
	}

	runtime, err := NewRuntime(EngineFactoryFunc(func(_ context.Context, binding BindingRef) (Engine, error) {
		binding.Labels["workspace"] = "/mutated-adapter"
		return NewScriptedEngine(), nil
	}), NewMemoryJournalStore(), RuntimeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	harness, err := runtime.Open(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	status, err := harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Binding.Label("workspace") != "/book" {
		t.Fatalf("engine adapter mutated runtime binding: %#v", status.Binding.Labels)
	}
	status.Binding.Labels["workspace"] = "/mutated-projection"
	status, err = harness.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Binding.Label("workspace") != "/book" {
		t.Fatalf("status projection aliases actor binding: %#v", status.Binding.Labels)
	}
}
