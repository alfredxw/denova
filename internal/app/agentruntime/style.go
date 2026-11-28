package agentruntime

import (
	"fmt"
	"strings"

	"denova/internal/agents/prompts"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

// StyleRules resolves one shared stable prompt projection for Writing and Game.
// Always emit the full style index for the active director so per-turn #scene
// selections stay in the turn message instead of mutating the system prompt.
func StyleRules(dataDir string, globalRefs []string, rules []teller.StyleRule) []prompts.StyleRule {
	converted := make([]prompts.StyleRule, 0, len(rules)+1)
	styleRefs := style.NewLibrary(dataDir)
	if len(globalRefs) > 0 {
		converted = append(converted, prompts.StyleRule{
			Global:          true,
			StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(globalRefs)),
		})
	}
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if scene == "" || (len(rule.StyleRefs) == 0 && len(rule.StyleContents) == 0) {
			continue
		}
		if isGlobalStyleScene(scene) {
			converted = append(converted, prompts.StyleRule{
				Global: true, StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(rule.StyleRefs)),
				StyleContents: rule.StyleContents,
			})
			continue
		}
		converted = append(converted, prompts.StyleRule{
			Scene: scene, StyleReferences: styleReferencesForPrompt(styleRefs.Resolve(rule.StyleRefs)),
			StyleContents: rule.StyleContents,
		})
	}
	return converted
}

func StyleRuleNames(rules []prompts.StyleRule) []string {
	names := make([]string, 0, len(rules))
	for _, rule := range rules {
		scene := strings.TrimSpace(rule.Scene)
		if rule.Global {
			scene = "global"
		}
		names = append(names, fmt.Sprintf("%s -> %d refs, %d legacy contents", scene, len(rule.StyleReferences), len(rule.StyleContents)))
	}
	return names
}

func isGlobalStyleScene(scene string) bool {
	normalized := strings.ToLower(strings.TrimSpace(scene))
	return normalized == "全局" || normalized == "global"
}

func styleReferencesForPrompt(refs []style.Reference) []prompts.StyleReference {
	result := make([]prompts.StyleReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, prompts.StyleReference{
			Name: ref.Name, Description: ref.Description, Path: ref.Path,
			DisplayPath: ref.DisplayPath, Missing: ref.Missing, Error: ref.Error,
		})
	}
	return result
}
