package toolresult

import (
	"reflect"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
)

func TestLoreReceiptEnrichmentUsesProductCapability(t *testing.T) {
	manifest := Manifest{
		Name: "read_lore_items",
		ToolDescriptor: agent.ToolDescriptor{
			Source: agent.ToolSource("denova.lore"), Capability: config.AgentToolLoreRead,
		},
	}
	result := agent.TextToolResult("## 林照（character）\nID: lore-1\n\n## Winter Court\nID：lore-2")
	receipt := buildRetainedToolReceipt(manifest, result)
	if !reflect.DeepEqual(receipt.SourceIDs, []string{"lore-1", "lore-2"}) ||
		!reflect.DeepEqual(receipt.Names, []string{"林照", "Winter Court"}) {
		t.Fatalf("lore receipt enrichment = %#v", receipt)
	}
	if receipt.Source != manifest.Source {
		t.Fatalf("product source changed: got=%q want=%q", receipt.Source, manifest.Source)
	}
}
