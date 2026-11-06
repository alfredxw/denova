package configresource

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"denova/config"
	"denova/internal/interactive/teller"
)

func TestConfigReadPaginatesOversizedItemsWithoutLosingContent(t *testing.T) {
	cfg := &config.Config{NovaDir: t.TempDir(), Workspace: t.TempDir()}
	library := teller.NewLibrary(cfg.NovaDir)
	var created []teller.Definition
	for _, id := range []string{"a-before", "b-large", "c-after"} {
		content := "Keep this rule."
		if id == "b-large" {
			content = strings.Repeat("叙述🌌\"\\\n", 12000)
		}
		item, err := library.Create(teller.Definition{
			ID: id, Name: id,
			Slots: []teller.PromptSlot{{ID: "rule", Target: "system", Enabled: true, Content: content}},
		})
		if err != nil {
			t.Fatal(err)
		}
		created = append(created, item)
	}
	catalog, err := library.List()
	if err != nil {
		t.Fatal(err)
	}
	read := configManagerDefinitionByName(t, cfg, "config_read")
	for _, test := range []struct {
		name    string
		request configReadInput
		want    []teller.Definition
		missing []string
	}{
		{name: "catalog", request: configReadInput{Operation: ReadList}, want: catalog},
		{name: "single large item", request: configReadInput{Operation: ReadGet, IDs: []string{"b-large"}}, want: created[1:2]},
		{name: "large last item", request: configReadInput{Operation: ReadGet, IDs: []string{"a-before", "b-large"}}, want: created[:2]},
		{
			name: "mixed exact read", request: configReadInput{Operation: ReadGet, IDs: []string{"missing-before", "a-before", "b-large", "c-after", "missing-after"}},
			want: created, missing: []string{"missing-before", "missing-after"},
		},
		{
			name: "explicit limit", request: configReadInput{Operation: ReadGet, IDs: []string{"b-large", "c-after", "missing-after"}, Limit: 1},
			want: created[1:], missing: []string{"missing-after"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := test.request
			request.Resource = "narrative_style"
			var items []teller.Definition
			var missing []string
			seen := map[string]bool{}
			for {
				output, err := read.Tool.Run(t.Context(), mustJSON(t, request))
				if err != nil {
					t.Fatal(err)
				}
				var page struct {
					Items      []teller.Definition `json:"items"`
					MissingIDs []string            `json:"missing_ids"`
					Failures   []ReadFailure       `json:"failures"`
					Processed  int                 `json:"processed"`
					Returned   int                 `json:"returned"`
					Truncated  bool                `json:"truncated"`
					NextCursor string              `json:"next_cursor"`
				}
				if err := json.Unmarshal([]byte(output.ModelContent), &page); err != nil {
					t.Fatal(err)
				}
				count := len(page.Items) + len(page.MissingIDs) + len(page.Failures)
				if count == 0 || len(page.Failures) != 0 || (request.Limit > 0 && count > request.Limit) {
					t.Fatalf("invalid page: items=%d missing=%v failures=%v", len(page.Items), page.MissingIDs, page.Failures)
				}
				if (request.Operation == ReadGet && page.Processed != count) || (request.Operation == ReadList && page.Returned != count) {
					t.Fatalf("incorrect page count: processed=%d returned=%d want=%d", page.Processed, page.Returned, count)
				}
				if len(output.ModelContent) > defaultToolResultMaxBytes && (count != 1 || len(page.Items) != 1 || page.Items[0].ID != "b-large") {
					t.Fatal("oversized item did not occupy its own page")
				}
				items = append(items, page.Items...)
				missing = append(missing, page.MissingIDs...)
				if !page.Truncated {
					if page.NextCursor != "" {
						t.Fatal("completed read still has a cursor")
					}
					break
				}
				if page.NextCursor == "" || seen[page.NextCursor] {
					t.Fatal("pagination did not advance")
				}
				seen[page.NextCursor] = true
				request.Cursor = page.NextCursor
			}
			if mustJSON(t, items) != mustJSON(t, test.want) || !reflect.DeepEqual(missing, test.missing) {
				t.Fatalf("pagination changed content or order: items=%d/%d missing=%v/%v", len(items), len(test.want), missing, test.missing)
			}
		})
	}
}
