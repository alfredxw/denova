package config

import "strings"

// migrateRenamedModelProfileReferences applies explicit aliases emitted by the
// settings editor. Rename intent cannot be inferred safely from list positions:
// inherited profiles can change list length, while delete-and-add can keep it.
func migrateRenamedModelProfileReferences(existing, incoming Settings) Settings {
	if incoming.ModelProfileAliases == nil {
		incoming.ModelProfileAliases = cloneModelProfileAliases(existing.ModelProfileAliases)
	}
	if incoming.ImageAPIProfileAliases == nil {
		incoming.ImageAPIProfileAliases = cloneModelProfileAliases(existing.ImageAPIProfileAliases)
	}
	incoming.ModelProfileAliases = sanitizeModelProfileAliases(incoming.ModelProfileAliases)
	incoming.ImageAPIProfileAliases = sanitizeModelProfileAliases(incoming.ImageAPIProfileAliases)
	incoming.ModelProfileAliases = dropReintroducedProfileAliases(incoming.ModelProfileAliases, languageProfileIDs(incoming.ModelProfiles))
	incoming.ImageAPIProfileAliases = dropReintroducedProfileAliases(incoming.ImageAPIProfileAliases, imageProfileIDs(incoming.ImageAPIProfiles))
	return applyModelProfileAliases(incoming)
}

// applyModelProfileAliases resolves stale references after all settings layers
// have been merged. This also keeps workspace-scoped SubAgents working when a
// user-scoped model profile changes its model-derived ID.
func applyModelProfileAliases(settings Settings) Settings {
	languageIDs := languageProfileIDs(settings.ModelProfiles)
	languageIDs["default"] = true
	resolveLanguage := func(id string) string {
		return resolveModelProfileAlias(id, settings.ModelProfileAliases, languageIDs, normalizeModelProfileID)
	}
	settings.AgentModels = resolveAgentModelProfileReferences(settings.AgentModels, resolveLanguage)
	settings.SubAgents = resolveSubAgentModelProfileReferences(settings.SubAgents, resolveLanguage)
	settings.ModelProfiles = canonicalizeLanguageProfiles(settings.ModelProfiles, resolveLanguage)

	imageIDs := imageProfileIDs(settings.ImageAPIProfiles)
	imageIDs[DefaultImageAPIProfileID] = true
	resolveImage := func(id string) string {
		return resolveModelProfileAlias(id, settings.ImageAPIProfileAliases, imageIDs, normalizeImageAPIProfileID)
	}
	settings.DefaultImageAPIProfileID = resolveModelProfileAlias(
		settings.DefaultImageAPIProfileID,
		settings.ImageAPIProfileAliases,
		imageIDs,
		normalizeImageAPIProfileID,
	)
	settings.ImageAPIProfiles = canonicalizeImageProfiles(settings.ImageAPIProfiles, resolveImage)
	return settings
}

func resolveModelProfileAlias(id string, aliases map[string]string, existingIDs map[string]bool, normalize func(string) string) string {
	original := normalize(id)
	if original == "" {
		return original
	}
	current := original
	visited := map[string]bool{current: true}
	lastExisting := ""
	if existingIDs[current] {
		lastExisting = current
	}
	for {
		next := normalize(aliases[current])
		if next == "" {
			if lastExisting != "" {
				return lastExisting
			}
			return original
		}
		if visited[next] {
			return original
		}
		visited[next] = true
		current = next
		if existingIDs[current] {
			lastExisting = current
		}
	}
}

func languageProfileIDs(profiles []ModelProfileSettings) map[string]bool {
	ids := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if id := modelProfileID(profile); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func imageProfileIDs(profiles []ImageAPIProfileSettings) map[string]bool {
	ids := make(map[string]bool, len(profiles))
	for _, profile := range profiles {
		if id := imageAPIProfileID(profile); id != "" {
			ids[id] = true
		}
	}
	return ids
}

func canonicalizeLanguageProfiles(profiles []ModelProfileSettings, resolve func(string) string) []ModelProfileSettings {
	type group struct {
		sources []ModelProfileSettings
		targets []ModelProfileSettings
	}
	groups := make(map[string]*group, len(profiles))
	order := make([]string, 0, len(profiles))
	incomplete := make([]ModelProfileSettings, 0)
	for _, profile := range profiles {
		id := modelProfileID(profile)
		if id == "" {
			incomplete = append(incomplete, profile)
			continue
		}
		canonicalID := resolve(id)
		entry := groups[canonicalID]
		if entry == nil {
			entry = &group{}
			groups[canonicalID] = entry
			order = append(order, canonicalID)
		}
		if id == canonicalID {
			entry.targets = append(entry.targets, profile)
		} else {
			entry.sources = append(entry.sources, profile)
		}
	}
	out := make([]ModelProfileSettings, 0, len(order)+len(incomplete))
	for _, canonicalID := range order {
		entry := groups[canonicalID]
		merged := ModelProfileSettings{}
		for _, profile := range entry.sources {
			merged = mergeModelProfile(merged, profile)
		}
		for _, profile := range entry.targets {
			merged = mergeModelProfile(merged, profile)
		}
		merged.ID = canonicalID
		out = append(out, merged)
	}
	return append(out, incomplete...)
}

func canonicalizeImageProfiles(profiles []ImageAPIProfileSettings, resolve func(string) string) []ImageAPIProfileSettings {
	type group struct {
		sources []ImageAPIProfileSettings
		targets []ImageAPIProfileSettings
	}
	groups := make(map[string]*group, len(profiles))
	order := make([]string, 0, len(profiles))
	incomplete := make([]ImageAPIProfileSettings, 0)
	for _, profile := range profiles {
		id := imageAPIProfileID(profile)
		if id == "" {
			incomplete = append(incomplete, profile)
			continue
		}
		canonicalID := resolve(id)
		entry := groups[canonicalID]
		if entry == nil {
			entry = &group{}
			groups[canonicalID] = entry
			order = append(order, canonicalID)
		}
		if id == canonicalID {
			entry.targets = append(entry.targets, profile)
		} else {
			entry.sources = append(entry.sources, profile)
		}
	}
	out := make([]ImageAPIProfileSettings, 0, len(order)+len(incomplete))
	for _, canonicalID := range order {
		entry := groups[canonicalID]
		merged := ImageAPIProfileSettings{}
		for _, profile := range entry.sources {
			merged = mergeImageAPIProfile(merged, profile)
		}
		for _, profile := range entry.targets {
			merged = mergeImageAPIProfile(merged, profile)
		}
		merged.ID = canonicalID
		out = append(out, merged)
	}
	return append(out, incomplete...)
}

func dropReintroducedProfileAliases(aliases map[string]string, profileIDs map[string]bool) map[string]string {
	if len(aliases) == 0 {
		return aliases
	}
	out := cloneModelProfileAliases(aliases)
	for id := range profileIDs {
		delete(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func resolveAgentModelProfileReferences(settings AgentModelSettings, resolve func(string) string) AgentModelSettings {
	apply := func(override AgentModelOverride) AgentModelOverride {
		override.ProfileID = resolve(override.ProfileID)
		return override
	}
	settings.Default = apply(settings.Default)
	settings.IDE = apply(settings.IDE)
	settings.InteractiveStory = apply(settings.InteractiveStory)
	settings.ConfigManager = apply(settings.ConfigManager)
	settings.InteractiveDirector = apply(settings.InteractiveDirector)
	settings.VersionSummary = apply(settings.VersionSummary)
	settings.ToolAgent = apply(settings.ToolAgent)
	settings.Image = apply(settings.Image)
	settings.Automation = apply(settings.Automation)
	settings.ContextCompaction = apply(settings.ContextCompaction)
	return settings
}

func resolveSubAgentModelProfileReferences(subAgents []SubAgentConfig, resolve func(string) string) []SubAgentConfig {
	if subAgents == nil {
		return nil
	}
	out := append([]SubAgentConfig(nil), subAgents...)
	for i := range out {
		out[i].Model.ProfileID = resolve(out[i].Model.ProfileID)
	}
	return out
}

func mergeModelProfileAliases(parent, child map[string]string) map[string]string {
	if child == nil {
		return parent
	}
	out := cloneModelProfileAliases(parent)
	if out == nil {
		out = make(map[string]string, len(child))
	}
	for previousID, nextID := range child {
		out[previousID] = nextID
	}
	return out
}

func sanitizeModelProfileAliases(aliases map[string]string) map[string]string {
	if len(aliases) == 0 {
		return nil
	}
	out := make(map[string]string, len(aliases))
	for previousID, nextID := range aliases {
		previousID = strings.TrimSpace(previousID)
		nextID = strings.TrimSpace(nextID)
		if previousID == "" || nextID == "" || previousID == nextID {
			continue
		}
		out[previousID] = nextID
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneModelProfileAliases(aliases map[string]string) map[string]string {
	if aliases == nil {
		return nil
	}
	out := make(map[string]string, len(aliases))
	for previousID, nextID := range aliases {
		out[previousID] = nextID
	}
	return out
}
