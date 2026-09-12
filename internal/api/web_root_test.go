package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWebRoot(t *testing.T) {
	for _, tc := range []struct {
		name     string
		files    []string
		override string
		want     string
	}{
		{
			name:  "repository serves compiled frontend instead of TypeScript sources",
			files: []string{"web/index.html", "web/src/main.tsx", "web/dist/index.html"},
			want:  "web/dist",
		},
		{
			name:  "packaged frontend remains supported",
			files: []string{"web/index.html", "web/assets/app.js"},
			want:  "web",
		},
		{
			name:     "explicit assets take precedence over repository build",
			files:    []string{"custom/index.html", "web/dist/index.html"},
			override: "custom",
			want:     "custom",
		},
		{
			name:     "explicit source directory resolves its build",
			files:    []string{"custom/index.html", "custom/src/main.tsx", "custom/dist/index.html"},
			override: "custom",
			want:     "custom/dist",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("DENOVA_WEB_DIR", tc.override)
			t.Setenv("NOVA_WEB_DIR", "")
			for _, name := range tc.files {
				if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(name, []byte("fixture"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got, want := resolveWebRoot(), normalizeStaticRoot(tc.want); got != want {
				t.Fatalf("static root = %q, want %q", got, want)
			}
		})
	}
}

func TestResolveWebRootNeverServesUnbuiltSource(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DENOVA_WEB_DIR", "")
	t.Setenv("NOVA_WEB_DIR", "")
	if err := os.MkdirAll("web/src", 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"web/index.html", "web/src/main.tsx"} {
		if err := os.WriteFile(name, []byte("unbuilt frontend"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A binary may fall back to embedded assets, but must never expose the source tree.
	if got := resolveWebRoot(); got == normalizeStaticRoot("web") {
		t.Fatalf("unbuilt TypeScript source selected as static assets: %s", got)
	}
}
