package platform

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

// ConfigurationDeclaration is a frozen, declarative form. Defaults are TOML;
// schema and optional uiSchema are JSON. Views never implement persistence.
type ConfigurationDeclaration struct {
	Schema   string `json:"schema"`
	Defaults string `json:"defaults"`
	UISchema string `json:"uiSchema,omitempty"`
}

type ConfigurationForm struct {
	Schema   map[string]any `json:"schema"`
	UISchema map[string]any `json:"uiSchema"`
	Defaults map[string]any `json:"defaults"`
}

type ConfigurationIssue struct {
	Path    []string `json:"path"`
	Keyword string   `json:"keyword"`
}

// ConfigurationDocument is the management projection, not a persisted copy.
type ConfigurationDocument struct {
	Revision  string             `json:"revision"`
	Problem   *Error             `json:"problem,omitempty"`
	ReleaseID string             `json:"releaseId"`
	Form      *ConfigurationForm `json:"form"`
	Overrides map[string]any     `json:"overrides"`
	Values    map[string]any     `json:"values"`
	TOML      string             `json:"toml"`
}

// ConfigurationInput replaces all user overrides. Validate converts between the
// form and TOML without saving. An empty override object restores defaults.
type ConfigurationInput struct {
	ExpectedRevision string         `json:"expectedRevision"`
	ReleaseID        string         `json:"releaseId"`
	Format           string         `json:"format"`
	Overrides        map[string]any `json:"overrides"`
	TOML             string         `json:"toml"`
}

func configurationValues(value any) (any, error) {
	switch v := value.(type) {
	case map[string]any:
		result := map[string]any{}
		for key, item := range v {
			parsed, err := configurationValues(item)
			if err != nil {
				return nil, err
			}
			result[key] = parsed
		}
		return result, nil
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			parsed, err := configurationValues(item)
			if err != nil {
				return nil, err
			}
			result[i] = parsed
		}
		return result, nil
	case string, bool:
		return v, nil
	case int:
		return configurationValues(int64(v))
	case int64:
		if v > 9007199254740991 || v < -9007199254740991 {
			return nil, failure("INVALID_CONFIGURATION", "Integer exceeds the browser's exact range")
		}
		return float64(v), nil
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 9007199254740991 {
			return nil, failure("INVALID_CONFIGURATION", "Configuration numbers must be finite and safely representable")
		}
		return v, nil
	default:
		return nil, failure("INVALID_CONFIGURATION", "Unsupported configuration value %T; use strings for dates and omit unset fields", value)
	}
}

func parseConfiguration(raw []byte) (map[string]any, error) {
	if len(raw) > MaxDefinitionBytes {
		return nil, failure("LIMIT_EXCEEDED", "Configuration exceeds %d bytes", MaxDefinitionBytes)
	}
	value := map[string]any{}
	if err := toml.Unmarshal(raw, &value); err != nil {
		return nil, failure("INVALID_TOML", "Invalid configuration TOML: %v", err)
	}
	parsed, err := configurationValues(value)
	if err != nil {
		return nil, err
	}
	return parsed.(map[string]any), nil
}

// Tables merge recursively; arrays and scalar values replace in full.
func mergeConfiguration(base, overrides map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range base {
		result[key] = value
	}
	for key, value := range overrides {
		if object, ok := value.(map[string]any); ok {
			prior, _ := result[key].(map[string]any)
			result[key] = mergeConfiguration(prior, object)
		} else {
			result[key] = value
		}
	}
	return result
}

func readConfiguration(declaration *ConfigurationDeclaration, read func(string) ([]byte, error)) (*ConfigurationForm, error) {
	if declaration == nil {
		return nil, nil
	}
	if filepath.Ext(declaration.Defaults) != ".toml" {
		return nil, failure("INVALID_PACKAGE", "Configuration defaults must use TOML")
	}
	schema, err := read(declaration.Schema)
	if err != nil {
		return nil, err
	}
	form := &ConfigurationForm{UISchema: map[string]any{}}
	if err := json.Unmarshal(schema, &form.Schema); err != nil {
		return nil, err
	}
	if form.Schema["type"] != "object" {
		return nil, failure("INVALID_PACKAGE", "Configuration schema must describe an object")
	}
	defaults, err := read(declaration.Defaults)
	if err != nil {
		return nil, err
	}
	form.Defaults, err = parseConfiguration(defaults)
	if err != nil {
		return nil, err
	}
	if declaration.UISchema != "" {
		data, err := read(declaration.UISchema)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &form.UISchema); err != nil {
			return nil, err
		}
	}
	if _, err := validateConfiguration(form, map[string]any{}); err != nil {
		return nil, err
	}
	return form, nil
}

func validateConfiguration(form *ConfigurationForm, overrides map[string]any) (map[string]any, error) {
	if form == nil {
		if len(overrides) != 0 {
			return nil, failure("INVALID_CONFIGURATION", "The extension does not declare configuration")
		}
		return map[string]any{}, nil
	}
	values := mergeConfiguration(form.Defaults, overrides)
	parsed, err := configurationValues(values)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(form.Schema)
	if err != nil {
		return nil, err
	}
	schema, err := compileSchema(raw)
	if err != nil {
		return nil, err
	}
	if err := schema.Validate(parsed); err != nil {
		problem := &Error{Code: "INVALID_CONFIGURATION", MessageKey: "platform.errors.INVALID_CONFIGURATION", Diagnostic: err.Error()}
		var validation *jsonschema.ValidationError
		if errors.As(err, &validation) {
			var visit func(*jsonschema.ValidationError)
			visit = func(v *jsonschema.ValidationError) {
				if len(v.Causes) == 0 {
					problem.Fields = append(problem.Fields, ConfigurationIssue{Path: v.InstanceLocation, Keyword: strings.Join(v.ErrorKind.KeywordPath(), "/")})
				}
				for _, child := range v.Causes {
					visit(child)
				}
			}
			visit(validation)
		}
		return nil, problem
	}
	return parsed.(map[string]any), nil
}

func (m *Manager) readReleaseFile(release Release, name string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(m.releasePath(release.Ref), filepath.FromSlash(name)))
	if err == nil && len(data) > MaxDefinitionBytes {
		return nil, failure("LIMIT_EXCEEDED", "Configuration definition exceeds %d bytes", MaxDefinitionBytes)
	}
	return data, err
}

func (m *Manager) settingsOverrides(ref PackageRef) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(m.packagePath(ref), "settings.toml"))
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	return parseConfiguration(raw)
}

func (m *Manager) settingsValues(release Release, environment string, supplied map[string]any) (map[string]any, error) {
	form, err := readConfiguration(release.Manifest.Settings, func(name string) ([]byte, error) { return m.readReleaseFile(release, name) })
	if err != nil {
		return nil, err
	}
	// Older releases without a declaration never consume shared settings added
	// by a newer version. Explicit caller-supplied configuration is still invalid.
	if form == nil {
		return validateConfiguration(nil, supplied)
	}
	overrides := map[string]any{}
	if environment == "installed" {
		overrides, err = m.settingsOverrides(release.Ref.Package)
		if err != nil {
			return nil, err
		}
	}
	return validateConfiguration(form, mergeConfiguration(overrides, supplied))
}

func (m *Manager) gameSetup(release Release, supplied map[string]any) (map[string]any, error) {
	form, err := readConfiguration(release.Manifest.Game.Setup, func(name string) ([]byte, error) { return m.readReleaseFile(release, name) })
	if err != nil {
		return nil, err
	}
	return validateConfiguration(form, supplied)
}

// Localization affects only the management projection. Schema constraints and
// model-visible definitions remain unchanged in the frozen package.
func localizeConfiguration(value any, locales map[string]map[string]json.RawMessage, locale string) error {
	switch node := value.(type) {
	case map[string]any:
		if _, exists := node["default"]; exists {
			return failure("INVALID_PACKAGE", "Configuration defaults belong in the TOML defaults file")
		}
		if err := localizeConfigurationText(node, locales, locale, [][2]string{{"x-titleKey", "title"}, {"x-descriptionKey", "description"}}); err != nil {
			return err
		}
		for key, child := range node {
			switch key {
			case "const", "enum", "examples", "x-titleKey", "x-descriptionKey", "title", "description":
				// These contain literal values rather than nested schemas.
				continue
			case "properties", "$defs", "definitions", "patternProperties", "dependentSchemas":
				fields, _ := child.(map[string]any)
				for name, field := range fields {
					if key == "properties" {
						object, _ := field.(map[string]any)
						if label, _ := object["x-titleKey"].(string); label == "" {
							return failure("INVALID_PACKAGE", "Configuration field %s requires x-titleKey", name)
						}
					}
					if err := localizeConfiguration(field, locales, locale); err != nil {
						return err
					}
				}
			default:
				if err := localizeConfiguration(child, locales, locale); err != nil {
					return err
				}
			}
		}
	case []any:
		for _, child := range node {
			if err := localizeConfiguration(child, locales, locale); err != nil {
				return err
			}
		}
	}
	return nil
}

func localizeConfigurationUI(node map[string]any, locales map[string]map[string]json.RawMessage, locale string) error {
	if err := localizeConfigurationText(node, locales, locale, [][2]string{{"ui:titleKey", "ui:title"}, {"ui:descriptionKey", "ui:description"}, {"ui:helpKey", "ui:help"}, {"ui:placeholderKey", "ui:placeholder"}}); err != nil {
		return err
	}
	for _, value := range node {
		if child, ok := value.(map[string]any); ok {
			if err := localizeConfigurationUI(child, locales, locale); err != nil {
				return err
			}
		}
	}
	return nil
}

func localizeConfigurationText(node map[string]any, locales map[string]map[string]json.RawMessage, locale string, pairs [][2]string) error {
	for _, pair := range pairs {
		key, _ := node[pair[0]].(string)
		if _, exists := node[pair[1]]; exists && key == "" {
			return failure("INVALID_PACKAGE", "Configuration text %s requires %s", pair[1], pair[0])
		}
		if key == "" {
			continue
		}
		for _, language := range []string{"zh-CN", "en-US"} {
			var text string
			if err := json.Unmarshal(locales[language][key], &text); err != nil || text == "" {
				return failure("INVALID_PACKAGE", "Missing %s configuration label %s", language, key)
			}
			if language == locale {
				node[pair[1]] = text
			}
		}
	}
	return nil
}

func (m *Manager) configurationDocument(release Release, overrides map[string]any, locale string) (ConfigurationDocument, error) {
	read := func(name string) ([]byte, error) { return m.readReleaseFile(release, name) }
	return configurationDocument(release.Ref.ReleaseID, release.Manifest, release.Manifest.Settings, read, overrides, locale)
}

func configurationDocument(releaseID string, manifest Manifest, declaration *ConfigurationDeclaration, read func(string) ([]byte, error), overrides map[string]any, locale string) (ConfigurationDocument, error) {
	form, err := readConfiguration(declaration, read)
	if err != nil {
		return ConfigurationDocument{}, err
	}
	values, validationErr := validateConfiguration(form, overrides)
	var problem *Error
	if validationErr != nil {
		_, problem = ErrorResponse(validationErr)
		values = overrides
		if form != nil {
			values = mergeConfiguration(form.Defaults, overrides)
		}
	}
	if form != nil {
		locales := map[string]map[string]json.RawMessage{}
		for language, file := range manifest.Locales {
			raw, err := read(file)
			if err != nil {
				return ConfigurationDocument{}, err
			}
			var labels map[string]json.RawMessage
			if err := json.Unmarshal(raw, &labels); err != nil {
				return ConfigurationDocument{}, err
			}
			locales[language] = labels
		}
		if err := localizeConfiguration(form.Schema, locales, locale); err != nil {
			return ConfigurationDocument{}, err
		}
		if err := localizeConfigurationUI(form.UISchema, locales, locale); err != nil {
			return ConfigurationDocument{}, err
		}
	}
	raw, err := toml.Marshal(overrides)
	if err != nil {
		return ConfigurationDocument{}, fmt.Errorf("encode configuration: %w", err)
	}
	return ConfigurationDocument{ReleaseID: releaseID, Form: form, Overrides: overrides, Values: values, TOML: string(raw), Problem: problem}, nil
}

// SetupConfiguration projects frozen starting options without reading shared
// settings. The selected release is checked again when the instance is created.
func (m *Manager) SetupConfiguration(gameID, releaseID, locale string) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	release, _, err := m.release(ReleaseRef{Package: PackageRef{Kind: Game, ID: gameID}, ReleaseID: releaseID})
	if err != nil {
		return ConfigurationDocument{}, err
	}
	return configurationDocument(releaseID, release.Manifest, release.Manifest.Game.Setup, func(name string) ([]byte, error) { return m.readReleaseFile(release, name) }, map[string]any{}, locale)
}

// CandidateConfiguration supplies host-rendered forms for isolated previews.
func (m *Manager) CandidateConfiguration(id, purpose, locale string) (ConfigurationDocument, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	candidate := m.candidates[id]
	if candidate == nil {
		return ConfigurationDocument{}, failure("NOT_FOUND", "Preview candidate is unavailable")
	}
	declaration := candidate.Manifest.Settings
	switch purpose {
	case "settings":
	case "setup":
		if candidate.Manifest.Game == nil {
			return ConfigurationDocument{}, failure("INVALID_ARGUMENT", "Only games have starting options")
		}
		declaration = candidate.Manifest.Game.Setup
	default:
		return ConfigurationDocument{}, failure("INVALID_ARGUMENT", "Unknown configuration purpose")
	}
	return configurationDocument(candidate.Digest, candidate.Manifest, declaration, func(name string) ([]byte, error) { return candidate.files[name], nil }, map[string]any{}, locale)
}
