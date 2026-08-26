package agentrun

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunTraceVisitor receives each valid persisted record in chronological order.
// The zero-based index counts valid records only, matching RunTrace record order.
type RunTraceVisitor func(index int, record RunTraceRecord) error

// ScanRunTrace visits every persisted record without applying the bounded UI
// head/tail projection. It is intended for read-time projections that need a
// complete run while retaining the original JSONL file as the source of truth.
func ScanRunTrace(location TraceLocation, id string, visit RunTraceVisitor) (RunTraceSummary, int, error) {
	path, err := resolveRunTracePath(location, id)
	if err != nil {
		return RunTraceSummary{}, 0, err
	}
	return scanRunTraceFile(path, visit)
}

func scanRunTraceFile(path string, visit RunTraceVisitor) (RunTraceSummary, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return RunTraceSummary{}, 0, err
	}
	defer file.Close()

	summary := RunTraceSummary{}
	total := 0
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRunTraceRecordBytes)
	for scanner.Scan() {
		var record RunTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		updateRunTraceSummary(&summary, record, path)
		if visit != nil {
			if err := visit(total, record); err != nil {
				return RunTraceSummary{}, 0, fmt.Errorf("visit run trace record %d: %w", total, err)
			}
		}
		total++
	}
	if err := scanner.Err(); err != nil {
		return RunTraceSummary{}, 0, err
	}
	if summary.ID == "" {
		summary.ID = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if summary.Path == "" {
		summary.Path = path
	}
	return summary, total, nil
}
