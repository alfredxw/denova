package tools

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"denova/internal/book/lore"
	agent "github.com/alfredxw/denova/agent"
)

func TestWriteLoreItemsKeepsBatchEntitiesSeparateDuringPartialUpdates(t *testing.T) {
	workspace := t.TempDir()
	definitions, err := newLoreTools(workspace, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		info, err := definition.Tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name != "write_lore_items" {
			continue
		}
		run := func(input string, wantIDs []string) {
			t.Helper()
			result, err := definition.Tool.Run(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != agent.ToolResultSuccess {
				t.Fatalf("write failed: %s", result.ModelContent)
			}
			var receipt struct {
				ItemIDs []string `json:"item_ids"`
			}
			if err := json.Unmarshal(result.Details, &receipt); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(receipt.ItemIDs, wantIDs) {
				t.Fatalf("receipt item IDs = %v, want %v", receipt.ItemIDs, wantIDs)
			}
		}
		run(`{"items":[
			{"id":"hero","type":"character","name":"Mira","tags":["pilot"],"content":"Mira pilots the ferry."},
			{"id":"harbor","type":"location","name":"North Harbor","content":"The harbor closes at night."},
			{"id":"guild","type":"faction","name":"River Guild","content":"The guild maintains the ferry."}
		]}`, []string{"hero", "harbor", "guild"})
		store := lore.NewStore(workspace)
		before, err := store.ListAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(before) != 3 {
			t.Fatalf("created %d items, want 3", len(before))
		}
		run(`{"items":[
			{"id":"hero","content":"Mira pilots the ferry and founded the guild."},
			{"id":"guild","content":"The guild maintains the ferry and was founded by Mira."}
		]}`, []string{"hero", "guild"})
		after, err := lore.NewStore(workspace).ListAll()
		if err != nil {
			t.Fatal(err)
		}
		if len(after) != len(before) {
			t.Fatalf("updated collection has %d items, want %d", len(after), len(before))
		}
		want := append([]lore.Item(nil), before...)
		for i := range want {
			switch want[i].ID {
			case "hero":
				want[i].Content = "Mira pilots the ferry and founded the guild."
				want[i].UpdatedAt = after[i].UpdatedAt
			case "guild":
				want[i].Content = "The guild maintains the ferry and was founded by Mira."
				want[i].UpdatedAt = after[i].UpdatedAt
			}
		}
		if !reflect.DeepEqual(after, want) {
			t.Fatalf("batch update changed unrelated data:\n got %#v\nwant %#v", after, want)
		}
		return
	}
	t.Fatal("write_lore_items was not registered")
}
