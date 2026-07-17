package api

import (
	"net/http"
	"testing"

	runtimeapp "denova/internal/app"
	"denova/internal/book"
)

func TestLoreClassificationPreviewAndApplyAPI(t *testing.T) {
	application := newTestApplication(t)
	server := NewServer(application, "0")
	item, err := application.CreateLoreItem(book.LoreItemInput{
		ID: "shen", Type: "other", TypeSource: book.LoreTypeSourceHeuristic, Name: "人物详情：沈凝", Content: "沈凝负责见证公开比试。",
		Provenance: &book.LoreProvenance{Kind: "tavern_worldbook_entry", SourceName: "card.json", SourceRecordID: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	previewResp := performJSONRequest(t, server, http.MethodPost, "/api/lore/classification/preview", map[string]any{"mode": "heuristic"})
	if previewResp.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewResp.Code, previewResp.Body.String())
	}
	var preview runtimeapp.LoreClassificationPreview
	decodeResponse(t, previewResp.Body.Bytes(), &preview)
	if preview.Revision == "" || len(preview.Items) != 1 || preview.Items[0].ID != item.ID || preview.Items[0].SuggestedType != "character" {
		t.Fatalf("unexpected classification preview: %#v", preview)
	}

	applyResp := performJSONRequest(t, server, http.MethodPost, "/api/lore/classification/apply", runtimeapp.LoreClassificationApplyRequest{
		Revision: preview.Revision,
		Changes:  []book.LoreTypeChange{{ID: item.ID, Type: preview.Items[0].SuggestedType}},
	})
	if applyResp.Code != http.StatusOK {
		t.Fatalf("apply status=%d body=%s", applyResp.Code, applyResp.Body.String())
	}
	var result book.LoreTypeApplyResult
	decodeResponse(t, applyResp.Body.Bytes(), &result)
	if len(result.Updated) != 1 || result.Updated[0].Type != "character" || result.Updated[0].TypeSource != book.LoreTypeSourceManual {
		t.Fatalf("confirmed classification should persist as manual metadata: %#v", result)
	}

	staleResp := performJSONRequest(t, server, http.MethodPost, "/api/lore/classification/apply", runtimeapp.LoreClassificationApplyRequest{
		Revision: preview.Revision,
		Changes:  []book.LoreTypeChange{{ID: item.ID, Type: "world"}},
	})
	if staleResp.Code != http.StatusConflict {
		t.Fatalf("stale preview should conflict: status=%d body=%s", staleResp.Code, staleResp.Body.String())
	}
}
