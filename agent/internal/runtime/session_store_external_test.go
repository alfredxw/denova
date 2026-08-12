package runtime_test

import (
	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	sessionfile "github.com/alfredxw/denova/agent/internal/sessionfile"
)

func newCanonicalFileJournalStore(root string) (runstate.JournalStore, error) {
	store, err := sessionfile.New(root)
	if err != nil {
		return nil, err
	}
	return runstate.NewSessionJournalStore(store), nil
}
