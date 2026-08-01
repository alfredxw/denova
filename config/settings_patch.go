package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	jsonpatch "github.com/evanphx/json-patch/v5"
)

// SettingsLayer identifies a writable persisted settings document. Resolved
// settings and runtime state are intentionally outside this mutation seam.
type SettingsLayer string

const (
	SettingsLayerUser      SettingsLayer = "user"
	SettingsLayerWorkspace SettingsLayer = "workspace"
)

var (
	ErrInvalidSettingsPatch     = errors.New("invalid settings patch")
	ErrUnsupportedSettingsLayer = errors.New("unsupported settings layer")
)

// ParseSettingsLayer validates the stable API vocabulary for writable layers.
func ParseSettingsLayer(value string) (SettingsLayer, error) {
	layer := SettingsLayer(strings.ToLower(strings.TrimSpace(value)))
	switch layer {
	case SettingsLayerUser, SettingsLayerWorkspace:
		return layer, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrUnsupportedSettingsLayer, value)
	}
}

// ApplySettingsMergePatch applies RFC 7386 object merge semantics to a
// Settings document. Missing fields are preserved, arrays are replaced, and a
// JSON null clears a field. Strict decoding rejects misspelled or retired keys.
func ApplySettingsMergePatch(existing Settings, changes json.RawMessage) (Settings, error) {
	if err := validateSettingsPatchObject(changes); err != nil {
		return Settings{}, err
	}
	document, err := json.Marshal(existing)
	if err != nil {
		return Settings{}, fmt.Errorf("encode current settings: %w", err)
	}
	merged, err := jsonpatch.MergePatch(document, changes)
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	var next Settings
	decoder := json.NewDecoder(bytes.NewReader(merged))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&next); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	if err := ensureSettingsJSONEOF(decoder); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	return next, nil
}

// ValidateWorkspaceSettingsPatch rejects user-only fields instead of silently
// retaining them in a workspace document where they can never take effect.
func ValidateWorkspaceSettingsPatch(changes json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(changes, &fields); err != nil || fields == nil {
		return fmt.Errorf("%w: changes must be a JSON object", ErrInvalidSettingsPatch)
	}
	for field := range fields {
		switch field {
		case "agent_tools", "agent_prompts", "agent_skills", "agent_context",
			"general_sub_agents", "sub_agents", "agent_tool_parallelism":
		default:
			return fmt.Errorf("%w: field %q is not workspace-scoped", ErrInvalidSettingsPatch, field)
		}
	}
	return nil
}

func validateSettingsPatchObject(changes json.RawMessage) error {
	trimmed := bytes.TrimSpace(changes)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("%w: changes must be a JSON object", ErrInvalidSettingsPatch)
	}
	var value map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &value); err != nil || value == nil {
		if err == nil {
			err = errors.New("changes must be a JSON object")
		}
		return fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	// Decode the patch itself as Settings as well as the merged document.
	// RFC 7386 drops unknown keys whose value is null, so validating only the
	// merge result would otherwise let a misspelled clear operation pass.
	var shape Settings
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&shape); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	if err := ensureSettingsJSONEOF(decoder); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSettingsPatch, err)
	}
	return nil
}

func ensureSettingsJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("unexpected trailing JSON value")
		}
		return err
	}
	return nil
}
