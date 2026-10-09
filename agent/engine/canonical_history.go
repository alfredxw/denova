package engine

import (
	"encoding/json"
	"errors"

	agenthistory "github.com/alfredxw/denova/agent/context/history"
)

// encodeCanonicalWindow retains only the bodies needed by the accepted
// compaction projection. Direct imports without an archive source stay whole.
func encodeCanonicalWindow(state engineTranscript, capabilities map[string]json.RawMessage) (json.RawMessage, error) {
	state, err := projectCanonicalWindow(state, capabilities)
	if err != nil {
		return nil, err
	}
	return json.Marshal(state)
}

func projectCanonicalWindow(state engineTranscript, capabilities map[string]json.RawMessage) (engineTranscript, error) {
	if state.HistoryHead.Identity == "" && state.Archive == nil {
		return state, nil
	}
	clear, clearPresent, err := agenthistory.ClearStateFrom(capabilities)
	if err != nil {
		return engineTranscript{}, err
	}
	compact, present, err := agenthistory.CompactionStateFrom(capabilities)
	if err != nil {
		return engineTranscript{}, err
	}
	compact, present = agenthistory.ClearCompaction(compact, present, clear, clearPresent)
	if state.HistoryHead.Identity != "" && present && !compact.Removed {
		state.Messages, state.Archive = agenthistory.ArchiveHistory(state.Messages, state.Archive, compact, state.ContextState)
	}
	if state.Archive != nil && (!present || compact.Removed) {
		return engineTranscript{}, errors.New("canonical archive requires its accepted compaction")
	}
	state.Version = transcriptVersion(state.Archive)
	return state, nil
}
