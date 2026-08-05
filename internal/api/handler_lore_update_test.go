package api

import (
	"net/http"
	"net/url"
	"testing"

	"denova/internal/book/lore"
)

func TestLoreItemUpdateUsesFullPutContract(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book/lore/items/"
	created, err := application.Lore().CreateItem(lore.ItemInput{
		ID:               "hero",
		Type:             "character",
		Name:             "林川",
		Importance:       "major",
		Tags:             []string{"主角", "调查员"},
		BriefDescription: "谨慎的调查员",
		Keywords:         []string{"林川", "调查"},
		LoadMode:         lore.LoadModeResident,
		Content:          "旧正文",
	})
	if err != nil {
		t.Fatal(err)
	}

	update := map[string]any{
		"id":                created.ID,
		"enabled":           created.Enabled,
		"type":              created.Type,
		"type_source":       created.TypeSource,
		"name":              "林川（成年）",
		"importance":        created.Importance,
		"tags":              created.Tags,
		"brief_description": created.BriefDescription,
		"keywords":          created.Keywords,
		"load_mode":         created.LoadMode,
		"content":           "新正文",
		"base_revision":     created.UpdatedAt,
	}
	response := performJSONRequest(t, server, http.MethodPut, base+created.ID, update)
	if response.Code != http.StatusOK {
		t.Fatalf("PUT update status=%d body=%s", response.Code, response.Body.String())
	}
	var updateEnvelope struct {
		ProjectID string    `json:"project_id"`
		Item      lore.Item `json:"item"`
	}
	decodeResponse(t, response.Body.Bytes(), &updateEnvelope)
	if updateEnvelope.ProjectID != projectID {
		t.Fatalf("PUT response lost Project scope: %#v", updateEnvelope)
	}
	updated := updateEnvelope.Item
	if updated.Name != "林川（成年）" || updated.Content != "新正文" {
		t.Fatalf("unexpected updated lore item: %#v", updated)
	}
	if updated.Type != created.Type || updated.Importance != created.Importance || updated.LoadMode != created.LoadMode {
		t.Fatalf("full update lost canonical fields: %#v", updated)
	}
	if len(updated.Tags) != len(created.Tags) || len(updated.Keywords) != len(created.Keywords) {
		t.Fatalf("full update lost list fields: %#v", updated)
	}
	partial := performJSONRequest(t, server, http.MethodPut, base+created.ID, map[string]any{
		"name":          "不完整更新",
		"base_revision": updated.UpdatedAt,
	})
	if partial.Code != http.StatusBadRequest {
		t.Fatalf("partial PUT must be rejected before it can clear omitted fields: status=%d body=%s", partial.Code, partial.Body.String())
	}
	items, err := application.Lore().Items()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != updated.Name || items[0].Content != updated.Content {
		t.Fatalf("rejected partial PUT mutated canonical Lore: %#v", items)
	}

	patch := performJSONRequest(t, server, http.MethodPatch, base+created.ID, update)
	if patch.Code != http.StatusNotFound {
		t.Fatalf("legacy PATCH must not remain a second update contract: status=%d body=%s", patch.Code, patch.Body.String())
	}
}
