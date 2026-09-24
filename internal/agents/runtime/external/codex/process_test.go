package codex

import (
	"errors"
	"reflect"
	"slices"
	"testing"
)

func TestRuntimeEnvironmentSharesCodexConfiguration(t *testing.T) {
	base := []string{"PATH=tools", "HOME=user-home", "USERPROFILE=user-home", "CODEX_HOME=previous-home", "OPENAI_API_KEY=fixture-key", "CODEX_API_KEY=fixture-codex-key", "HTTPS_PROXY=http://localhost:7890"}
	want := []string{"PATH=tools", "HOME=user-home", "USERPROFILE=user-home", "OPENAI_API_KEY=fixture-key", "CODEX_API_KEY=fixture-codex-key", "HTTPS_PROXY=http://localhost:7890", "CODEX_HOME=shared-home"}
	before := slices.Clone(base)
	if got := sharedEnvironment(base, "shared-home"); !reflect.DeepEqual(got, want) {
		t.Fatalf("shared environment = %v, want %v", got, want)
	}
	if !reflect.DeepEqual(base, before) {
		t.Fatal("building the child environment changed the parent environment")
	}
}

func TestSupportedVersionUsesMinimumWithoutUpperBound(t *testing.T) {
	for _, test := range []struct {
		output string
		want   string
	}{
		{"codex-cli 0.130.0", "0.130.0"},
		{"codex-cli 0.130.1", "0.130.1"},
		{"codex-cli 0.139.0", "0.139.0"},
		{"codex-cli 0.1000.0", "0.1000.0"},
		{"codex-cli 1.0.0", "1.0.0"},
		{"codex-cli 0.131.0-alpha.1", "0.131.0-alpha.1"},
		{"codex-cli 0.130.0+build.5", "0.130.0+build.5"},
		{" codex-cli v0.139.0\r\n", "0.139.0"},
		{"codex-cli 0.129.999", ""},
		{"codex-cli 0.9.999", ""},
		{"codex-cli 0.130.0-alpha.1", ""},
		{"codex-cli 0.139", ""},
		{"codex-cli 00.139.0", ""},
		{"codex-cli unknown", ""},
		{"codex-cli 0.139.0\nunexpected output", ""},
		{"different-program 0.139.0", ""},
		{"", ""},
	} {
		t.Run(test.output, func(t *testing.T) {
			got, err := parseSupportedVersion(test.output)
			if got != test.want {
				t.Fatalf("version = %q, want %q (error: %v)", got, test.want, err)
			}
			if test.want == "" {
				if !errors.Is(err, ErrVersionUnsupported) {
					t.Fatalf("error = %v, want ErrVersionUnsupported", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
