package agent

import (
	"strings"

	"nova/config"
)

// ResolveWritingSkillName selects the effective Writing Skill name for this IDE
// turn without reading SKILL.md. The model decides whether to load it through
// the skill tool based on the dynamic turn hint.
func ResolveWritingSkillName(cfg *config.Config, selected string) string {
	name := strings.TrimSpace(selected)
	if name == "" && cfg != nil {
		name = strings.TrimSpace(cfg.WritingSkillDefault)
	}
	if name == "" {
		name = config.DefaultWritingSkillName
	}
	return name
}
