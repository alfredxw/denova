package interactive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"denova/internal/revisionfile"
	"denova/internal/revisionjson"
)

const (
	gamePlanningTemplateVersion            = 1
	DefaultGamePlanningTemplateID          = "default"
	MaxGamePlanningTemplateSections        = 64
	maxGamePlanningSectionTitleBytes       = 256
	maxGamePlanningSectionDescriptionBytes = 16 * 1024
)

var ErrGamePlanningTemplateRevisionConflict = errors.New("planning template changed elsewhere; reload before saving")

// GamePlanningSection is one ordered planning concern. The ID is stable UI
// identity; only Title and Description are projected into model-visible Markdown.
type GamePlanningSection struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// GamePlanningTemplate is a reusable outline for a story's current plan. It
// deliberately owns no narrative, rule, state, image, or event module choices;
// those selections belong to each story.
type GamePlanningTemplate struct {
	Version     int                   `json:"version"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Sections    []GamePlanningSection `json:"sections"`
	Path        string                `json:"path,omitempty"`
	Custom      bool                  `json:"custom"`
	Invalid     bool                  `json:"invalid,omitempty"`
	Error       string                `json:"error,omitempty"`
	CreatedAt   string                `json:"created_at,omitempty"`
	UpdatedAt   string                `json:"updated_at,omitempty"`
	Revision    string                `json:"revision,omitempty"`
}

type GamePlanningTemplateLibrary struct {
	novaDir string
}

func NewGamePlanningTemplateLibrary(novaDir string) *GamePlanningTemplateLibrary {
	return &GamePlanningTemplateLibrary{novaDir: strings.TrimSpace(novaDir)}
}

func (l *GamePlanningTemplateLibrary) List() ([]GamePlanningTemplate, error) {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return nil, err
	}
	items := builtinGamePlanningTemplates()
	files, err := filepath.Glob(filepath.Join(l.dir(), "*.json"))
	if err != nil {
		return nil, err
	}
	custom := make([]GamePlanningTemplate, 0, len(files))
	for _, path := range files {
		item, readErr := readGamePlanningTemplate(path)
		if readErr != nil {
			custom = append(custom, GamePlanningTemplate{
				ID: strings.TrimSuffix(filepath.Base(path), ".json"), Path: path,
				Custom: true, Invalid: true, Error: readErr.Error(),
			})
			continue
		}
		item.Custom, item.Path = true, path
		custom = append(custom, item)
	}
	sort.Slice(custom, func(i, j int) bool { return custom[i].ID < custom[j].ID })
	return append(items, custom...), nil
}

func (l *GamePlanningTemplateLibrary) Get(id string) (GamePlanningTemplate, error) {
	id = NormalizeGamePlanningTemplateID(id)
	if id == "" {
		id = DefaultGamePlanningTemplateID
	}
	if builtin, ok := builtinGamePlanningTemplateByID(id); ok {
		return builtin, nil
	}
	if err := validateGamePlanningTemplateID(id); err != nil {
		return GamePlanningTemplate{}, err
	}
	item, err := readGamePlanningTemplate(filepath.Join(l.dir(), id+".json"))
	if err != nil {
		return GamePlanningTemplate{}, err
	}
	item.Custom = true
	return item, nil
}

func (l *GamePlanningTemplateLibrary) Create(item GamePlanningTemplate) (GamePlanningTemplate, error) {
	if err := os.MkdirAll(l.dir(), 0o755); err != nil {
		return GamePlanningTemplate{}, err
	}
	if err := validateGamePlanningTemplateWriteBounds(item); err != nil {
		return GamePlanningTemplate{}, err
	}
	item = normalizeGamePlanningTemplate(item)
	if item.ID == "" {
		item.ID = newGamePlanningTemplateID(item.Name)
	}
	if err := validateGamePlanningTemplateID(item.ID); err != nil {
		return GamePlanningTemplate{}, err
	}
	if IsBuiltinGamePlanningTemplateID(item.ID) {
		return GamePlanningTemplate{}, fmt.Errorf("built-in planning templates must be copied before editing: %s", item.ID)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	item.CreatedAt = firstNonEmptyString(item.CreatedAt, now)
	item.UpdatedAt = now
	path := filepath.Join(l.dir(), item.ID+".json")
	document, err := gamePlanningTemplateStore(path).Create(context.Background(), item)
	if errors.Is(err, revisionjson.ErrAlreadyExists) {
		return GamePlanningTemplate{}, fmt.Errorf("planning template already exists: %s", item.ID)
	}
	if err != nil {
		return GamePlanningTemplate{}, err
	}
	item = document.Value
	item.Path, item.Revision, item.Custom = path, document.Revision, true
	return item, nil
}

func (l *GamePlanningTemplateLibrary) Update(id string, item GamePlanningTemplate, baseRevision string) (GamePlanningTemplate, error) {
	id = NormalizeGamePlanningTemplateID(id)
	if err := validateGamePlanningTemplateID(id); err != nil {
		return GamePlanningTemplate{}, err
	}
	if IsBuiltinGamePlanningTemplateID(id) {
		return GamePlanningTemplate{}, fmt.Errorf("built-in planning templates must be copied before editing: %s", id)
	}
	if err := validateGamePlanningTemplateWriteBounds(item); err != nil {
		return GamePlanningTemplate{}, err
	}
	item = normalizeGamePlanningTemplate(item)
	item.ID = id
	path := filepath.Join(l.dir(), id+".json")
	document, err := gamePlanningTemplateStore(path).Update(context.Background(), baseRevision, func(current GamePlanningTemplate) (GamePlanningTemplate, error) {
		item.CreatedAt = firstNonEmptyString(current.CreatedAt, item.CreatedAt)
		item.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		return item, validateGamePlanningTemplate(item)
	})
	if err != nil {
		if errors.Is(err, revisionfile.ErrRevisionConflict) || errors.Is(err, revisionjson.ErrRevisionRequired) {
			return GamePlanningTemplate{}, fmt.Errorf("%w: %v", ErrGamePlanningTemplateRevisionConflict, err)
		}
		return GamePlanningTemplate{}, err
	}
	item = document.Value
	item.Path, item.Revision, item.Custom = path, document.Revision, true
	return item, nil
}

func (l *GamePlanningTemplateLibrary) Delete(id string) error {
	id = NormalizeGamePlanningTemplateID(id)
	if err := validateGamePlanningTemplateID(id); err != nil {
		return err
	}
	if IsBuiltinGamePlanningTemplateID(id) {
		return fmt.Errorf("built-in planning templates cannot be deleted: %s", id)
	}
	return os.Remove(filepath.Join(l.dir(), id+".json"))
}

func (l *GamePlanningTemplateLibrary) dir() string {
	return filepath.Join(l.novaDir, "game-planning-templates")
}

func readGamePlanningTemplate(path string) (GamePlanningTemplate, error) {
	document, err := gamePlanningTemplateStore(path).Read(context.Background())
	if err != nil {
		return GamePlanningTemplate{}, err
	}
	item := document.Value
	item.Path, item.Revision = path, document.Revision
	return item, nil
}

func gamePlanningTemplateStore(path string) revisionjson.Store[GamePlanningTemplate] {
	return revisionjson.NewStore(path, revisionjson.Codec[GamePlanningTemplate]{
		Decode: func(data []byte) (GamePlanningTemplate, error) {
			var item GamePlanningTemplate
			if err := json.Unmarshal(data, &item); err != nil {
				return GamePlanningTemplate{}, fmt.Errorf("parse planning template JSON: %w", err)
			}
			item = normalizeGamePlanningTemplate(item)
			return item, validateGamePlanningTemplate(item)
		},
		Encode: func(item GamePlanningTemplate) ([]byte, error) {
			item = normalizeGamePlanningTemplate(item)
			if err := validateGamePlanningTemplate(item); err != nil {
				return nil, err
			}
			item.Path, item.Revision, item.Error = "", "", ""
			item.Custom, item.Invalid = false, false
			data, err := json.MarshalIndent(item, "", "  ")
			return append(data, '\n'), err
		},
	})
}

func NormalizeGamePlanningTemplateID(id string) string {
	return NormalizeStoryDirectorID(id)
}

func IsBuiltinGamePlanningTemplateID(id string) bool {
	_, ok := builtinGamePlanningTemplateByID(NormalizeGamePlanningTemplateID(id))
	return ok
}

func validateGamePlanningTemplateID(id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("planning template ID is required")
	}
	if id != NormalizeGamePlanningTemplateID(id) {
		return fmt.Errorf("planning template ID may contain only lowercase letters, digits, and hyphens: %s", id)
	}
	return nil
}

func normalizeGamePlanningTemplate(item GamePlanningTemplate) GamePlanningTemplate {
	item.Version = gamePlanningTemplateVersion
	item.ID = NormalizeGamePlanningTemplateID(item.ID)
	item.Name = strings.TrimSpace(item.Name)
	item.Description = strings.TrimSpace(item.Description)
	sections := make([]GamePlanningSection, 0, len(item.Sections))
	usedIDs := map[string]bool{}
	for index, section := range item.Sections {
		section.Title = strings.Join(strings.Fields(section.Title), " ")
		section.Description = strings.TrimSpace(section.Description)
		baseID := NormalizeGamePlanningTemplateID(section.ID)
		if baseID == "" {
			baseID = NormalizeGamePlanningTemplateID(section.Title)
		}
		if baseID == "" {
			baseID = fmt.Sprintf("section-%d", index+1)
		}
		section.ID = baseID
		for suffix := 2; usedIDs[section.ID]; suffix++ {
			section.ID = fmt.Sprintf("%s-%d", baseID, suffix)
		}
		usedIDs[section.ID] = true
		sections = append(sections, section)
	}
	item.Sections = sections
	return item
}

func validateGamePlanningTemplateWriteBounds(item GamePlanningTemplate) error {
	if len([]byte(strings.TrimSpace(item.Name))) > 256 {
		return errors.New("planning template name exceeds 256 bytes")
	}
	if len([]byte(strings.TrimSpace(item.Description))) > 1024 {
		return errors.New("planning template description exceeds 1024 bytes")
	}
	if len(item.Sections) > MaxGamePlanningTemplateSections {
		return fmt.Errorf("planning template may contain at most %d sections", MaxGamePlanningTemplateSections)
	}
	for _, section := range item.Sections {
		if len([]byte(strings.TrimSpace(section.Title))) > maxGamePlanningSectionTitleBytes {
			return fmt.Errorf("planning section title exceeds %d bytes", maxGamePlanningSectionTitleBytes)
		}
		if len([]byte(strings.TrimSpace(section.Description))) > maxGamePlanningSectionDescriptionBytes {
			return fmt.Errorf("planning section description exceeds %d bytes", maxGamePlanningSectionDescriptionBytes)
		}
	}
	return nil
}

func validateGamePlanningTemplate(item GamePlanningTemplate) error {
	if err := validateGamePlanningTemplateID(item.ID); err != nil {
		return err
	}
	if err := validateGamePlanningTemplateWriteBounds(item); err != nil {
		return err
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("planning template name is required")
	}
	if len(item.Sections) == 0 {
		return errors.New("planning template requires at least one section")
	}
	seenTitles := map[string]bool{}
	for _, section := range item.Sections {
		if strings.TrimSpace(section.Title) == "" {
			return errors.New("planning section title is required")
		}
		key := strings.ToLower(strings.TrimSpace(section.Title))
		if seenTitles[key] {
			return fmt.Errorf("planning section titles must be unique: %s", section.Title)
		}
		seenTitles[key] = true
	}
	if len([]byte(RenderGamePlanningTemplateMarkdown(item))) > StoryContextMaxBytes {
		return fmt.Errorf("rendered planning template exceeds %d bytes", StoryContextMaxBytes)
	}
	return nil
}

func newGamePlanningTemplateID(name string) string {
	base := NormalizeGamePlanningTemplateID(name)
	if base == "" || IsBuiltinGamePlanningTemplateID(base) {
		base = "planning-template"
	}
	return fmt.Sprintf("%s-%d", base, time.Now().UTC().Unix())
}

// RenderGamePlanningTemplateMarkdown keeps the persisted template structured
// while providing the Markdown document expected by branch-plan tooling.
func RenderGamePlanningTemplateMarkdown(item GamePlanningTemplate) string {
	item = normalizeGamePlanningTemplate(item)
	var sb strings.Builder
	for index, section := range item.Sections {
		if index > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "## %s", section.Title)
		if section.Description != "" {
			sb.WriteString("\n\n")
			sb.WriteString(section.Description)
		}
	}
	return strings.TrimSpace(sb.String())
}

// GamePlanningGuideMarkdown projects the selected outline and story-owned
// optional event material into stable English model context.
func GamePlanningGuideMarkdown(item GamePlanningTemplate, eventPackages []EventPackage, limitBytes int) string {
	item = normalizeGamePlanningTemplate(item)
	if len(item.Sections) == 0 {
		item = DefaultGamePlanningTemplate()
	}
	var sb strings.Builder
	sb.WriteString("### Planning document template\n\n")
	sb.WriteString("Use the ordered outline below as a mutable adventure blueprint for the current branch. Preserve every ATX H2 heading so routine turns can update section bodies independently. Plan future direction, setups, entrances, exits, event windows, likely responses, and payoffs; give the active arc and next few candidate scenes more detail than distant material. Use committed history, Actor State, Lore, and user intent only as constraints, never as content to summarize. Remove completed or invalid material instead of archiving it here. Do not duplicate exact state values or action-choice labels. Section descriptions are planning guidance, not established canon or a fixed plot.\n\n")
	sb.WriteString(RenderGamePlanningTemplateMarkdown(item))
	for _, pkg := range eventPackages {
		if !pkg.Enabled {
			continue
		}
		for _, card := range pkg.Events {
			if !card.Enabled || strings.TrimSpace(card.DescriptionMarkdown) == "" {
				continue
			}
			fmt.Fprintf(&sb, "\n\n### Optional event material: %s / %s\n\n%s", strings.TrimSpace(pkg.Name), strings.TrimSpace(card.TypeName), strings.TrimSpace(card.DescriptionMarkdown))
		}
	}
	return trimBytes(strings.TrimSpace(sb.String()), limitBytes)
}

func DefaultGamePlanningTemplate() GamePlanningTemplate {
	item, _ := builtinGamePlanningTemplateByID(DefaultGamePlanningTemplateID)
	return item
}
