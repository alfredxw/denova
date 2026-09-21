package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestStartupTemplateUsesSupportedFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg Config
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&cfg); err != nil {
		t.Fatalf("startup template must decode without duplicate or ignored fields: %v", err)
	}
}

func TestStartupTemplatePreservesDataDirectorySelection(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DENOVA_DIR", "")
	t.Setenv("NOVA_DIR", "")
	for _, test := range []struct {
		name string
		dirs []string
		want string
	}{
		{name: "fresh installation", want: ".denova"},
		{name: "existing legacy data", dirs: []string{".nova"}, want: ".nova"},
		{name: "current data takes precedence", dirs: []string{".nova", ".denova"}, want: ".denova"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			if err := os.WriteFile("config.toml", data, 0600); err != nil {
				t.Fatal(err)
			}
			for _, dir := range test.dirs {
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if got, want := startupNovaDir(), normalizePath(filepath.Join(root, test.want)); got != want {
				t.Fatalf("startup data directory = %q, want %q", got, want)
			}
		})
	}
}
