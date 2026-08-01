package filejournal

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	runstate "github.com/alfredxw/denova/agent/runtime"
)

var fallbackIDSequence atomic.Uint64

func newID(prefix string) string {
	var random [16]byte
	if _, err := rand.Read(random[:]); err == nil {
		return prefix + "-" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("%s-fallback-%x-%x-%x", prefix, time.Now().UTC().UnixNano(), os.Getpid(), fallbackIDSequence.Add(1))
}

func cloneJournalEvents(events []runstate.Event) []runstate.Event {
	cloned := make([]runstate.Event, len(events))
	for index, event := range events {
		encoded, err := runstate.EncodeJournalEvent(event)
		if err != nil {
			// All events reached this helper only after the same codec accepted
			// them for durable storage. A mismatch is an internal invariant break.
			panic(fmt.Sprintf("clone durable event at cursor %d: %v", event.Cursor, err))
		}
		decoded, err := runstate.DecodeJournalEvent(encoded)
		if err != nil {
			panic(fmt.Sprintf("decode cloned durable event at cursor %d: %v", event.Cursor, err))
		}
		cloned[index] = decoded
	}
	return cloned
}
