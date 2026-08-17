package agentruntime

import (
	"fmt"
	"strings"

	"denova/internal/agents/prompts"
	"denova/internal/interactive/teller"
	"denova/internal/style"
)

// StyleRules resolves one shared prompt projection for Writing and Game. Both
// modes therefore apply global references, scene filtering, and missing-file
// diagnostics identically.
func StyleRules(dataDir string, globalRefs []string, rules []teller.StyleRule, scenes []string) []prompts.StyleRule {
	converted := make([]prompts.StyleRule, 0, len(rules)+1)
	allowed := styleSceneSet(scenes)
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
		if len(allowed) > 0 && !allowed[scene] {
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

func styleSceneSet(scenes []string) map[string]bool {
	if len(scenes) == 0 {
		return nil
	}
	set := make(map[string]bool, len(scenes))
	for _, scene := range scenes {
		if scene = strings.TrimSpace(scene); scene != "" {
			set[scene] = true
		}
	}
	return set
}
