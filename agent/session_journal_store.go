package agent

import (
	runstate "github.com/alfredxw/denova/agent/internal/runtime"
	agentsession "github.com/alfredxw/denova/agent/session"
)

func journalStoreFor(store agentsession.Store) runstate.JournalStore {
	return runstate.NewSessionJournalStore(store)
}
