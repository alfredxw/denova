package toolruntime

import (
	"denova/internal/agents/run"
	"fmt"
	"strings"

	agenttool "denova/internal/agents/tool"
)

func ApplyMutationWarnings(options agentrun.Options, verification agenttool.Verification, warnings []string) agenttool.Verification {
	verification.Warnings = append(verification.Warnings, warnings...)
	if strings.EqualFold(strings.TrimSpace(options.WriteMode), agentrun.WriteModeReadOnly) && verification.Mutations > 0 {
		verification.Warnings = append(verification.Warnings, fmt.Sprintf(
			"read_only run produced %d committed workspace mutation receipt(s); changes were retained",
			verification.Mutations,
		))
	}
	if len(verification.Warnings) > 0 && verification.Status != "warning" {
		verification.Status = "warning"
	}
	return verification
}

func EventDataStringSlice(data any, key string) []string {
	switch typed := data.(type) {
	case map[string]interface{}:
		value, ok := typed[key]
		if !ok {
			return nil
		}
		return anyToStringSlice(value)
	case map[string][]string:
		return typed[key]
	default:
		return nil
	}
}

func anyToStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}
