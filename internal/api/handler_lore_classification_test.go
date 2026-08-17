package api

import (
	"net/http"
	"net/url"
	"testing"

	loreapp "denova/internal/app/lore"
	"denova/internal/book/lore"
)

func TestLoreClassificationPreviewAndApplyAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	projectID := application.ProjectID()
	base := "/api/projects/" + url.PathEscape(projectID) + "/book/lore"
	item, err := application.ProjectBook().CreateLoreItem(projectID, lore.ItemInput{
		ID: "shen", Type: "other", TypeSource: lore.TypeSourceHeuristic, Name: "人物详情：沈凝", Content: "沈凝负责见证公开比试。",
		Provenance: &lore.Provenance{Kind: "tavern_worldbook_entry", SourceName: "card.json", SourceRecordID: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyItem, err := application.ProjectBook().CreateLoreItem(projectID, lore.ItemInput{
		ID: "legacy", Type: "world", TypeSource: lore.TypeSourceManual, Name: "人物详情：旧资料", Content: "旧资料也应当可以重新分类。",
	})
	if err != nil {
		t.Fatal(err)
	}
	previewResp := performJSONRequest(t, server, http.MethodPost, base+"/classification/preview", map[string]any{"mode": "heuristic"})
	if previewResp.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResp.Code, previewResp.Body.String())
	}
	var preview loreapp.ClassificationPreview
	decodeResponse(t, previewResp.Body.Bytes(), &preview)
	previewByID := make(map[string]loreapp.ClassificationPreviewItem, len(preview.Items))
	for _, previewItem := range preview.Items {
		previewByID[previewItem.ID] = previewItem
	}
	if preview.Revision == "" || len(preview.Items) != 2 || previewByID[item.ID].SuggestedType != "character" || previewByID[legacyItem.ID].SuggestedType != "character" {
		t.Fatalf("unexpected classification preview: %#v", preview)
	}

	applyResp := performJSONRequest(t, server, http.MethodPost, base+"/classification/apply", loreapp.ClassificationApplyRequest{
		Revision: preview.Revision,
		Changes:  []lore.TypeChange{{ID: item.ID, Type: preview.Items[0].SuggestedType}},
	})
	if applyResp.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", applyResp.Code, applyResp.Body.String())
	}
	var result lore.TypeApplyResult
	decodeResponse(t, applyResp.Body.Bytes(), &result)
	if len(result.Updated) != 1 || result.Updated[0].Type != "character" || result.Updated[0].TypeSource != lore.TypeSourceManual {
		t.Fatalf("confirmed classification should persist as manual metadata: %#v", result)
	}

	staleResp := performJSONRequest(t, server, http.MethodPost, base+"/classification/apply", loreapp.ClassificationApplyRequest{
		Revision: preview.Revision,
		Changes:  []lore.TypeChange{{ID: item.ID, Type: "world"}},
	})
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale preview should conflict: status=%d body=%s", staleResp.Code, staleResp.Body.String())
	}
}
