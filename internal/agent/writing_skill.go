package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"nova/config"
	novaskills "nova/internal/skills"
)

const maxWritingSkillContextChars = 24000

var builtinWritingSkillNames = map[string]bool{
	"novel-lite":     true,
	"novel-standard": true,
	"novel-heavy":    true,
}

// WritingSkillContext is the bounded, effective SKILL.md selected for one IDE
// writing turn. Scope/path are surfaced so context analysis can explain which
// override layer won without exposing the full skill search implementation.
type WritingSkillContext struct {
	Name      string
	Scope     string
	Source    string
	Path      string
	Content   string
	Truncated bool
	MaxChars  int
}

// ResolveWritingSkillContext selects the active IDE-compatible writing Skill
// for a request. Empty selected values fall back to cfg.WritingSkillDefault,
// then to the built-in standard preset.
func ResolveWritingSkillContext(ctx context.Context, cfg *config.Config, selected string) (*WritingSkillContext, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	name := strings.TrimSpace(selected)
	if name == "" {
		name = strings.TrimSpace(cfg.WritingSkillDefault)
	}
	if name == "" {
		name = config.DefaultWritingSkillName
	}
	dirs := novaskills.NewDirectories(cfg.SkillsDir, cfg.NovaDir, cfg.Workspace)
	backend := novaskills.NewAgentBackend(dirs, config.AgentKindIDE, config.ResolveAgentSkillOverrides(cfg, config.AgentKindIDE))
	skill, err := backend.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("writing skill %q is not active for IDE agent: %w", name, err)
	}

	summary, err := activeSkillSummary(ctx, dirs, name, skill.BaseDirectory)
	if err != nil {
		return nil, err
	}
	content, truncated := boundWritingSkillContent(skill.Content, maxWritingSkillContextChars)
	return &WritingSkillContext{
		Name:      name,
		Scope:     string(summary.Scope),
		Source:    string(summary.Scope),
		Path:      summary.Path,
		Content:   content,
		Truncated: truncated,
		MaxChars:  maxWritingSkillContextChars,
	}, nil
}

func activeSkillSummary(ctx context.Context, dirs []novaskills.Directory, name, baseDir string) (novaskills.SkillSummary, error) {
	snapshot, err := novaskills.SnapshotFor(ctx, dirs)
	if err != nil {
		return novaskills.SkillSummary{}, err
	}
	baseDir = filepath.Clean(baseDir)
	var fallback *novaskills.SkillSummary
	for i := range snapshot.Skills {
		summary := snapshot.Skills[i]
		if !summary.Active || summary.Name != name {
			continue
		}
		if fallback == nil {
			copied := summary
			fallback = &copied
		}
		if baseDir != "." && filepath.Clean(filepath.Dir(summary.Path)) == baseDir {
			return summary, nil
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return novaskills.SkillSummary{}, fmt.Errorf("active writing skill summary not found: %s", name)
}

func boundWritingSkillContent(content string, maxChars int) (string, bool) {
	content = strings.TrimSpace(content)
	if maxChars <= 0 || content == "" {
		return "", false
	}
	if utf8.RuneCountInString(content) <= maxChars {
		return content, false
	}
	runes := []rune(content)
	return string(runes[:maxChars]) + "\n\n[Writing Skill content truncated by Nova]", true
}

func isBuiltinWritingSkill(name string) bool {
	return builtinWritingSkillNames[strings.TrimSpace(name)]
}
