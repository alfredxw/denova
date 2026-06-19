package app

import "testing"

func TestParseInteractiveMemoryOutputReadsStateOpsAndMemoryEntry(t *testing.T) {
	result, err := parseInteractiveMemoryOutput(`{
	  "state_ops": [{"op":"set","path":"location","value":"旧宅门口"}],
	  "memory_entry": {
	    "title": "旧宅门口",
	    "summary": "主角抵达旧宅门口。",
	    "content": "旧宅门口有新鲜脚印。",
	    "people": ["主角"],
	    "places": ["旧宅"],
	    "tags": ["线索"],
	    "importance": 4
	  }
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StateOps) != 1 || result.StateOps[0].Path != "location" {
		t.Fatalf("state ops mismatch: %#v", result.StateOps)
	}
	if result.MemoryEntry == nil || result.MemoryEntry.Title != "旧宅门口" || result.MemoryEntry.Importance != 4 {
		t.Fatalf("memory entry mismatch: %#v", result.MemoryEntry)
	}
}

func TestParseInteractiveMemoryOutputAllowsEmptyStateOps(t *testing.T) {
	result, err := parseInteractiveMemoryOutput(`{"state_ops":[],"memory_entry":null}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StateOps) != 0 || result.MemoryEntry != nil {
		t.Fatalf("result mismatch: %#v", result)
	}
}

func TestParseInteractiveMemoryOutputCoercesStoryMemoryPatchValuesToText(t *testing.T) {
	result, err := parseInteractiveMemoryOutput(`{
	  "story_memory_patches": [{
	    "op": "append",
	    "structure_id": "plot_summary",
	    "values": {
	      "sequence": 3,
	      "confirmed": true,
	      "event": "主角进入旧宅。",
	      "tags": ["线索", 2]
	    }
	  }]
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StoryMemoryPatches) != 1 {
		t.Fatalf("patches mismatch: %#v", result.StoryMemoryPatches)
	}
	values := result.StoryMemoryPatches[0].Values
	if values["sequence"] != "3" || values["confirmed"] != "true" || values["tags"] != `["线索",2]` {
		t.Fatalf("values were not coerced to text: %#v", values)
	}
}
