package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"denova/internal/book"
)

const (
	loreClassificationPreviewMaxBytes = 64 * 1024
	loreClassificationBodyMaxBytes    = 2 * 1024
)

type LoreClassificationPreviewRequest struct {
	ItemIDs []string `json:"item_ids,omitempty"`
	Mode    string   `json:"mode,omitempty"`
}

type LoreClassificationPreviewItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	CurrentType       string `json:"current_type"`
	CurrentTypeSource string `json:"current_type_source"`
	SuggestedType     string `json:"suggested_type"`
	Confidence        string `json:"confidence"`
	Reason            string `json:"reason,omitempty"`
	SuggestionSource  string `json:"suggestion_source"`
}

type LoreClassificationPreview struct {
	Revision string                          `json:"revision"`
	Mode     string                          `json:"mode"`
	Items    []LoreClassificationPreviewItem `json:"items"`
	Counts   map[string]int                  `json:"counts"`
	Warning  string                          `json:"warning,omitempty"`
}

type LoreClassificationApplyRequest struct {
	Revision string                `json:"revision"`
	Changes  []book.LoreTypeChange `json:"changes"`
}

func (a *App) PreviewLoreClassification(ctx context.Context, request LoreClassificationPreviewRequest) (LoreClassificationPreview, error) {
	return a.lore().PreviewLoreClassification(ctx, request)
}

func (s *LoreAppService) PreviewLoreClassification(ctx context.Context, request LoreClassificationPreviewRequest) (LoreClassificationPreview, error) {
	state := s.bookState()
	if state == nil {
		return LoreClassificationPreview{}, ErrNoWorkspace
	}
	store := book.NewLoreStore(state.Workspace())
	items, err := store.ListAll()
	if err != nil {
		return LoreClassificationPreview{}, err
	}
	revision, err := store.AllRevision()
	if err != nil {
		return LoreClassificationPreview{}, err
	}
	selected := selectLoreClassificationCandidates(items, request.ItemIDs)
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode != book.LoreClassificationModeSemantic {
		mode = book.LoreClassificationModeHeuristic
	}
	preview := LoreClassificationPreview{Revision: revision, Mode: mode, Items: make([]LoreClassificationPreviewItem, 0, len(selected)), Counts: map[string]int{}}
	semanticInputs := make([]book.LoreClassificationInput, 0, len(selected))
	previewIndexByID := map[string]int{}
	usedBytes := 2
	semanticEligible := 0
	for _, item := range selected {
		input := loreClassificationInputFromItem(item)
		suggestion := book.ClassifyLoreItemHeuristic(input)
		preview.Items = append(preview.Items, LoreClassificationPreviewItem{
			ID: item.ID, Name: item.Name, CurrentType: item.Type, CurrentTypeSource: item.TypeSource,
			SuggestedType: suggestion.Type, Confidence: suggestion.Confidence, Reason: suggestion.Reason, SuggestionSource: book.LoreTypeSourceHeuristic,
		})
		previewIndexByID[item.ID] = len(preview.Items) - 1
		if mode != book.LoreClassificationModeSemantic || suggestion.Confidence == book.LoreClassificationConfidenceHigh {
			continue
		}
		semanticEligible++
		encoded, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			return LoreClassificationPreview{}, marshalErr
		}
		if usedBytes+len(encoded)+1 > loreClassificationPreviewMaxBytes {
			continue
		}
		usedBytes += len(encoded) + 1
		semanticInputs = append(semanticInputs, input)
	}
	if mode == book.LoreClassificationModeSemantic && len(semanticInputs) > 0 {
		suggestions, classifyErr := s.app.ClassifyLoreItems(ctx, semanticInputs)
		if classifyErr != nil {
			preview.Warning = "语义分类暂时不可用，当前展示本地名称分析结果：" + classifyErr.Error()
		} else {
			for _, suggestion := range suggestions {
				index, ok := previewIndexByID[strings.TrimSpace(suggestion.ID)]
				if !ok {
					continue
				}
				preview.Items[index].SuggestedType = suggestion.Type
				preview.Items[index].Confidence = suggestion.Confidence
				preview.Items[index].Reason = suggestion.Reason
				preview.Items[index].SuggestionSource = book.LoreTypeSourceSemantic
			}
		}
	}
	if mode == book.LoreClassificationModeSemantic {
		if omitted := semanticEligible - len(semanticInputs); omitted > 0 && preview.Warning == "" {
			preview.Warning = fmt.Sprintf("分类输入达到 64 KiB 上限；%d 条保留本地分析结果", omitted)
		}
	}
	for _, item := range preview.Items {
		preview.Counts[item.SuggestedType]++
	}
	return preview, nil
}

func (a *App) ApplyLoreClassification(request LoreClassificationApplyRequest) (book.LoreTypeApplyResult, error) {
	return a.lore().ApplyLoreClassification(request)
}

func (s *LoreAppService) ApplyLoreClassification(request LoreClassificationApplyRequest) (book.LoreTypeApplyResult, error) {
	state := s.bookState()
	if state == nil {
		return book.LoreTypeApplyResult{}, ErrNoWorkspace
	}
	return book.NewLoreStore(state.Workspace()).ApplyTypeChanges(request.Revision, request.Changes)
}

func selectLoreClassificationCandidates(items []book.LoreItem, requestedIDs []string) []book.LoreItem {
	wanted := map[string]bool{}
	for _, id := range requestedIDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	explicit := len(wanted) > 0
	result := make([]book.LoreItem, 0, len(items))
	for _, item := range items {
		if explicit {
			if wanted[item.ID] {
				result = append(result, item)
			}
			continue
		}
		if item.Provenance == nil || item.Provenance.Kind != "tavern_worldbook_entry" || item.TypeSource == book.LoreTypeSourceManual {
			continue
		}
		if item.Type == "world" || item.Type == "other" {
			result = append(result, item)
		}
	}
	return result
}

func loreClassificationInputFromItem(item book.LoreItem) book.LoreClassificationInput {
	content, _ := trimStringToUTF8Bytes(item.Content, loreClassificationBodyMaxBytes)
	return book.LoreClassificationInput{
		ID: item.ID, Name: item.Name, Tags: append([]string(nil), item.Tags...), Keywords: append([]string(nil), item.Keywords...),
		BriefDescription: item.BriefDescription, Content: content, CurrentType: item.Type,
	}
}
