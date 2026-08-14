package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseRunsAttributesLLMCallByRunIDToTurn guards against WR-01: normal
// llm_call spans in this project carry no data.turn_id (only the record-level
// run_id plus data.attrs); the parser must learn each run's turn_id from a
// prior run_created / run_context entry and attribute llm_call counts and
// reasoning tokens to that turn.
func TestParseRunsAttributesLLMCallByRunIDToTurn(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs.jsonl")
	lines := []string{
		`{"type":"run_created","run_id":"run-A","data":{"turn_id":"turn-1"}}`,
		`{"type":"llm_call","run_id":"run-A","data":{"attrs":{"reasoning_tokens":42,"prompt_tokens":10,"completion_tokens":5}}}`,
		`{"type":"llm_call","run_id":"run-A","data":{"attrs":{"reasoning_tokens":7}}}`,
		`{"type":"run_created","run_id":"run-B","data":{"turn_id":"turn-2"}}`,
		`{"type":"llm_call","run_id":"run-B","data":{"attrs":{"reasoning_tokens":0}}}`,
		// No run_created entry at all — should fall through to "other".
		`{"type":"llm_call","run_id":"run-orphan","data":{"attrs":{"reasoning_tokens":3}}}`,
	}
	if err := os.WriteFile(runs, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write runs: %v", err)
	}
	agg := parseRuns([]string{runs})
	slot1 := agg["turn-1"]
	if slot1 == nil || slot1.LLMCalls != 2 || slot1.ReasoningTokens != 49 {
		t.Fatalf("turn-1 expected 2 calls / 49 tokens, got %#v", slot1)
	}
	slot2 := agg["turn-2"]
	if slot2 == nil || slot2.LLMCalls != 1 || slot2.ReasoningTokens != 0 {
		t.Fatalf("turn-2 expected 1 call / 0 tokens, got %#v", slot2)
	}
	other := agg["other"]
	if other == nil || other.LLMCalls != 1 || other.ReasoningTokens != 3 {
		t.Fatalf("other expected 1 call / 3 tokens, got %#v", other)
	}
}

// TestParseRunsHandlesMissingRunsFile silently maps an unreadable file to a
// warning rather than panicking; parseRuns must continue aggregating from
// remaining inputs.
func TestParseRunsHandlesMissingRunsFile(t *testing.T) {
	agg := parseRuns([]string{filepath.Join(t.TempDir(), "missing.jsonl")})
	if len(agg) != 0 {
		t.Fatalf("missing runs file must produce empty aggregation: %#v", agg)
	}
}

// TestParseRunsIgnoresUnrelatedRecordTypes guards the type switch so future
// trace span additions (run_finished, tool_call, etc.) don't accidentally
// register as llm_call aggregates.
func TestParseRunsIgnoresUnrelatedRecordTypes(t *testing.T) {
	dir := t.TempDir()
	runs := filepath.Join(dir, "runs.jsonl")
	rec := struct {
		Type  string `json:"type"`
		RunID string `json:"run_id"`
		Data  struct {
			TurnID string `json:"turn_id"`
			Attrs  struct {
				ReasoningTokens int `json:"reasoning_tokens"`
			} `json:"attrs"`
		} `json:"data"`
	}{}
	lines := []string{
		func() string {
			rec.Type = "run_finished"
			rec.RunID = "run-A"
			b, _ := json.Marshal(rec)
			return string(b)
		}(),
		func() string {
			rec.Type = "tool_call"
			rec.RunID = "run-A"
			b, _ := json.Marshal(rec)
			return string(b)
		}(),
	}
	if err := os.WriteFile(runs, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write runs: %v", err)
	}
	agg := parseRuns([]string{runs})
	if len(agg) != 0 {
		t.Fatalf("only llm_call records should contribute: %#v", agg)
	}
}
