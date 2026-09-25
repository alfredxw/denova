package observability

import (
	"maps"
	"net/url"
	"regexp"
	"runtime"
	"strings"

	"denova/internal/buildinfo"
)

var diagnosticRedactions = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(?:bearer|basic)\s+[^\s,;"']+`),
	regexp.MustCompile(`(?i)(["']?(?:authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|secret|cookie)["']?\s*[:=]\s*)(?:"[^"\n]*"|'[^'\n]*'|[^\s,;}]+)`),
	regexp.MustCompile(`(?i)(["']?(?:prompt|messages|request_body|response_body|body)["']?\s*[:=]\s*)[\s\S]*`),
}
var diagnosticURL = regexp.MustCompile(`https?://[^\s<>"']+`)
var diagnosticPath = regexp.MustCompile(`(?:[A-Za-z]:\\|/(?:Users|home|private|var|tmp)/)[^\s"':;]+`)

// DiagnosticText is a bounded support summary, never a request/response dump.
// Keep full errors in their owning logs; credentials and user content do not
// belong in screenshots or copied reports.
func DiagnosticText(value string) string {
	value = diagnosticURL.ReplaceAllStringFunc(value, func(raw string) string {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "[url]"
		}
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		return parsed.String()
	})
	for _, pattern := range diagnosticRedactions {
		value = pattern.ReplaceAllString(value, "${1}[redacted]")
	}
	value = diagnosticPath.ReplaceAllString(value, "[local-path]")
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 2048 {
		return string(runes[:2048]) + "…"
	}
	return string(runes)
}

func ErrorCause(err error) string {
	if err == nil {
		return ""
	}
	return DiagnosticText(err.Error())
}

// EnrichError adds screenshot diagnostics to an existing transport envelope.
// Domain codes and details stay authoritative. No new recovery state is created.
func EnrichError(payload map[string]any, operation string) {
	details, _ := payload["details"].(map[string]any)
	details = maps.Clone(details)
	if details == nil {
		details = map[string]any{}
	}
	if details["operation"] == nil {
		details["operation"] = operation
	}
	if details["backend_version"] == nil {
		details["backend_version"] = buildinfo.Version
	}
	if details["platform"] == nil {
		details["platform"] = runtime.GOOS + "/" + runtime.GOARCH
	}
	for _, key := range []string{"detail", "reason"} {
		if value, ok := details[key].(string); ok {
			details[key] = DiagnosticText(value)
		}
	}
	for _, key := range []string{"error", "message"} {
		if value, ok := payload[key].(string); ok {
			payload[key] = DiagnosticText(value)
		}
	}
	payload["details"] = details
}
