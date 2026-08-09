package filewatch

import (
	"reflect"
	"testing"
)

func TestChangeCoalescer(t *testing.T) {
	tests := []struct {
		name string
		in   []Change
		want []Change
	}{
		{
			name: "create followed by writes remains create",
			in: []Change{
				{Path: "chapters/new.md", Type: ChangeAdded},
				{Path: "chapters/new.md", Type: ChangeUpdated},
				{Path: "chapters/new.md", Type: ChangeUpdated},
			},
			want: []Change{{Path: "chapters/new.md", Type: ChangeAdded}},
		},
		{
			name: "short lived file disappears",
			in: []Change{
				{Path: "chapters/transient.md", Type: ChangeAdded},
				{Path: "chapters/transient.md", Type: ChangeDeleted},
			},
			want: nil,
		},
		{
			name: "atomic replacement becomes update",
			in: []Change{
				{Path: "chapters/ch01.md", Type: ChangeDeleted},
				{Path: "chapters/ch01.md", Type: ChangeAdded},
			},
			want: []Change{{Path: "chapters/ch01.md", Type: ChangeUpdated}},
		},
		{
			name: "delete wins over earlier update",
			in: []Change{
				{Path: "chapters/ch01.md", Type: ChangeUpdated},
				{Path: "chapters/ch01.md", Type: ChangeDeleted},
			},
			want: []Change{{Path: "chapters/ch01.md", Type: ChangeDeleted}},
		},
		{
			name: "parent deletion suppresses redundant children",
			in: []Change{
				{Path: "chapters/volume/ch01.md", Type: ChangeDeleted},
				{Path: "chapters/volume", Type: ChangeDeleted},
				{Path: "ideas.md", Type: ChangeUpdated},
			},
			want: []Change{
				{Path: "chapters/volume", Type: ChangeDeleted},
				{Path: "ideas.md", Type: ChangeUpdated},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			coalescer := newChangeCoalescer()
			for _, change := range test.in {
				coalescer.add(change)
			}
			if got := coalescer.take(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("coalesced changes = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestHiddenPathDetection(t *testing.T) {
	for _, path := range []string{".git/config", "chapters/.draft.md", "setting/.cache/item"} {
		if !hasHiddenSegment(path) {
			t.Fatalf("hasHiddenSegment(%q) = false", path)
		}
	}
	if hasHiddenSegment("chapters/draft.md") {
		t.Fatal("visible path was classified as hidden")
	}
}

func TestIgnoredPathDetection(t *testing.T) {
	for _, path := range []string{"node_modules/pkg/index.js", "app/dist/bundle.js", "vendor/module.go"} {
		if !isIgnoredPath(path) {
			t.Fatalf("isIgnoredPath(%q) = false", path)
		}
	}
	if isIgnoredPath("chapters/building-a-world.md") {
		t.Fatal("visible path was classified as ignored")
	}
}
