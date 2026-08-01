package compaction

import "strings"

// NormalizeTriggerReason supplies the stable durable reason when a planner or
// manual caller did not provide one explicitly.
func NormalizeTriggerReason(reason, phase string) string {
	if reason = strings.TrimSpace(reason); reason != "" {
		return reason
	}
	if strings.TrimSpace(phase) == "manual" {
		return "manual"
	}
	return reasonLimit
}
