package character

import (
	"denova/internal/book"
	"denova/internal/book/lore"
)

const tavernCardCoverPath = "assets/image/cover.png"
const interactiveOpeningPresetPath = "setting/interactive-openings.json"

var pngSignature = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// ImportResult 描述酒馆角色卡导入结果。
type ImportResult struct {
	Name                 string              `json:"name"`
	TargetPath           string              `json:"target_path"`
	EntryCount           int                 `json:"entry_count"`
	ItemCount            int                 `json:"item_count"`
	ItemIDs              []string            `json:"item_ids"`
	CoverPath            string              `json:"cover_path,omitempty"`
	OpeningPresetPath    string              `json:"opening_preset_path,omitempty"`
	OpeningPresetCount   int                 `json:"opening_preset_count"`
	UserPlaceholderFound bool                `json:"user_placeholder_found"`
	UserCharacterName    string              `json:"user_character_name,omitempty"`
	Workspace            string              `json:"workspace,omitempty"`
	BookMeta             *book.BookMeta      `json:"book_meta,omitempty"`
	Compatibility        CompatibilityReport `json:"compatibility"`
	Message              string              `json:"message"`
	ResidentLoreBytes    int                 `json:"resident_lore_bytes"`
	ClassificationMode   string              `json:"classification_mode"`
	ClassificationCounts map[string]int      `json:"classification_counts"`
	UncertainTypeCount   int                 `json:"uncertain_type_count"`
}

// Preview 描述酒馆角色卡预览信息，解析但不写入 workspace。
type Preview struct {
	Name                  string              `json:"name"`
	EntryCount            int                 `json:"entry_count"`
	Tags                  []string            `json:"tags"`
	OpeningPresetCount    int                 `json:"opening_preset_count"`
	UserPlaceholderFound  bool                `json:"user_placeholder_found"`
	WillImportCover       bool                `json:"will_import_cover"`
	Compatibility         CompatibilityReport `json:"compatibility"`
	EnabledEntryCount     int                 `json:"enabled_entry_count"`
	DisabledEntryCount    int                 `json:"disabled_entry_count"`
	ResidentEntryCount    int                 `json:"resident_entry_count"`
	ResidentEntryBytes    int                 `json:"resident_entry_bytes"`
	ResidentLoreBytes     int                 `json:"resident_lore_bytes"`
	AutoEntryCount        int                 `json:"auto_entry_count"`
	RemovedRuntimeCount   int                 `json:"removed_runtime_entry_count"`
	SanitizedMixedCount   int                 `json:"sanitized_mixed_entry_count"`
	OpeningTruncatedCount int                 `json:"opening_truncated_count"`
	ResidentLoreWarning   bool                `json:"resident_lore_warning"`
	ResidentLoreWarningKB int                 `json:"resident_lore_warning_threshold_kb"`
	ClassificationMode    string              `json:"classification_mode"`
	ClassificationCounts  map[string]int      `json:"classification_counts"`
	UncertainTypeCount    int                 `json:"uncertain_type_count"`
}

// CompatibilityReport reports Denova capabilities rather than
// exposing Tavern's runtime field vocabulary to users.
type CompatibilityReport struct {
	Capabilities        []string `json:"capabilities"`
	SanitizedRuntime    []string `json:"sanitized_runtime"`
	DiscardedExtensions []string `json:"discarded_extensions"`
	Warnings            []string `json:"warnings"`
	IgnoredLoadingRules bool     `json:"ignored_loading_rules"`
}

type ImportOptions struct {
	UserCharacterName  string
	ClassificationMode string
	ClassifyLore       lore.SemanticClassifier
}

type tavernCard struct {
	Spec                    string               `json:"spec"`
	SpecVersion             string               `json:"spec_version"`
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	CreatorComment          string               `json:"creatorcomment"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Avatar                  string               `json:"avatar"`
	Talkativeness           any                  `json:"talkativeness"`
	Fav                     any                  `json:"fav"`
	CreateDate              any                  `json:"create_date"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
	Data                    *tavernCardData      `json:"data"`
}

type tavernCardData struct {
	Name                    string               `json:"name"`
	Description             string               `json:"description"`
	Personality             string               `json:"personality"`
	Scenario                string               `json:"scenario"`
	FirstMes                string               `json:"first_mes"`
	MesExample              string               `json:"mes_example"`
	CreatorNotes            string               `json:"creator_notes"`
	SystemPrompt            string               `json:"system_prompt"`
	PostHistoryInstructions string               `json:"post_history_instructions"`
	Creator                 string               `json:"creator"`
	CharacterVersion        string               `json:"character_version"`
	Extensions              map[string]any       `json:"extensions"`
	Tags                    []string             `json:"tags"`
	AlternateGreetings      []string             `json:"alternate_greetings"`
	CharacterBook           *tavernCharacterBook `json:"character_book"`
}

type tavernCharacterBook struct {
	Name    string            `json:"name"`
	Entries []tavernBookEntry `json:"entries"`
}

type tavernBookEntry struct {
	ID                  int      `json:"id"`
	Keys                []string `json:"keys"`
	SecondaryKeys       []string `json:"secondary_keys"`
	Comment             string   `json:"comment"`
	Content             string   `json:"content"`
	Constant            bool     `json:"constant"`
	Selective           bool     `json:"selective"`
	Enabled             *bool    `json:"enabled"`
	Position            any      `json:"position"`
	InsertionOrder      int      `json:"insertion_order"`
	SelectiveLogic      any      `json:"selectiveLogic"`
	Probability         any      `json:"probability"`
	UseProbability      bool     `json:"useProbability"`
	Group               string   `json:"group"`
	Depth               any      `json:"depth"`
	Role                any      `json:"role"`
	PreventRecursion    bool     `json:"preventRecursion"`
	DelayUntilRecursion bool     `json:"delayUntilRecursion"`
	Sticky              any      `json:"sticky"`
	Cooldown            any      `json:"cooldown"`
	Vectorized          any      `json:"vectorized"`
}

type normalizedTavernCard struct {
	Spec                    string
	SpecVersion             string
	Name                    string
	Description             string
	Personality             string
	Scenario                string
	FirstMes                string
	MesExample              string
	CreatorNotes            string
	CreatorComment          string
	SystemPrompt            string
	PostHistoryInstructions string
	Creator                 string
	CharacterVersion        string
	Avatar                  string
	Talkativeness           any
	Fav                     any
	CreateDate              any
	Extensions              map[string]any
	Tags                    []string
	AlternateGreetings      []string
	CharacterBook           *tavernCharacterBook
	IsPNG                   bool
	HasUserPlaceholder      bool
	Warnings                []string
}

type pngTextChunk struct {
	Keyword string
	Text    string
}
