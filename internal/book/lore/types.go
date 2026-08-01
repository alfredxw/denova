package lore

import (
	"encoding/json"
	"errors"
	"sync"
)

const loreItemsVersion = 2

const (
	LoadModeResident = "resident"
	LoadModeAuto     = "auto"
	LoadModeManual   = "manual"

	// ResidentLoreWarningBytes is guidance only and never blocks persistence.
	ResidentLoreWarningBytes = 32 * 1024
	// ResidentLoreSafetyMaxBytes bounds model-visible context assembly without
	// limiting what users may import or store in the lore library.
	ResidentLoreSafetyMaxBytes = 1024 * 1024

	IndexDefaultMaxBytes = 64 * 1024
	IndexDefaultLimit    = 10

	IndexMatchAny = "any"
	IndexMatchAll = "all"

	TypeSourceHeuristic = "heuristic"
	TypeSourceSemantic  = "semantic"
	TypeSourceManual    = "manual"
	TypeSourceLegacy    = "legacy"
)

// Item 是用户可编辑的作品资料条目。固定字段只负责索引和展示，正文继续使用 Markdown。
type Item struct {
	ID               string      `json:"id"`
	Enabled          bool        `json:"enabled"`
	Type             string      `json:"type"`
	TypeSource       string      `json:"type_source"`
	Name             string      `json:"name"`
	Importance       string      `json:"importance"`
	Tags             []string    `json:"tags"`
	BriefDescription string      `json:"brief_description"`
	Keywords         []string    `json:"keywords"`
	LoadMode         string      `json:"load_mode"`
	Content          string      `json:"content"`
	CreatedAt        string      `json:"created_at"`
	UpdatedAt        string      `json:"updated_at"`
	Image            *Image      `json:"image,omitempty"`
	Provenance       *Provenance `json:"provenance,omitempty"`
}

type ItemInput struct {
	ID               string      `json:"id"`
	Enabled          *bool       `json:"enabled,omitempty"`
	Type             string      `json:"type"`
	TypeSource       string      `json:"type_source,omitempty"`
	Name             string      `json:"name"`
	Importance       string      `json:"importance"`
	Tags             []string    `json:"tags"`
	BriefDescription string      `json:"brief_description"`
	Keywords         []string    `json:"keywords"`
	LoadMode         string      `json:"load_mode"`
	Content          string      `json:"content"`
	Image            *Image      `json:"image,omitempty"`
	Provenance       *Provenance `json:"provenance,omitempty"`
	BaseRevision     string      `json:"base_revision,omitempty"`
}

// ReadResult preserves the caller's lookup order while reporting entries
// that no longer exist. Batch reads are intentionally tolerant so one stale
// catalog name cannot discard every valid item in the same tool call.
type ReadResult struct {
	Items   []Item
	Missing []string
}

// Provenance records an item's external origin without exposing it in
// model-visible lore markdown. It is intentionally generic so future importers
// can use the same storage boundary.
type Provenance struct {
	Kind           string `json:"kind"`
	SourceName     string `json:"source_name"`
	SourceRecordID string `json:"source_record_id"`
	SourceHash     string `json:"source_hash"`
}

// Image is the current visual asset attached to a lore item.
type Image struct {
	Schema        string `json:"schema"`
	ImagePath     string `json:"image_path"`
	MetaPath      string `json:"meta_path"`
	AltText       string `json:"alt_text,omitempty"`
	ImagePresetID string `json:"image_preset_id,omitempty"`
	ProfileID     string `json:"profile_id"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	Size          string `json:"size,omitempty"`
	Quality       string `json:"quality,omitempty"`
	OutputFormat  string `json:"output_format,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`

	RevisedPrompt string `json:"revised_prompt,omitempty"`
	MIMEType      string `json:"mime_type,omitempty"`
	SizeBytes     int    `json:"size_bytes,omitempty"`
}

type Collection struct {
	Version int    `json:"version"`
	Items   []Item `json:"items"`
}

type Operation struct {
	Op   string    `json:"op"`
	ID   string    `json:"id,omitempty"`
	Item ItemInput `json:"item,omitempty"`
}

type ApplyResult struct {
	Message    string   `json:"message"`
	Items      []Item   `json:"items"`
	Created    []Item   `json:"created"`
	Updated    []Item   `json:"updated"`
	DeletedIDs []string `json:"deleted_ids"`
}

type Store struct {
	workspace  string
	mutationMu *sync.Mutex
}

// NameAllocator claims unique, normalized lore names for transactional imports.
type NameAllocator struct {
	used map[string]bool
}

// IndexOptions controls model-visible lore index rendering. Keywords are
// matched independently; Match selects OR (any) or AND (all) semantics.
type IndexOptions struct {
	Keywords        []string
	Match           string
	Types           []string
	LoadModes       []string
	Limit           int
	Offset          int
	Paginate        bool
	MaxBytes        int
	ExcludeResident bool
	OmitTitle       bool
}

var ErrRevisionConflict = errors.New("资料已被其他操作更新，请重新加载后再保存")

var loreMutationLocks sync.Map

func (item *Item) UnmarshalJSON(data []byte) error {
	type loreItemAlias Item
	raw := struct {
		Enabled *bool `json:"enabled"`
		*loreItemAlias
	}{
		loreItemAlias: (*loreItemAlias)(item),
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	item.Enabled = true
	if raw.Enabled != nil {
		item.Enabled = *raw.Enabled
	}
	return nil
}
