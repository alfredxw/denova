package state

import (
	"reflect"
	"testing"
)

func TestNormalizeUpdatesCanonicalizesOperationAndPreservesSubmittedValues(t *testing.T) {
	updates := NormalizeUpdates([]Update{
		{Op: " REPLACE ", Path: " /actor/profile/name ", Value: "Ada"},
		{Op: " DELTA ", Path: " /actor/score ", Value: 2},
	})
	want := []Update{
		{Op: Replace, Path: "/actor/profile/name", Value: "Ada"},
		{Op: Delta, Path: "/actor/score", Value: 2},
	}
	if !reflect.DeepEqual(updates, want) {
		t.Fatalf("NormalizeUpdates() = %#v, want %#v", updates, want)
	}
}

func TestValidateUpdateRejectsInvalidOperationPathAndDeltaValue(t *testing.T) {
	tests := []Update{
		{Op: "merge", Path: "/actor/profile", Value: map[string]any{"name": "Ada"}},
		{Op: Replace, Path: "actor/profile", Value: "Ada"},
		{Op: Delta, Path: "/actor/score", Value: "two"},
	}
	for _, update := range tests {
		if err := ValidateUpdate(update); err == nil {
			t.Fatalf("ValidateUpdate accepted %#v", update)
		}
	}
}

func TestPathHelpersRoundTripEscapedSegmentsAndMutateNestedValue(t *testing.T) {
	segments := []string{"inventory", "a/b", "~draft"}
	path := FormatPath(segments)
	parsed, err := ParsePath(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(parsed, segments) {
		t.Fatalf("ParsePath(%q) = %#v, want %#v", path, parsed, segments)
	}

	root := map[string]any{"inventory": map[string]any{"a/b": map[string]any{"~draft": 1}}}
	if err := SetNestedValue(root, parsed, 2, true); err != nil {
		t.Fatal(err)
	}
	got, ok := NestedValue(root, parsed)
	if !ok || got != 2 {
		t.Fatalf("NestedValue after mutation = %#v, %t; want 2, true", got, ok)
	}
}
