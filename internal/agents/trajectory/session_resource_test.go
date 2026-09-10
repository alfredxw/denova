package trajectory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	agenttools "github.com/alfredxw/denova/agent/tools"

	"denova/internal/agents/session"
)

func TestSessionResourceReadsBeyondResidentHistory(t *testing.T) {
	const count = 420
	messages := make([]*agent.Message, count)
	for index := range count {
		messages[index] = agent.UserMessage(fmt.Sprintf("message-%03d", index))
	}
	catalog, source, target, resource := newSessionResourceFixture(t, messages...)
	first, err := catalog.read(context.Background(), readInput{Path: resource, Limit: 31})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first.Content, "message-000") || !first.Truncated || first.Total != count+1 {
		t.Fatalf("early evidence or history coverage is missing: first marker=%t, truncated=%t, total=%d", strings.Contains(first.Content, "message-000"), first.Truncated, first.Total)
	}
	definition := sessionReadTool(t, catalog, 32<<10)
	input := readInput{Path: resource, Limit: 31}
	var got []string
	for window := 0; ; window++ {
		if window > count {
			t.Fatal("Session continuation did not terminate")
		}
		envelope, content := readSessionToolWindow(t, definition, input, 32<<10)
		if window == 0 {
			if envelope.Status != "partial" || !envelope.Limits.Truncated || envelope.Limits.Total != count+1 {
				t.Fatalf("incomplete history must disclose its full range: %+v", envelope)
			}
			// A new tail record must not change the positions of earlier history.
			if err := target.Append(agent.UserMessage("appended-during-pagination")); err != nil {
				t.Fatal(err)
			}
		}
		for _, line := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
			var entry struct {
				Type    string `json:"type"`
				Content string `json:"content"`
			}
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Fatalf("invalid Session JSONL: %v: %q", err, line)
			}
			if entry.Type == "message" {
				got = append(got, entry.Content)
			}
		}
		if !envelope.Limits.Truncated {
			if envelope.Status != "success" || envelope.Limits.NextOffset != 0 || envelope.Limits.Total != count+2 {
				t.Fatalf("invalid final coverage: %+v", envelope)
			}
			break
		}
		if envelope.Limits.NextOffset <= input.Offset {
			t.Fatalf("continuation did not advance: %+v", envelope)
		}
		input.Offset = envelope.Limits.NextOffset
		input.Limit = 19
	}
	want := make([]string, count, count+1)
	for index := range count {
		want[index] = fmt.Sprintf("message-%03d", index)
	}
	want = append(want, "appended-during-pagination")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history contains omissions or duplicates: got %v, want %v", got, want)
	}
	// History must remain available on disk; this resource is only a projection.
	store, err := session.NewStore(sessionDir(source.StateRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	reopened, err := store.Get(target.ID)
	if err != nil {
		t.Fatal(err)
	}
	page, err := reopened.ReadHistoryPage(context.Background(), 1, 1)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].Content != want[0] {
		t.Fatalf("first durable entry changed: %+v, %v", page, err)
	}
}

func TestSessionResourceContinuesLargeUTF8RowsAndExcludesReasoning(t *testing.T) {
	catalog, source, target, resource := newSessionResourceFixture(t)
	if err := target.Append(agent.UserMessage("initial requirement")); err != nil {
		t.Fatal(err)
	}
	if err := target.AppendDisplayEvent(session.DisplayEvent{Role: "thinking", Content: "private-reasoning-marker"}); err != nil {
		t.Fatal(err)
	}
	longText := strings.Repeat("长文本界", 1500) + filepath.Join(source.Workspace, "chapter.md")
	if err := target.AppendDisplayEvent(session.DisplayEvent{Role: "assistant", Content: longText}); err != nil {
		t.Fatal(err)
	}
	if err := target.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := target.Append(agent.UserMessage("after-clear")); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(sessionDir(source.StateRoot), target.ID+".jsonl")
	before, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	full, err := catalog.read(context.Background(), readInput{Path: resource, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if full.Total != 6 || full.Limit != 6 || full.Truncated {
		t.Fatalf("unexpected complete Session window: total=%d, limit=%d, truncated=%t", full.Total, full.Limit, full.Truncated)
	}
	if strings.Contains(full.Content, "private-reasoning-marker") || strings.Contains(full.Content, source.Workspace) {
		t.Fatal("Session projection exposed private content")
	}
	if !strings.Contains(full.Content, "[private-root]") || !strings.Contains(full.Content, `"type":"clear"`) || !strings.Contains(full.Content, "after-clear") {
		t.Fatal("Session projection lost observable history or its redaction marker")
	}
	const budget = 1024
	definition := sessionReadTool(t, catalog, budget)
	input := readInput{Path: resource, Limit: 100}
	var reconstructed strings.Builder
	byteContinuations := 0
	for window := 0; ; window++ {
		if window > 200 {
			t.Fatal("large row continuation did not terminate")
		}
		envelope, content := readSessionToolWindow(t, definition, input, budget)
		reconstructed.WriteString(content)
		if !envelope.Limits.Truncated {
			break
		}
		if envelope.Limits.NextByteOffset > 0 {
			byteContinuations++
			if envelope.Limits.NextOffset == input.Offset && envelope.Limits.NextByteOffset <= input.ByteOffset {
				t.Fatalf("byte continuation did not advance: %+v", envelope)
			}
		}
		input.Offset, input.ByteOffset = envelope.Limits.NextOffset, envelope.Limits.NextByteOffset
	}
	if byteContinuations == 0 || reconstructed.String() != full.Content {
		t.Fatalf("bounded windows failed to reconstruct exact history: byte continuations=%d", byteContinuations)
	}
	after, err := os.ReadFile(journalPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("trajectory reads changed the canonical journal: %v", err)
	}
}

func TestSessionResourceKeepsExactPositionsWithinSingleTurn(t *testing.T) {
	catalog, _, target, resource := newSessionResourceFixture(t)
	if err := target.Append(agent.UserMessage("long turn")); err != nil {
		t.Fatal(err)
	}
	const count = 110
	for index := range count {
		role := "assistant"
		if index%3 == 0 {
			role = "thinking"
		}
		if err := target.AppendDisplayEvent(session.DisplayEvent{Role: role, Content: fmt.Sprintf("segment-%03d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	// The UI reader expands this small request to include the user boundary;
	// the trajectory projection must still return exactly the selected slots.
	page, err := target.ReadHistoryPage(context.Background(), 80, 7)
	if err != nil || len(page.Entries) <= 7 {
		t.Fatalf("fixture did not create a whole-turn UI page: rows=%d, error=%v", len(page.Entries), err)
	}
	var lines []string
	for offset := 1; offset <= count+2; {
		result, err := catalog.read(context.Background(), readInput{Path: resource, Offset: offset, Limit: 7})
		if err != nil {
			t.Fatal(err)
		}
		window := strings.Split(strings.TrimSuffix(result.Content, "\n"), "\n")
		if len(window) > 7 || result.Limit != len(window) || result.Offset != offset || result.Total != count+2 {
			t.Fatalf("history window ignored exact requested positions: offset=%d, limit=%d, total=%d", result.Offset, result.Limit, result.Total)
		}
		lines = append(lines, window...)
		if !result.Truncated {
			break
		}
		if result.NextOffset <= offset {
			t.Fatal("history window did not advance")
		}
		offset = result.NextOffset
	}
	if len(lines) != count+2 {
		t.Fatalf("got %d lines, want %d", len(lines), count+2)
	}
	for index, line := range lines[2:] {
		if index%3 == 0 {
			if line != "null" {
				t.Fatalf("private reasoning left content at line %d", index+3)
			}
			continue
		}
		var entry session.HistoryEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.Content != fmt.Sprintf("segment-%03d", index) {
			t.Fatalf("history position %d changed: %+v, %v", index, entry, err)
		}
	}
}

func TestSessionResourceEmptyAndContinuationBoundaries(t *testing.T) {
	catalog, _, target, resource := newSessionResourceFixture(t)
	definition := sessionReadTool(t, catalog, 1024)
	envelope, content := readSessionToolWindow(t, definition, readInput{Path: resource, Limit: 1}, 1024)
	if envelope.Status != "success" || envelope.Limits.Total != 1 || envelope.Limits.Truncated {
		t.Fatalf("empty Session has incorrect coverage: %+v", envelope)
	}
	var manifest struct {
		Schema    string `json:"schema"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal([]byte(content), &manifest); err != nil || manifest.Schema != "denova.trajectory.session.v2" || manifest.SessionID != target.ID {
		t.Fatalf("empty Session manifest is invalid: %+v, %v", manifest, err)
	}
	if err := target.Append(agent.UserMessage("界")); err != nil {
		t.Fatal(err)
	}
	catalog.Limit = 1
	first, err := catalog.read(context.Background(), readInput{Path: resource, Limit: 100})
	if err != nil || first.Limit != 1 || !first.Truncated || first.NextOffset != 2 {
		t.Fatalf("configured cap was not applied: %+v, %v", first, err)
	}
	row, err := catalog.read(context.Background(), readInput{Path: resource, Offset: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	midRune := strings.Index(row.Content, "界") + 1
	for _, input := range []readInput{
		{Path: resource, Offset: 2, ByteOffset: midRune},
		{Path: resource, Offset: 2, ByteOffset: len(row.Content)},
		{Path: resource, Offset: 2, ByteOffset: len(row.Content) + 1},
		{Path: resource, Offset: 3, ByteOffset: 1},
		{Path: resource, Offset: math.MaxInt},
	} {
		if _, err := catalog.read(context.Background(), input); err == nil {
			t.Fatalf("invalid continuation was accepted: %+v", input)
		}
	}
	end, err := catalog.read(context.Background(), readInput{Path: resource, Offset: 3})
	if err != nil || end.Content != "" || end.Truncated || end.Total != 2 || end.NextOffset != 0 {
		t.Fatalf("read past the final line is invalid: %+v, %v", end, err)
	}
}

func newSessionResourceFixture(t *testing.T, initial ...*agent.Message) (Catalog, Source, *session.Session, string) {
	t.Helper()
	source := Source{ProjectID: "project-1", Name: "Test Project", StateRoot: t.TempDir(), Workspace: t.TempDir()}
	store, err := session.NewStore(sessionDir(source.StateRoot))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	if len(initial) > 0 {
		// Import an ordinary message JSONL fixture in one write instead of
		// syncing hundreds of independent user turns just to seed this test.
		var content strings.Builder
		encoder := json.NewEncoder(&content)
		for _, message := range initial {
			if err := encoder.Encode(message); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(sessionDir(source.StateRoot), "session-history.jsonl"), []byte(content.String()), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	target, err := store.GetOrCreate("session-history")
	if err != nil {
		t.Fatal(err)
	}
	catalog := Catalog{Sources: func(context.Context) ([]Source, error) { return []Source{source}, nil }}
	return catalog, source, target, Scheme + "projects/" + source.ProjectID + "/sessions/" + target.ID
}

func sessionReadTool(t *testing.T, catalog Catalog, budget int) agent.ToolDefinition {
	t.Helper()
	adapter, err := NewReadAdapter(catalog)
	if err != nil {
		t.Fatal(err)
	}
	definition, err := agenttools.Read([]agenttools.ReadAdapter{adapter}, agenttools.WithMaxResultBytes(budget))
	if err != nil {
		t.Fatal(err)
	}
	return definition
}

type sessionReadEnvelope struct {
	Status string `json:"status"`
	Limits struct {
		Total          int  `json:"total"`
		Truncated      bool `json:"truncated"`
		NextOffset     int  `json:"next_offset"`
		NextByteOffset int  `json:"next_byte_offset"`
	} `json:"limits"`
}

func readSessionToolWindow(t *testing.T, definition agent.ToolDefinition, input readInput, budget int) (sessionReadEnvelope, string) {
	t.Helper()
	arguments, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	result, err := definition.Tool.Run(context.Background(), string(arguments))
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(result.ModelContent) || len(result.ModelContent) > budget {
		t.Fatal("tool result exceeded its UTF-8 byte budget")
	}
	var envelope sessionReadEnvelope
	if err := json.Unmarshal(result.Details, &envelope); err != nil {
		t.Fatal(err)
	}
	_, numbered, _ := strings.Cut(result.ModelContent, "\n")
	var content strings.Builder
	for _, line := range strings.SplitAfter(numbered, "\n") {
		if line == "" {
			continue
		}
		_, body, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("expected a numbered resource line: %q", line)
		}
		content.WriteString(body)
	}
	return envelope, content.String()
}
