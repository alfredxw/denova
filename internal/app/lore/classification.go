package loreapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	agentcontext "denova/internal/agents/context"
	booklore "denova/internal/book/lore"
)

const (
	classificationPreviewMaxBytes = 256 * 1024
	classificationBodyMaxBytes    = 8 * 1024
)

type ClassificationPreviewRequest struct {
	ItemIDs []string `json:"item_ids,omitempty"`
	Mode    string   `json:"mode,omitempty"`
}

type ClassificationPreviewItem struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	CurrentType       string `json:"current_type"`
	CurrentTypeSource string `json:"current_type_source"`
	SuggestedType     string `json:"suggested_type"`
	Confidence        string `json:"confidence"`
	Reason            string `json:"reason,omitempty"`
	SuggestionSource  string `json:"suggestion_source"`
}

type ClassificationPreview struct {
	Revision string                      `json:"revision"`
	Mode     string                      `json:"mode"`
	Items    []ClassificationPreviewItem `json:"items"`
	Counts   map[string]int              `json:"counts"`
	Warning  string                      `json:"warning,omitempty"`
}

type ClassificationApplyRequest struct {
	Revision string                `json:"revision"`
	Changes  []booklore.TypeChange `json:"changes"`
}

func (service *Service) PreviewClassification(ctx context.Context, expectedWorkspace string, request ClassificationPreviewRequest) (ClassificationPreview, error) {
	if service == nil || service.images == nil || service.host == nil {
		return ClassificationPreview{}, ErrNoWorkspace
	}
	runtime, err := service.images.AcquireRuntime(ctx, expectedWorkspace)
	if err != nil {
		return ClassificationPreview{}, err
	}
	defer runtime.Release()

	store := booklore.NewStore(runtime.Workspace)
	items, err := store.ListAll()
	if err != nil {
		return ClassificationPreview{}, err
	}
	revision, err := store.AllRevision()
	if err != nil {
		return ClassificationPreview{}, err
	}
	selected := selectClassificationCandidates(items, request.ItemIDs)
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode != booklore.ClassificationModeSemantic {
		mode = booklore.ClassificationModeHeuristic
	}
	preview := ClassificationPreview{
		Revision: revision,
		Mode:     mode,
		Items:    make([]ClassificationPreviewItem, 0, len(selected)),
		Counts:   make(map[string]int),
	}
	semanticInputs := make([]booklore.ClassificationInput, 0, len(selected))
	previewIndexByID := make(map[string]int, len(selected))
	usedBytes := 2
	semanticEligible := 0
	for _, item := range selected {
		input := classificationInputFromItem(item)
		suggestion := booklore.ClassifyItemHeuristic(input)
		preview.Items = append(preview.Items, ClassificationPreviewItem{
			ID: item.ID, Name: item.Name, CurrentType: item.Type, CurrentTypeSource: item.TypeSource,
			SuggestedType: suggestion.Type, Confidence: suggestion.Confidence, Reason: suggestion.Reason,
			SuggestionSource: booklore.TypeSourceHeuristic,
		})
		previewIndexByID[item.ID] = len(preview.Items) - 1
		if mode != booklore.ClassificationModeSemantic || suggestion.Confidence == booklore.ClassificationConfidenceHigh {
			continue
		}
		semanticEligible++
		encoded, marshalErr := json.Marshal(input)
		if marshalErr != nil {
			return ClassificationPreview{}, marshalErr
		}
		if usedBytes+len(encoded)+1 > classificationPreviewMaxBytes {
			continue
		}
		usedBytes += len(encoded) + 1
		semanticInputs = append(semanticInputs, input)
	}
	if mode == booklore.ClassificationModeSemantic && len(semanticInputs) > 0 {
		suggestions, classifyErr := service.host.ClassifyLoreItems(runtime.Context(), semanticInputs)
		if classifyErr != nil {
			preview.Warning = "Semantic classification is temporarily unavailable; local name analysis is shown. / 语义分类暂时不可用，当前展示本地名称分析结果：" + classifyErr.Error()
		} else {
			for _, suggestion := range suggestions {
				index, ok := previewIndexByID[strings.TrimSpace(suggestion.ID)]
				if !ok {
					continue
				}
				preview.Items[index].SuggestedType = suggestion.Type
				preview.Items[index].Confidence = suggestion.Confidence
				preview.Items[index].Reason = suggestion.Reason
				preview.Items[index].SuggestionSource = booklore.TypeSourceSemantic
			}
		}
	}
	if mode == booklore.ClassificationModeSemantic {
		if omitted := semanticEligible - len(semanticInputs); omitted > 0 && preview.Warning == "" {
			preview.Warning = fmt.Sprintf(
				"Classification input reached the %d KiB limit; %d items keep local analysis results. / 分类输入达到 %d KiB 上限；%d 条保留本地分析结果",
				classificationPreviewMaxBytes/1024, omitted,
				classificationPreviewMaxBytes/1024, omitted,
			)
		}
	}
	for _, item := range preview.Items {
		preview.Counts[item.SuggestedType]++
	}
	return preview, nil
}

func (service *Service) PreviewClassificationForWorkspace(ctx context.Context, expectedWorkspace string, request ClassificationPreviewRequest) (ClassificationPreview, error) {
	if service == nil || service.host == nil {
		return ClassificationPreview{}, ErrNoWorkspace
	}
	if _, err := service.host.ValidateLoreWorkspace(expectedWorkspace); err != nil {
		return ClassificationPreview{}, err
	}
	return service.PreviewClassification(ctx, expectedWorkspace, request)
}

func (service *Service) ApplyClassification(expectedWorkspace string, request ClassificationApplyRequest) (booklore.TypeApplyResult, error) {
	var result booklore.TypeApplyResult
	_, err := service.withStore(expectedWorkspace, func(store *booklore.Store) error {
		var applyErr error
		result, applyErr = store.ApplyTypeChanges(request.Revision, request.Changes)
		return applyErr
	})
	return result, err
}

func selectClassificationCandidates(items []booklore.Item, requestedIDs []string) []booklore.Item {
	wanted := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = struct{}{}
		}
	}
	explicit := len(wanted) > 0
	result := make([]booklore.Item, 0, len(items))
	for _, item := range items {
		if explicit {
			if _, selected := wanted[item.ID]; selected {
				result = append(result, item)
			}
			continue
		}
		// Preview is read-only and every suggested change still requires explicit
		// confirmation, so all source formats remain visible for review.
		result = append(result, item)
	}
	return result
}

func classificationInputFromItem(item booklore.Item) booklore.ClassificationInput {
	content, _ := agentcontext.TrimUTF8Bytes(item.Content, classificationBodyMaxBytes)
	return booklore.ClassificationInput{
		ID: item.ID, Name: item.Name, Tags: append([]string(nil), item.Tags...), Keywords: append([]string(nil), item.Keywords...),
		BriefDescription: item.BriefDescription, Content: content, CurrentType: item.Type,
	}
}
