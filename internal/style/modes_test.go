package style

import (
	"reflect"
	"testing"
)

func TestNormalizeModesDefaultsAndCanonicalizes(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{name: "legacy missing", want: []string{ModeWriting, ModeGame}},
		{name: "unsupported only", input: []string{"future"}, want: []string{ModeWriting, ModeGame}},
		{name: "dedupe and order", input: []string{ModeGame, ModeWriting, ModeGame}, want: []string{ModeWriting, ModeGame}},
		{name: "writing only", input: []string{ModeWriting}, want: []string{ModeWriting}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeModes(test.input); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("NormalizeModes(%#v) = %#v, want %#v", test.input, got, test.want)
			}
		})
	}
}

func TestSupportsUsesLegacyDefault(t *testing.T) {
	if !Supports(nil, ModeWriting) || !Supports(nil, ModeGame) {
		t.Fatal("legacy styles should support both modes")
	}
	if Supports([]string{ModeWriting}, ModeGame) {
		t.Fatal("writing-only style should not support game mode")
	}
}
