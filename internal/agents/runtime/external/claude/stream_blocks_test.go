package claude

import (
	"fmt"
	"testing"
)

func TestStreamMergesSplitAssistantBlocks(t *testing.T) {
	for _, order := range []string{"deltas-first", "wrappers-first", "wrappers-only"} {
		t.Run(order, func(t *testing.T) {
			var output streamOutput
			host := &testHost{}
			feed := func(frame string) {
				t.Helper()
				if err := output.feed([]byte(frame), host); err != nil {
					t.Fatal(err)
				}
			}
			// CLI assistant wrappers contain newly completed blocks, not the full
			// message. Identical text in distinct blocks must remain distinct.
			for _, message := range []string{"first", "second"} {
				feed(fmt.Sprintf(`{"type":"stream_event","event":{"type":"message_start","message":{"id":%q}}}`, message))
				blocks := []string{`{"type":"thinking","thinking":"Private"}`, `{"type":"text","text":"Hello"}`, `{"type":"tool_use","id":"tool","name":"read","input":{}}`, `{"type":"text","text":"Hello"}`}
				for index, block := range blocks {
					wrapper := fmt.Sprintf(`{"type":"assistant","uuid":"%s-%d","message":{"id":%q,"content":[%s]}}`, message, index, message, block)
					delta := fmt.Sprintf(`{"type":"stream_event","event":{"type":"content_block_delta","index":%d,"delta":{"type":"text_delta","text":"Hel"}}}`, index)
					if order == "deltas-first" && (index == 1 || index == 3) {
						feed(delta)
					}
					feed(wrapper)
					feed(wrapper) // Replayed transport envelope has the same UUID.
					if order == "wrappers-first" && (index == 1 || index == 3) {
						feed(delta)
					}
				}
			}
			if got := output.result().Text; got != "HelloHelloHelloHello" || host.text != got {
				t.Fatalf("canonical=%q displayed=%q", got, host.text)
			}
			wantChunks := 4
			if order == "deltas-first" {
				wantChunks = 8 // Partial prefix followed by the missing suffix.
			}
			if len(host.events) != wantChunks {
				t.Fatalf("got %d chunks, want %d", len(host.events), wantChunks)
			}
		})
	}
}
