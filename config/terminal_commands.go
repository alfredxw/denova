package config

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	TerminalCommandCodexID  = "codex"
	TerminalCommandClaudeID = "claude"

	DefaultTerminalCodexCommand  = "codex"
	DefaultTerminalClaudeCommand = "claude"

	maxTerminalCommandIDRunes   = 128
	maxTerminalCommandNameRunes = 128
	maxTerminalCommandLineBytes = 32 * 1024
)

var ErrInvalidTerminalCommand = errors.New("invalid terminal command configuration")

// TerminalCommandSettings is one user-owned shortcut shown in the terminal
// launch menu. ID is stable across renames so persisted tabs never depend on a
// display label or copy an executable command into browser storage.
type TerminalCommandSettings struct {
	ID      string `toml:"id" json:"id"`
	Name    string `toml:"name" json:"name"`
	Command string `toml:"command" json:"command"`
	Enabled bool   `toml:"enabled" json:"enabled"`
}

// DefaultTerminalCommands returns editable presets. They use the same model as
// every custom CLI; only their stable IDs and initial values are built in.
func DefaultTerminalCommands() []TerminalCommandSettings {
	return []TerminalCommandSettings{
		{ID: TerminalCommandCodexID, Name: "Codex CLI", Command: DefaultTerminalCodexCommand, Enabled: true},
		{ID: TerminalCommandClaudeID, Name: "Claude Code", Command: DefaultTerminalClaudeCommand, Enabled: true},
	}
}

// normalizeTerminalCommands trims presentation input without dropping,
// truncating, reordering, or otherwise hiding user configuration.
func normalizeTerminalCommands(commands []TerminalCommandSettings) []TerminalCommandSettings {
	if commands == nil {
		return nil
	}
	result := make([]TerminalCommandSettings, len(commands))
	for index, command := range commands {
		command.ID = strings.TrimSpace(command.ID)
		command.Name = strings.TrimSpace(command.Name)
		command.Command = strings.TrimSpace(command.Command)
		result[index] = command
	}
	return result
}

// validateTerminalCommands rejects ambiguous or unusable writes before the
// complete settings file is committed. There is deliberately no item-count
// limit; each selected command is still parsed by the terminal runtime.
func validateTerminalCommands(commands []TerminalCommandSettings) error {
	seen := make(map[string]struct{}, len(commands))
	for index, command := range normalizeTerminalCommands(commands) {
		path := fmt.Sprintf("terminal_commands[%d]", index)
		if err := validateTerminalCommandID(command.ID); err != nil {
			return fmt.Errorf("%w: %s.id: %v", ErrInvalidTerminalCommand, path, err)
		}
		if _, duplicate := seen[command.ID]; duplicate {
			return fmt.Errorf("%w: %s.id %q is duplicated / %s.id %q 重复", ErrInvalidTerminalCommand, path, command.ID, path, command.ID)
		}
		seen[command.ID] = struct{}{}
		if command.Name == "" {
			return fmt.Errorf("%w: %s.name is required / %s.name 不能为空", ErrInvalidTerminalCommand, path, path)
		}
		if utf8.RuneCountInString(command.Name) > maxTerminalCommandNameRunes {
			return fmt.Errorf("%w: %s.name exceeds %d characters / %s.name 超过 %d 个字符", ErrInvalidTerminalCommand, path, maxTerminalCommandNameRunes, path, maxTerminalCommandNameRunes)
		}
		if command.Command == "" {
			return fmt.Errorf("%w: %s.command is required / %s.command 不能为空", ErrInvalidTerminalCommand, path, path)
		}
		if len(command.Command) > maxTerminalCommandLineBytes {
			return fmt.Errorf("%w: %s.command exceeds %d bytes / %s.command 超过 %d bytes", ErrInvalidTerminalCommand, path, maxTerminalCommandLineBytes, path, maxTerminalCommandLineBytes)
		}
	}
	return nil
}

func validateTerminalCommandID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("is required / 不能为空")
	}
	if id == "shell" || id == "custom" {
		return fmt.Errorf("%q is reserved / %q 是保留 ID", id, id)
	}
	if utf8.RuneCountInString(id) > maxTerminalCommandIDRunes {
		return fmt.Errorf("exceeds %d characters / 超过 %d 个字符", maxTerminalCommandIDRunes, maxTerminalCommandIDRunes)
	}
	for index, character := range id {
		isASCIILetter := character >= 'a' && character <= 'z'
		isASCIIDigit := character >= '0' && character <= '9'
		if isASCIILetter || isASCIIDigit || (index > 0 && (character == '-' || character == '_' || character == '.')) {
			continue
		}
		return errors.New("must use lowercase letters, numbers, '.', '_' or '-' / 只能使用小写字母、数字、点、下划线或短横线")
	}
	return nil
}

// migrateLegacyTerminalCommands maps the two former scalar settings into the
// shared registry. It is invoked on read and before merge, so the next normal
// settings save rewrites legacy files without losing either preset.
func migrateLegacyTerminalCommands(settings Settings) Settings {
	codex := strings.TrimSpace(settings.TerminalCodexCommand)
	claude := strings.TrimSpace(settings.TerminalClaudeCommand)
	if codex == "" && claude == "" {
		return settings
	}
	commands := settings.TerminalCommands
	if commands == nil {
		commands = DefaultTerminalCommands()
	} else {
		commands = normalizeTerminalCommands(commands)
	}
	if codex != "" {
		commands = upsertLegacyTerminalCommand(commands, TerminalCommandSettings{
			ID: TerminalCommandCodexID, Name: "Codex CLI", Command: codex, Enabled: true,
		})
	}
	if claude != "" {
		commands = upsertLegacyTerminalCommand(commands, TerminalCommandSettings{
			ID: TerminalCommandClaudeID, Name: "Claude Code", Command: claude, Enabled: true,
		})
	}
	settings.TerminalCommands = commands
	settings.TerminalCodexCommand = ""
	settings.TerminalClaudeCommand = ""
	return settings
}

func upsertLegacyTerminalCommand(commands []TerminalCommandSettings, replacement TerminalCommandSettings) []TerminalCommandSettings {
	for index := range commands {
		if strings.TrimSpace(commands[index].ID) == replacement.ID {
			commands[index] = replacement
			return commands
		}
	}
	return append(commands, replacement)
}
