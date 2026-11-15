package antigravity

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrUnsafeCustomization means the installed CLI cannot isolate executable
// ambient hooks or enforce its OS terminal sandbox. User files stay intact.
var ErrUnsafeCustomization = errors.New("Antigravity runtime isolation cannot be verified")

func checkIsolation(dir string) error {
	if err := checkAmbientHooksInDir(dir); err != nil {
		if errors.Is(err, ErrUnsafeCustomization) {
			return err
		}
		return fmt.Errorf("%w: %w", ErrUnsafeCustomization, err)
	}
	if err := verifyOSSandbox(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnsafeCustomization, err)
	}
	return nil
}

// The CLI's hook locations and enabled flag are documented at
// antigravity.google/docs/hooks. Inheritance controls do not document hook
// suppression, so refuse active global hooks before starting a conversation.
func checkAmbientHooks() error {
	return checkAmbientHooksInDir("")
}

func checkAmbientHooksInDir(dir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sources := []struct {
		path     string
		settings bool
	}{
		{filepath.Join(home, ".gemini", "config", "hooks.json"), false},
		{filepath.Join(home, ".gemini", "antigravity-cli", "settings.json"), true},
	}
	if dir != "" {
		sources = append(sources, struct {
			path     string
			settings bool
		}{filepath.Join(dir, ".agents", "hooks.json"), false})
	}
	for _, source := range sources {
		body, err := os.ReadFile(source.path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect Antigravity hook configuration: %w", err)
		}
		var hooks map[string]json.RawMessage
		if err := json.Unmarshal(body, &hooks); err != nil {
			return fmt.Errorf("inspect Antigravity hook configuration: %w", err)
		}
		if source.settings {
			raw := hooks["hooks"]
			if len(raw) == 0 {
				continue
			}
			hooks = nil
			if err := json.Unmarshal(raw, &hooks); err != nil {
				return fmt.Errorf("inspect Antigravity settings hooks: %w", err)
			}
		}
		for _, raw := range hooks {
			var definition struct {
				Enabled *bool `json:"enabled"`
			}
			if err := json.Unmarshal(raw, &definition); err != nil {
				return fmt.Errorf("inspect Antigravity hook definition: %w", err)
			}
			if definition.Enabled == nil || *definition.Enabled {
				return fmt.Errorf("%w: active hooks in %s", ErrUnsafeCustomization, source.path)
			}
		}
	}
	return nil
}
