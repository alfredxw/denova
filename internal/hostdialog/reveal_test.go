package hostdialog

import (
	"reflect"
	"testing"
)

func TestRevealCommandUsesNativeFileManagerConventions(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		path      string
		directory bool
		command   string
		args      []string
	}{
		{name: "macOS file", goos: "darwin", path: "/projects/story/chapter.md", command: "open", args: []string{"-R", "/projects/story/chapter.md"}},
		{name: "Windows file", goos: "windows", path: `C:\projects\story\chapter.md`, command: "explorer.exe", args: []string{"/select,", `C:\projects\story\chapter.md`}},
		{name: "Linux file", goos: "linux", path: "/projects/story/chapter.md", command: "xdg-open", args: []string{"/projects/story"}},
		{name: "Linux directory", goos: "linux", path: "/projects/story/chapters", directory: true, command: "xdg-open", args: []string{"/projects/story/chapters"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command, args, err := revealCommand(test.goos, test.path, test.directory)
			if err != nil {
				t.Fatal(err)
			}
			if command != test.command || !reflect.DeepEqual(args, test.args) {
				t.Fatalf("reveal command = %q %#v, want %q %#v", command, args, test.command, test.args)
			}
		})
	}
}

func TestRevealCommandRejectsUnsupportedPlatforms(t *testing.T) {
	if _, _, err := revealCommand("plan9", "/projects/story", true); err == nil {
		t.Fatal("unsupported platform should fail")
	}
}
