package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHistoryPageBoundsOversizedToolLoopWithoutLosingResults(t *testing.T) {
	for _, fixture := range []struct{ count, bytes int }{{80, 128 << 10}, {600, 20}} {
		t.Run(fmt.Sprintf("%d-%d", fixture.count, fixture.bytes), func(t *testing.T) {
			directory := t.TempDir()
			file, err := os.Create(filepath.Join(directory, "tool-loop.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			writer := bufio.NewWriter(file)
			encoder := json.NewEncoder(writer)
			if err := encoder.Encode(map[string]string{"role": "user", "content": "Read all chapters"}); err != nil {
				t.Fatal(err)
			}
			body := strings.Repeat("x", fixture.bytes)
			for i := 0; i < fixture.count; i++ {
				err := encoder.Encode(displayRecord{Type: historyTypeDisplay, RecordID: fmt.Sprint(i), DisplayEvent: DisplayEvent{
					ID: fmt.Sprint(i), Role: "tool_call", Name: "read", Args: `{}`, Result: body, Status: "success", RunID: "one-run",
				}})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Flush(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := NewStore(directory)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			sess, err := store.Get("tool-loop")
			if err != nil {
				t.Fatal(err)
			}
			seen := make(map[string]bool)
			before, pages := -1, 0
			for {
				page, err := sess.ReadHistoryPage(t.Context(), before, 50)
				if err != nil {
					t.Fatal(err)
				}
				pages++
				bytes := 0
				for _, entry := range page.Entries {
					if entry.Role == "user" {
						continue
					}
					if seen[entry.ID] || entry.Result != body {
						t.Fatalf("duplicate or changed tool result %q", entry.ID)
					}
					seen[entry.ID] = true
					bytes += len(entry.Result) + len(entry.Args)
				}
				if bytes > historyPageMaxBytes || len(page.Entries) > 2*sessionHistoryAnchorEvery {
					t.Fatalf("unbounded page: bytes=%d rows=%d", bytes, len(page.Entries))
				}
				if !page.HasMore {
					break
				}
				if before >= 0 && page.NextBefore >= before {
					t.Fatal("pagination did not advance")
				}
				before = page.NextBefore
			}
			if len(seen) != fixture.count || pages < 2 {
				t.Fatalf("results=%d pages=%d", len(seen), pages)
			}
		})
	}
}
