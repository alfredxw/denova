package skills

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	einoskill "github.com/cloudwego/eino/adk/middlewares/skill"
	"gopkg.in/yaml.v3"
)

const (
	SkillFileName = "SKILL.md"

	ScopeBuiltin   Scope = "builtin"
	ScopeUser      Scope = "user"
	ScopeWorkspace Scope = "workspace"
)

var skillNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// Scope identifies where a skill definition is stored.
type Scope string

// Directory is a scanned skill root. Later directories override earlier ones.
type Directory struct {
	Scope    Scope  `json:"scope"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}

// ScopeInfo is returned to the frontend for displaying editable locations.
type ScopeInfo struct {
	Scope    Scope  `json:"scope"`
	Path     string `json:"path"`
	Writable bool   `json:"writable"`
}

// SkillSummary describes a discovered skill.
type SkillSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Context     string `json:"context,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Scope       Scope  `json:"scope"`
	Path        string `json:"path"`
	Editable    bool   `json:"editable"`
	Active      bool   `json:"active"`
	UpdatedAt   string `json:"updated_at,omitempty"`
}

// Snapshot is the full skills management view returned by the API.
type Snapshot struct {
	Scopes []ScopeInfo    `json:"scopes"`
	Skills []SkillSummary `json:"skills"`
}

// Document is a single editable SKILL.md payload.
type Document struct {
	SkillSummary
	Content string `json:"content"`
}

type record struct {
	skill   einoskill.Skill
	summary SkillSummary
}

type frontMatterFile struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Agent       string `yaml:"agent,omitempty"`
}

// NewDirectories returns the canonical skill search path for Nova.
func NewDirectories(builtinDir, novaDir, workspace string) []Directory {
	dirs := make([]Directory, 0, 3)
	if path := normalizePath(builtinDir); path != "" {
		dirs = append(dirs, Directory{Scope: ScopeBuiltin, Path: path, Writable: false})
	}
	if path := normalizePath(filepath.Join(novaDir, "skills")); novaDir != "" && path != "" {
		dirs = append(dirs, Directory{Scope: ScopeUser, Path: path, Writable: true})
	}
	if path := normalizePath(filepath.Join(workspace, ".nova", "skills")); workspace != "" && path != "" {
		dirs = append(dirs, Directory{Scope: ScopeWorkspace, Path: path, Writable: true})
	}
	return dirs
}

// Backend adapts multiple Nova skill directories to Eino's skill.Backend.
type Backend struct {
	dirs      []Directory
	agentKind string
	overrides map[string]bool
}

func NewBackend(dirs []Directory) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs)}
}

func NewAgentBackend(dirs []Directory, agentKind string, overrides map[string]bool) *Backend {
	return &Backend{dirs: dedupeDirectories(dirs), agentKind: strings.TrimSpace(agentKind), overrides: normalizeOverrideMap(overrides)}
}

func (b *Backend) List(ctx context.Context) ([]einoskill.FrontMatter, error) {
	records := b.activeRecords(ctx)
	matters := make([]einoskill.FrontMatter, 0, len(records))
	for _, rec := range records {
		matters = append(matters, rec.skill.FrontMatter)
	}
	sort.Slice(matters, func(i, j int) bool {
		return matters[i].Name < matters[j].Name
	})
	return matters, nil
}

func (b *Backend) Get(ctx context.Context, name string) (einoskill.Skill, error) {
	for _, rec := range b.activeRecords(ctx) {
		if rec.skill.Name == name {
			return rec.skill, nil
		}
	}
	return einoskill.Skill{}, fmt.Errorf("skill not found: %s", name)
}

func SnapshotFor(ctx context.Context, dirs []Directory) (Snapshot, error) {
	dirs = dedupeDirectories(dirs)
	records := loadRecords(ctx, dirs)
	active := activeRecordKeys(records)
	summaries := make([]SkillSummary, 0, len(records))
	for _, rec := range records {
		item := rec.summary
		item.Active = active[recordKey(rec)]
		summaries = append(summaries, item)
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Active != summaries[j].Active {
			return summaries[i].Active
		}
		if summaries[i].Name != summaries[j].Name {
			return summaries[i].Name < summaries[j].Name
		}
		return scopeRank(summaries[i].Scope) > scopeRank(summaries[j].Scope)
	})
	return Snapshot{Scopes: scopeInfos(dirs), Skills: summaries}, nil
}

func ReadDocument(ctx context.Context, dirs []Directory, scope Scope, name string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dirs = dedupeDirectories(dirs)
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	path := filepath.Join(dir.Path, name, SkillFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, err
	}
	rec, err := parseRecord(ctx, dir, path, string(data))
	if err != nil {
		return Document{}, err
	}
	active := activeRecordKeys(loadRecords(ctx, dirs))
	rec.summary.Active = active[recordKey(rec)]
	return Document{SkillSummary: rec.summary, Content: string(data)}, nil
}

func CreateDocument(ctx context.Context, dirs []Directory, scope Scope, name, description string, agents ...string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	content := DefaultContent(name, description, agents...)
	return writeDocument(ctx, dirs, dir, name, content, false)
}

func SaveDocument(ctx context.Context, dirs []Directory, scope Scope, name, content string) (Document, error) {
	if err := ValidateName(name); err != nil {
		return Document{}, err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return Document{}, err
	}
	return writeDocument(ctx, dirs, dir, name, content, true)
}

// SaveDocumentAs writes a skill to a new editable scope/name and removes the old
// editable document after the new copy has been validated and written.
func SaveDocumentAs(ctx context.Context, dirs []Directory, sourceScope Scope, sourceName string, targetScope Scope, targetName, content string) (Document, error) {
	sourceName = strings.TrimSpace(sourceName)
	targetName = strings.TrimSpace(targetName)
	if targetScope == "" {
		targetScope = sourceScope
	}
	if targetName == "" {
		targetName = sourceName
	}
	if sourceScope == targetScope && sourceName == targetName {
		return SaveDocument(ctx, dirs, sourceScope, sourceName, content)
	}
	if err := ValidateName(sourceName); err != nil {
		return Document{}, err
	}
	if err := ValidateName(targetName); err != nil {
		return Document{}, err
	}
	sourceDir, err := writableDirectoryForScope(dirs, sourceScope)
	if err != nil {
		return Document{}, err
	}
	targetDir, err := writableDirectoryForScope(dirs, targetScope)
	if err != nil {
		return Document{}, err
	}
	sourceSkillDir := filepath.Join(sourceDir.Path, sourceName)
	if _, err := os.Stat(filepath.Join(sourceSkillDir, SkillFileName)); err != nil {
		return Document{}, err
	}
	targetPath := filepath.Join(targetDir.Path, targetName, SkillFileName)
	if _, err := os.Stat(targetPath); err == nil {
		return Document{}, fmt.Errorf("skill already exists in %s scope: %s", targetScope, targetName)
	} else if !os.IsNotExist(err) {
		return Document{}, err
	}
	if _, err := writeDocument(ctx, dirs, targetDir, targetName, content, false); err != nil {
		return Document{}, err
	}
	if err := os.RemoveAll(sourceSkillDir); err != nil {
		return Document{}, err
	}
	return ReadDocument(ctx, dirs, targetScope, targetName)
}

func DeleteDocument(ctx context.Context, dirs []Directory, scope Scope, name string) error {
	_ = ctx
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := writableDirectoryForScope(dirs, scope)
	if err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(dir.Path, name))
}

func ValidateName(name string) error {
	if !skillNamePattern.MatchString(strings.TrimSpace(name)) {
		return fmt.Errorf("skill name must match %s", skillNamePattern.String())
	}
	return nil
}

func DefaultContent(name, description string, agents ...string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		description = fmt.Sprintf("Use this skill when the user asks for %s-specific guidance.", name)
	}
	frontmatter := marshalFrontmatter(name, description, normalizeAgentList(agents))
	return fmt.Sprintf(`---
%s---

# %s

Describe when to use this skill, what context to gather, and the concrete workflow the agent should follow.
`, frontmatter, name)
}

func (b *Backend) activeRecords(ctx context.Context) []record {
	records := loadRecords(ctx, b.dirs)
	active := make(map[string]record)
	for _, rec := range records {
		active[rec.skill.Name] = rec
	}
	out := make([]record, 0, len(active))
	for _, rec := range active {
		if !skillAllowedForAgent(rec, b.agentKind, b.overrides) {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].skill.Name < out[j].skill.Name
	})
	return out
}

func skillAllowedForAgent(rec record, agentKind string, overrides map[string]bool) bool {
	if agentKind == "" {
		return true
	}
	if enabled, ok := overrides[rec.skill.Name]; ok {
		return enabled
	}
	return agentMatches(rec.skill.Agent, agentKind)
}

func agentMatches(agentField, agentKind string) bool {
	agentField = strings.TrimSpace(agentField)
	if agentField == "" {
		return true
	}
	for _, part := range strings.FieldsFunc(agentField, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	}) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" || strings.EqualFold(part, "all") || part == agentKind {
			return true
		}
	}
	return false
}

func normalizeOverrideMap(overrides map[string]bool) map[string]bool {
	if len(overrides) == 0 {
		return nil
	}
	out := make(map[string]bool, len(overrides))
	for name, enabled := range overrides {
		name = strings.TrimSpace(name)
		if name != "" {
			out[name] = enabled
		}
	}
	return out
}

func loadRecords(ctx context.Context, dirs []Directory) []record {
	var records []record
	for _, dir := range dedupeDirectories(dirs) {
		entries, err := os.ReadDir(dir.Path)
		if err != nil {
			if !os.IsNotExist(err) {
				log.Printf("[skills] scan skill directory failed scope=%s path=%s err=%v", dir.Scope, dir.Path, err)
			}
			continue
		}
		for _, entry := range entries {
			if ctx.Err() != nil {
				return records
			}
			if !entry.IsDir() {
				continue
			}
			path := filepath.Join(dir.Path, entry.Name(), SkillFileName)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				if !os.IsNotExist(readErr) {
					log.Printf("[skills] read skill failed scope=%s path=%s err=%v", dir.Scope, path, readErr)
				}
				continue
			}
			rec, parseErr := parseRecord(ctx, dir, path, string(data))
			if parseErr != nil {
				log.Printf("[skills] parse skill failed scope=%s path=%s err=%v", dir.Scope, path, parseErr)
				continue
			}
			records = append(records, rec)
		}
	}
	return records
}

func parseRecord(ctx context.Context, dir Directory, path, data string) (record, error) {
	if ctx.Err() != nil {
		return record{}, ctx.Err()
	}
	frontmatter, body, err := parseFrontmatter(data)
	if err != nil {
		return record{}, err
	}
	var fm einoskill.FrontMatter
	if err := yaml.Unmarshal([]byte(frontmatter), &fm); err != nil {
		return record{}, err
	}
	fm.Name = strings.TrimSpace(fm.Name)
	fm.Description = strings.TrimSpace(fm.Description)
	if err := ValidateName(fm.Name); err != nil {
		return record{}, err
	}
	if fm.Description == "" {
		return record{}, fmt.Errorf("skill description is required")
	}
	info, _ := os.Stat(path)
	updatedAt := ""
	if info != nil {
		updatedAt = info.ModTime().UTC().Format("2006-01-02T15:04:05Z")
	}
	baseDir := filepath.Dir(path)
	return record{
		skill: einoskill.Skill{
			FrontMatter:   fm,
			Content:       strings.TrimSpace(body),
			BaseDirectory: baseDir,
		},
		summary: SkillSummary{
			Name:        fm.Name,
			Description: fm.Description,
			Context:     string(fm.Context),
			Agent:       fm.Agent,
			Model:       fm.Model,
			Scope:       dir.Scope,
			Path:        path,
			Editable:    dir.Writable,
			UpdatedAt:   updatedAt,
		},
	}, nil
}

func writeDocument(ctx context.Context, dirs []Directory, dir Directory, name, content string, overwrite bool) (Document, error) {
	if ctx.Err() != nil {
		return Document{}, ctx.Err()
	}
	skillDir := filepath.Join(dir.Path, name)
	path := filepath.Join(skillDir, SkillFileName)
	if !overwrite {
		if _, err := os.Stat(path); err == nil {
			return Document{}, fmt.Errorf("skill already exists: %s", name)
		}
	}
	rec, err := parseRecord(ctx, dir, path, content)
	if err != nil {
		return Document{}, err
	}
	if rec.skill.Name != name {
		return Document{}, fmt.Errorf("frontmatter name %q must match skill directory %q", rec.skill.Name, name)
	}
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		return Document{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return Document{}, err
	}
	doc, err := ReadDocument(ctx, dirs, dir.Scope, name)
	if err != nil {
		return Document{}, err
	}
	return doc, nil
}

func parseFrontmatter(data string) (string, string, error) {
	const delimiter = "---"
	data = strings.TrimSpace(data)
	if !strings.HasPrefix(data, delimiter) {
		return "", "", fmt.Errorf("file does not start with frontmatter delimiter")
	}
	rest := data[len(delimiter):]
	endIdx := strings.Index(rest, "\n"+delimiter)
	if endIdx == -1 {
		return "", "", fmt.Errorf("frontmatter closing delimiter not found")
	}
	frontmatter := strings.TrimSpace(rest[:endIdx])
	content := rest[endIdx+len("\n"+delimiter):]
	if strings.HasPrefix(content, "\n") {
		content = content[1:]
	}
	return frontmatter, content, nil
}

func directoryForScope(dirs []Directory, scope Scope) (Directory, error) {
	for _, dir := range dedupeDirectories(dirs) {
		if dir.Scope == scope {
			return dir, nil
		}
	}
	return Directory{}, fmt.Errorf("skill scope not configured: %s", scope)
}

func writableDirectoryForScope(dirs []Directory, scope Scope) (Directory, error) {
	dir, err := directoryForScope(dirs, scope)
	if err != nil {
		return Directory{}, err
	}
	if !dir.Writable {
		return Directory{}, fmt.Errorf("skill scope is read-only: %s", scope)
	}
	return dir, nil
}

func activeRecordKeys(records []record) map[string]bool {
	activeByName := make(map[string]record)
	for _, rec := range records {
		activeByName[rec.skill.Name] = rec
	}
	keys := make(map[string]bool, len(activeByName))
	for _, rec := range activeByName {
		keys[recordKey(rec)] = true
	}
	return keys
}

func recordKey(rec record) string {
	return string(rec.summary.Scope) + "\x00" + rec.summary.Path
}

func scopeInfos(dirs []Directory) []ScopeInfo {
	out := make([]ScopeInfo, 0, len(dirs))
	for _, dir := range dirs {
		out = append(out, ScopeInfo{Scope: dir.Scope, Path: dir.Path, Writable: dir.Writable})
	}
	return out
}

func dedupeDirectories(dirs []Directory) []Directory {
	seen := map[string]bool{}
	out := make([]Directory, 0, len(dirs))
	for _, dir := range dirs {
		if dir.Path == "" {
			continue
		}
		path := normalizePath(dir.Path)
		key := string(dir.Scope) + "\x00" + path
		if seen[key] {
			continue
		}
		seen[key] = true
		dir.Path = path
		out = append(out, dir)
	}
	return out
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if abs, err := filepath.Abs(path); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(path)
}

func scopeRank(scope Scope) int {
	switch scope {
	case ScopeWorkspace:
		return 3
	case ScopeUser:
		return 2
	case ScopeBuiltin:
		return 1
	default:
		return 0
	}
}

func marshalFrontmatter(name, description string, agents []string) string {
	data, err := yaml.Marshal(frontMatterFile{Name: name, Description: description, Agent: strings.Join(agents, ",")})
	if err != nil {
		agentLine := ""
		if len(agents) > 0 {
			agentLine = fmt.Sprintf("agent: %q\n", strings.Join(agents, ","))
		}
		return fmt.Sprintf("name: %q\ndescription: %q\n%s", name, description, agentLine)
	}
	return string(data)
}

func normalizeAgentList(agents []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(agents))
	for _, agent := range agents {
		agent = strings.TrimSpace(agent)
		if agent == "" || seen[agent] {
			continue
		}
		seen[agent] = true
		out = append(out, agent)
	}
	return out
}
