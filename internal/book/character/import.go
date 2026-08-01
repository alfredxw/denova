package character

import (
	"denova/internal/book/lore"
	"fmt"
)

func (s *Service) ImportTavernCard(filename string, data []byte, opts ...ImportOptions) (ImportResult, error) {
	card, err := parseTavernCharacterCard(filename, data)
	if err != nil {
		return ImportResult{}, err
	}
	options := mergeCharacterCardImportOptions(opts...)
	loreStore := lore.NewStore(s.workspace)
	existingItems, err := loreStore.ListAll()
	if err != nil {
		return ImportResult{}, err
	}
	coverPath := ""
	if card.IsPNG {
		coverPath = tavernCardCoverPath
	}
	ops, importStats := buildTavernCardLoreOperations(card, filename, coverPath, options.UserCharacterName, lore.NewNameAllocator(existingItems))
	importStats.ClassificationMode = lore.NormalizeClassificationMode(options.ClassificationMode)
	if importStats.ClassificationMode == lore.ClassificationModeSemantic && options.ClassifyLore != nil {
		if err := applySemanticTavernLoreClassification(ops, &importStats, options.ClassifyLore); err != nil {
			importStats.Warnings = append(importStats.Warnings, "语义资料分类失败，已保留名称优先的本地分类结果："+err.Error())
		}
	}
	// Semantic classification can be slow and performs no local writes. Take
	// the rollback snapshot only after it finishes so a later rollback cannot
	// overwrite user edits made while the model request was running.
	snapshots, err := snapshotCharacterCardImportFiles(s.workspace)
	if err != nil {
		return ImportResult{}, err
	}
	rollback := func(cause error) (ImportResult, error) {
		if rollbackErr := restoreCharacterCardImportFiles(snapshots); rollbackErr != nil {
			return ImportResult{}, fmt.Errorf("%w；回滚导入文件失败: %v", cause, rollbackErr)
		}
		return ImportResult{}, cause
	}
	coverPath, err = s.importTavernCardCover(card, data)
	if err != nil {
		return rollback(err)
	}
	openingCount, err := s.importTavernCardOpeningPresets(card)
	if err != nil {
		return rollback(err)
	}
	applyResult, err := loreStore.ApplyOperations(fmt.Sprintf("导入酒馆角色卡「%s」", card.Name), ops)
	if err != nil {
		return rollback(err)
	}

	itemIDs := make([]string, 0, len(applyResult.Created))
	for _, item := range applyResult.Created {
		itemIDs = append(itemIDs, item.ID)
	}
	result := ImportResult{
		Name:                 card.Name,
		TargetPath:           loreItemsRelPath(s.workspace),
		EntryCount:           characterBookEntryCount(card.CharacterBook),
		ItemCount:            len(itemIDs),
		ItemIDs:              itemIDs,
		CoverPath:            coverPath,
		OpeningPresetPath:    openingPresetPath(openingCount),
		OpeningPresetCount:   openingCount,
		UserPlaceholderFound: card.HasUserPlaceholder,
		UserCharacterName:    tavernUserCharacterName(card, options.UserCharacterName),
		Compatibility:        tavernCardCompatibility(card),
		Message:              fmt.Sprintf("已导入酒馆角色卡「%s」到互动资料库", card.Name),
		ResidentLoreBytes:    importStats.ResidentLoreBytes,
		ClassificationMode:   importStats.ClassificationMode,
		ClassificationCounts: cloneLoreTypeCounts(importStats.ClassificationCounts),
		UncertainTypeCount:   importStats.UncertainTypeCount,
	}
	result.Compatibility.Warnings = append(result.Compatibility.Warnings, importStats.Warnings...)
	return result, nil
}

func PreviewTavernCard(filename string, data []byte) (Preview, error) {
	card, err := parseTavernCharacterCard(filename, data)
	if err != nil {
		return Preview{}, err
	}
	_, stats := buildTavernCardLoreOperations(card, filename, "", "玩家角色", lore.NewNameAllocator(nil))
	return Preview{
		Name:                  card.Name,
		EntryCount:            characterBookEntryCount(card.CharacterBook),
		Tags:                  tavernCardTags(card.Tags...),
		OpeningPresetCount:    tavernCardOpeningPresetCount(card),
		UserPlaceholderFound:  card.HasUserPlaceholder,
		WillImportCover:       card.IsPNG,
		Compatibility:         tavernCardCompatibility(card),
		EnabledEntryCount:     stats.EnabledEntryCount,
		DisabledEntryCount:    stats.DisabledEntryCount,
		ResidentEntryCount:    stats.ResidentEntryCount,
		ResidentEntryBytes:    stats.ResidentEntryBytes,
		ResidentLoreBytes:     stats.ResidentLoreBytes,
		AutoEntryCount:        stats.AutoEntryCount,
		RemovedRuntimeCount:   stats.RemovedRuntimeCount,
		SanitizedMixedCount:   stats.SanitizedMixedCount,
		OpeningTruncatedCount: tavernCardOpeningTruncatedCount(card),
		ResidentLoreWarning:   stats.ResidentLoreBytes > lore.ResidentLoreWarningBytes,
		ResidentLoreWarningKB: bytesToKB(lore.ResidentLoreWarningBytes),
		ClassificationMode:    lore.ClassificationModeHeuristic,
		ClassificationCounts:  cloneLoreTypeCounts(stats.ClassificationCounts),
		UncertainTypeCount:    stats.UncertainTypeCount,
	}, nil
}
