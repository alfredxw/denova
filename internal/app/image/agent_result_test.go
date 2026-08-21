package imageapp

import "testing"

func TestEventInteractiveImageFallsBackToBoundedToolContent(t *testing.T) {
	image := eventInteractiveImage(map[string]any{
		"name":    "generate_image",
		"content": `{"schema":"interactive_image.v1","story_id":"story","branch_id":"main","turn_id":"turn","image_path":"assets/interactive/images/turn/image.png","meta_path":"assets/interactive/images/turn/meta.json","alt_text":"scene"}`,
	})
	if image == nil {
		t.Fatal("eventInteractiveImage() = nil, want the interactive image preserved in bounded ToolResult content")
	}
	if image.StoryID != "story" || image.BranchID != "main" || image.TurnID != "turn" || image.ImagePath != "assets/interactive/images/turn/image.png" {
		t.Fatalf("eventInteractiveImage() = %#v", image)
	}
}
