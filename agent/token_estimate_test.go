package agent

import "testing"

func TestEstimateMessageTokensIncludesModelVisibleAttachments(t *testing.T) {
	plain := EstimateMessageTokens(&Message{Role: User, Content: "inspect"})
	withDocument := EstimateMessageTokens(UserMessageWithAttachments("inspect", []Attachment{{
		ID: "att-doc", Name: "notes.md", MediaType: "text/markdown", Size: 128, Path: "/inputs/notes.md", SHA256: "digest",
	}}))
	withImage := EstimateMessageTokens(UserMessageWithAttachments("inspect", []Attachment{{
		ID: "att-image", Name: "map.png", MediaType: "image/png", Size: 3_000, Path: "/inputs/map.png", SHA256: "digest",
	}}))

	if withDocument <= plain {
		t.Fatalf("document attachment estimate = %d, plain = %d", withDocument, plain)
	}
	if withImage < withDocument+1_000 {
		t.Fatalf("native image payload was not charged: image = %d document = %d", withImage, withDocument)
	}
}
