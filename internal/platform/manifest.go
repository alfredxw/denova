package platform

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"denova/internal/portablepath"
	"github.com/Masterminds/semver/v3"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// Limits bound untrusted package metadata and individual capability inputs.
// Files and packages are rejected in full when too large, never truncated.
const (
	MaxDefinitionBytes = 1 << 20
	MaxFileBytes       = 16 << 20
	MaxPackageBytes    = 256 << 20
	MaxPackageFiles    = 10000
)

var identifier = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func validateID(id string) error {
	if len(id) > 120 || !identifier.MatchString(id) || id == "builtin" || id == "local" {
		return failure("INVALID_ARGUMENT", "Invalid package or contribution ID %q", id)
	}
	return portablepath.ValidateComponent(id)
}

func decodeJSON(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return failure("INVALID_ARGUMENT", "Invalid JSON: %v", err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return failure("INVALID_ARGUMENT", "Expected exactly one JSON value")
	}
	return nil
}

func validateManifest(kind Kind, files map[string][]byte) (Manifest, error) {
	var m Manifest
	if !kind.valid() {
		return m, failure("INVALID_ARGUMENT", "Unknown package kind %q", kind)
	}
	other := Plugin
	if kind == Plugin {
		other = Game
	}
	if _, found := files[other.manifestFile()]; found {
		return m, failure("INVALID_ARGUMENT", "A package must contain exactly one manifest")
	}
	raw, found := files[kind.manifestFile()]
	if !found || len(raw) > MaxDefinitionBytes {
		return m, failure("INVALID_ARGUMENT", "Missing or oversized %s", kind.manifestFile())
	}
	if err := decodeJSON(raw, &m); err != nil {
		return m, err
	}
	if m.ManifestVersion != 1 {
		return m, failure("UNSUPPORTED", "Unsupported manifest version %d", m.ManifestVersion)
	}
	if m.APIMajor != APIMajor {
		return m, failure("API_INCOMPATIBLE", "Package %s needs API %d; host provides %d", m.ID, m.APIMajor, APIMajor)
	}
	if err := validateID(m.ID); err != nil {
		return m, err
	}
	if _, err := semver.StrictNewVersion(m.Version); err != nil {
		return m, failure("INVALID_ARGUMENT", "version: %v", err)
	}
	if err := portablepath.ValidateComponent(m.Version); err != nil {
		return m, err
	}
	if strings.TrimSpace(m.Name.Chinese) == "" || strings.TrimSpace(m.Name.English) == "" {
		return m, failure("INVALID_ARGUMENT", "Both localized names are required")
	}
	if m.Description != nil && (m.Description.Chinese == "" || m.Description.English == "") {
		return m, failure("INVALID_ARGUMENT", "Both localized descriptions are required")
	}
	if (kind == Plugin && (m.Game != nil || m.Definitions != nil || m.Contributes == nil)) ||
		(kind == Game && (m.Game == nil || m.Contributes != nil)) {
		return m, failure("INVALID_ARGUMENT", "Manifest fields do not match %s identity", kind)
	}
	read := func(path string) ([]byte, error) {
		if err := portablepath.Validate(path); err != nil {
			return nil, failure("INVALID_ARGUMENT", "%v", err)
		}
		data, ok := files[path]
		if !ok {
			return nil, failure("INVALID_ARGUMENT", "Referenced file %s is not distributed", path)
		}
		if len(data) > MaxDefinitionBytes {
			return nil, failure("LIMIT_EXCEEDED", "Definition %s exceeds %d bytes", path, MaxDefinitionBytes)
		}
		return data, nil
	}
	if backend := m.backend(); backend != nil {
		if backend.Protocol != "denova-runtime-v1" || backend.Launch.Kind != "runtime" || backend.Launch.Runtime != "node" {
			return m, failure("UNSUPPORTED", "Only the Node denova-runtime-v1 backend is supported")
		}
		if _, err := read(backend.Launch.Entry); err != nil {
			return m, err
		}
	}
	views := map[string]bool{}
	for _, view := range m.Views {
		if err := validateID(view.ID); err != nil {
			return m, err
		}
		if views[view.ID] {
			return m, failure("INVALID_ARGUMENT", "Duplicate view %s", view.ID)
		}
		views[view.ID] = true
		switch view.Source.Kind {
		case "static":
			if _, err := read(view.Source.Path); err != nil {
				return m, err
			}
		case "backend":
			if m.backend() == nil || !validEndpoint(view.Source.Path) {
				return m, failure("INVALID_ARGUMENT", "Invalid backend view %s", view.ID)
			}
		default:
			return m, failure("UNSUPPORTED", "Unsupported view source %s", view.Source.Kind)
		}
	}
	if m.Game != nil {
		if m.Game.Cover != "" {
			if err := portablepath.Validate(m.Game.Cover); err != nil {
				return m, err
			}
			data, found := files[m.Game.Cover]
			if !found {
				return m, failure("INVALID_ARGUMENT", "Game cover %s is not distributed", m.Game.Cover)
			}
			if _, err := coverContentType(data); err != nil {
				return m, err
			}
		}
		if !views[m.Game.ViewID] || !slices.Contains([]string{"self", "story"}, m.Game.Storage.Kind) {
			return m, failure("INVALID_ARGUMENT", "Game requires a declared view and supported storage")
		}
		if (m.Game.Storage.Kind == "story") != (m.Game.Story != nil) {
			return m, failure("INVALID_ARGUMENT", "Story storage requires a Story declaration")
		}
	}
	localeKeys := map[string]map[string]json.RawMessage{}
	for _, locale := range []string{"zh-CN", "en-US"} {
		if len(m.Locales) == 0 {
			break
		}
		data, err := read(m.Locales[locale])
		if err != nil {
			return m, err
		}
		keys := map[string]json.RawMessage{}
		if err := json.Unmarshal(data, &keys); err != nil {
			return m, err
		}
		localeKeys[locale] = keys
	}
	for key := range localeKeys["zh-CN"] {
		if _, ok := localeKeys["en-US"][key]; !ok {
			return m, failure("INVALID_ARGUMENT", "Missing English locale key %s", key)
		}
	}
	for key := range localeKeys["en-US"] {
		if _, ok := localeKeys["zh-CN"][key]; !ok {
			return m, failure("INVALID_ARGUMENT", "Missing Chinese locale key %s", key)
		}
	}
	declarations := []*ConfigurationDeclaration{m.Settings}
	if m.Game != nil {
		declarations = append(declarations, m.Game.Setup)
	}
	for _, declaration := range declarations {
		form, err := readConfiguration(declaration, read)
		if err != nil {
			return m, err
		}
		if form != nil {
			if err := localizeConfiguration(form.Schema, localeKeys, ""); err != nil {
				return m, err
			}
			if err := localizeConfigurationUI(form.UISchema, localeKeys, ""); err != nil {
				return m, err
			}
		}
	}
	permissions := map[string]bool{}
	slots := map[string]bool{}
	for _, slot := range m.ModelSlots {
		if err := validateID(slot.ID); err != nil {
			return m, err
		}
		if slots[slot.ID] || !slices.Contains([]string{"text", "image"}, slot.Kind) {
			return m, failure("INVALID_ARGUMENT", "Model slots must have unique IDs and text or image kind")
		}
		slots[slot.ID] = true
		if slot.TitleKey == "" || localeKeys["zh-CN"][slot.TitleKey] == nil || localeKeys["en-US"][slot.TitleKey] == nil {
			return m, failure("INVALID_ARGUMENT", "Model slot %s requires localized titles", slot.ID)
		}
	}
	for _, permission := range append(slices.Clone(m.Permissions.Required), m.Permissions.Optional...) {
		if !slices.Contains([]string{"agents.run", "tools.invoke", "tools.write", "gameData", "pluginData", "library.read", "library.write", "assets.read", "assets.write", "images.generate", "stories.read", "stories.write", "settings.write"}, permission) {
			return m, failure("UNSUPPORTED", "Unsupported permission %s", permission)
		}
		if permissions[permission] {
			return m, failure("INVALID_ARGUMENT", "Duplicate permission %s", permission)
		}
		permissions[permission] = true
		if kind == Plugin && permission == "gameData" || kind == Game && permission == "pluginData" {
			return m, failure("INVALID_ARGUMENT", "Permission %s belongs to another product kind", permission)
		}
	}
	if m.Game != nil && m.Game.Story != nil {
		if !slices.ContainsFunc(m.ModelSlots, func(slot ModelSlot) bool { return slot.ID == m.Game.Story.ModelSlot && slot.Kind == "text" }) {
			return m, failure("INVALID_ARGUMENT", "Story requires a declared text model slot")
		}
		for _, permission := range []string{"stories.read", "stories.write"} {
			if !slices.Contains(m.Permissions.Required, permission) {
				return m, failure("INVALID_ARGUMENT", "Story games require %s", permission)
			}
		}
	}
	deps := map[string]Dependency{}
	for _, dep := range m.Requires {
		if err := validateID(dep.PluginID); err != nil {
			return m, err
		}
		if _, err := semver.NewConstraint(dep.VersionRange); err != nil || dep.VersionRange == "" {
			return m, failure("INVALID_ARGUMENT", "Invalid dependency version range for %s", dep.PluginID)
		}
		if _, exists := deps[dep.PluginID]; exists || kind == Plugin && dep.PluginID == m.ID {
			return m, failure("INVALID_ARGUMENT", "Duplicate or circular dependency %s", dep.PluginID)
		}
		deps[dep.PluginID] = dep
	}
	c := m.contributions()
	ids := map[string]bool{}
	tools := map[string]bool{}
	sets := map[string]bool{}
	agents := map[string]bool{}
	addID := func(id string) error {
		if err := validateID(id); err != nil {
			return err
		}
		if ids[id] {
			return failure("INVALID_ARGUMENT", "Duplicate contribution %s", id)
		}
		ids[id] = true
		return nil
	}
	for _, tool := range c.Tools {
		if err := addID(tool.ID); err != nil {
			return m, err
		}
		tools[tool.ID] = true
		if m.backend() == nil || tool.Endpoint.Method != "POST" || !validEndpoint(tool.Endpoint.Path) {
			return m, failure("INVALID_ARGUMENT", "Tool %s requires a backend and relative POST endpoint", tool.ID)
		}
		data, err := read(tool.Definition)
		if err != nil {
			return m, err
		}
		var def ToolDefinition
		if err := decodeJSON(data, &def); err != nil {
			return m, err
		}
		if def.Description == "" || !slices.Contains([]string{"pure", "read", "propose", "write"}, def.Effect) {
			return m, failure("INVALID_ARGUMENT", "Tool %s requires description and effect", tool.ID)
		}
		if _, err := compileSchema(def.InputSchema); err != nil {
			return m, err
		}
		if len(def.OutputSchema) > 0 {
			if _, err := compileSchema(def.OutputSchema); err != nil {
				return m, err
			}
		}
	}
	for _, set := range c.Toolsets {
		if err := addID(set.ID); err != nil {
			return m, err
		}
		sets[set.ID] = true
		for _, id := range set.Tools {
			if !tools[id] {
				return m, failure("INVALID_ARGUMENT", "Unknown tool %s in %s", id, set.ID)
			}
		}
	}
	for _, def := range m.privateAgents() {
		if err := addID(def.ID); err != nil {
			return m, err
		}
		agents[def.ID] = true
	}
	checkRef := func(ref string, local map[string]bool) error {
		if strings.HasPrefix(ref, "local:") {
			if kind != Game || !local[strings.TrimPrefix(ref, "local:")] {
				return failure("INVALID_ARGUMENT", "Unknown local reference %s", ref)
			}
			return nil
		}
		if local[ref] {
			return nil
		}
		provider, id, ok := strings.Cut(ref, "/")
		if !ok {
			return failure("INVALID_ARGUMENT", "Unknown reference %s", ref)
		}
		if provider == "builtin" && id == "assistant" {
			return nil
		}
		if provider == m.ID && kind == Plugin && local[id] {
			return nil
		}
		dep, ok := deps[provider]
		if !ok || !slices.Contains(dep.Contributions, id) {
			return failure("INVALID_ARGUMENT", "Reference %s is not covered by requires", ref)
		}
		return nil
	}
	for _, entry := range m.privateAgents() {
		data, err := read(entry.Definition)
		if err != nil {
			return m, err
		}
		var def AgentDefinition
		if err := decodeJSON(data, &def); err != nil {
			return m, err
		}
		if def.Instructions == "" || len(def.Instructions) > 256<<10 {
			return m, failure("INVALID_ARGUMENT", "Agent %s instructions must contain 1..262144 bytes", entry.ID)
		}
		if !slices.ContainsFunc(m.ModelSlots, func(slot ModelSlot) bool {
			return slot.ID == def.ModelSlot && slot.Kind == "text"
		}) {
			return m, failure("INVALID_ARGUMENT", "Agent %s references an undeclared text model slot", entry.ID)
		}
		for _, ref := range def.Tools {
			if err := checkRef(ref, tools); err != nil {
				return m, err
			}
		}
		for _, ref := range def.Toolsets {
			if err := checkRef(ref, sets); err != nil {
				return m, err
			}
		}

	}
	if m.Game != nil {
		for _, ref := range m.Game.Uses.Agents {
			if ref != "builtin/assistant" && (!strings.HasPrefix(ref, "local:") || !agents[strings.TrimPrefix(ref, "local:")]) {
				return m, failure("INVALID_ARGUMENT", "Game Agent %s must be a private definition or builtin/assistant", ref)
			}
		}
		for _, ref := range m.Game.Uses.Toolsets {
			if err := checkRef(ref, sets); err != nil {
				return m, err
			}
		}
	}
	return m, nil
}

func validEndpoint(path string) bool {
	u, err := url.Parse(path)
	return err == nil && strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") && !strings.ContainsAny(path, "\\\r\n") && u.Host == "" && u.RawQuery == "" && u.Fragment == "" && !strings.Contains(u.Path, "..")
}

type closedSchemaLoader struct{}

func (closedSchemaLoader) Load(location string) (any, error) {
	return nil, fmt.Errorf("external schema references are disabled: %s", location)
}

func compileSchema(raw []byte) (*jsonschema.Schema, error) {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, failure("INVALID_ARGUMENT", "Invalid JSON Schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.UseLoader(closedSchemaLoader{})
	c.DefaultDraft(jsonschema.Draft2020)
	if err := c.AddResource("https://denova.invalid/schema", value); err != nil {
		return nil, failure("INVALID_ARGUMENT", "Invalid JSON Schema: %v", err)
	}
	schema, err := c.Compile("https://denova.invalid/schema")
	if err != nil {
		return nil, failure("INVALID_ARGUMENT", "Invalid JSON Schema: %v", err)
	}
	return schema, nil
}

func validateValue(schema *jsonschema.Schema, raw []byte) error {
	value, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err == nil {
		err = schema.Validate(value)
	}
	if err != nil {
		return failure("INVALID_ARGUMENT", "Schema validation failed: %v", err)
	}
	return nil
}
