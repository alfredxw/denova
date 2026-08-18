package tools

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
)

type processDiagnostic struct {
	content string
	err     error
}

func readProcessDiagnostics(reader io.Reader, limit int) <-chan processDiagnostic {
	result := make(chan processDiagnostic, 1)
	go func() {
		diagnostic := processDiagnostic{}
		defer func() {
			if recovered := recover(); recovered != nil {
				diagnostic.err = fmt.Errorf("read process diagnostics panic: %v\n%s", recovered, debug.Stack())
			}
			result <- diagnostic
		}()
		content, _, err := readBoundedProcessOutput(reader, limit)
		diagnostic.content, diagnostic.err = content, err
	}()
	return result
}

func readBoundedProcessOutput(reader io.Reader, limit int) (string, bool, error) {
	limited := &io.LimitedReader{R: reader, N: int64(limit + 1)}
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", false, err
	}
	truncated := len(data) > limit
	content := strings.ToValidUTF8(string(data), "\uFFFD")
	if len(content) > limit {
		content = truncateUTF8(content, limit)
		truncated = true
	}
	if _, err := io.Copy(io.Discard, reader); err != nil {
		return content, truncated, err
	}
	return content, truncated, nil
}
