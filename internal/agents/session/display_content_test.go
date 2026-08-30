package session

import (
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestDisplayContentDoesNotChangeCanonicalModelHistory(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("configuration")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendWithMetadata(agent.UserMessage("/configuration\n\nhost context\n\nUpdate the preset."), MessageMetadata{
		DisplayContent: "Update the preset.",
	}); err != nil {
		t.Fatal(err)
	}

	history := sess.History()
	if len(history) != 1 || history[0].Content != "Update the preset." {
		t.Fatalf("display history = %#v", history)
	}
	effective := sess.GetEffectiveMessages()
	if len(effective) != 1 || effective[0].Content != "/configuration\n\nhost context\n\nUpdate the preset." {
		t.Fatalf("canonical model history = %#v", effective)
	}

	reloadedStore, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.Get("configuration")
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.History(); len(got) != 1 || got[0].Content != "Update the preset." {
		t.Fatalf("reloaded display history = %#v", got)
	}
}
