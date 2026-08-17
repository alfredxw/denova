package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	agent "github.com/alfredxw/denova/agent"
)

func TestExportHistoryJSONLStreamsEveryDomainRecord(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("export")
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < sessionExportBatchTransactions+2; index++ {
		if err := sess.Append(agent.UserMessage(strings.Repeat("x", index+1))); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.AppendClearMarker(); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := sess.ExportHistoryJSONL(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	exported := output.String()
	scanner := bufio.NewScanner(strings.NewReader(exported))
	lines := 0
	for scanner.Scan() {
		if !json.Valid(scanner.Bytes()) {
			t.Fatalf("export line %d is invalid JSON", lines+1)
		}
		lines++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	// One header, ten messages, and one clear marker. Physical transaction
	// envelopes and checksums intentionally remain an implementation detail.
	if lines != sessionExportBatchTransactions+4 {
		t.Fatalf("export lines=%d want=%d", lines, sessionExportBatchTransactions+4)
	}
	if !strings.Contains(exported, `"type":"clear"`) {
		t.Fatal("export omitted the durable clear marker")
	}
}
