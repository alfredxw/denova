package trajectory

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	agenttools "github.com/alfredxw/denova/agent/tools"
)

// trajectoryLineResult projects an already selected contiguous JSONL window.
// Session and Run resources share the read tool's exact UTF-8 continuation
// contract; total counts lines in the complete resource, including its manifest.
func trajectoryLineResult(resource, kind string, input readInput, lines []string, total int) (agenttools.ReadResult, error) {
	offset := max(1, input.Offset)
	if input.ByteOffset != 0 {
		if input.ByteOffset < 0 || len(lines) == 0 || input.ByteOffset >= len(lines[0]) || !utf8.ValidString(lines[0][input.ByteOffset:]) {
			return agenttools.ReadResult{}, errors.New("trajectory byte_offset must point inside the selected line at a UTF-8 boundary")
		}
		lines[0] = lines[0][input.ByteOffset:]
	}
	truncated := offset-1+len(lines) < total
	nextOffset := 0
	if truncated {
		nextOffset = offset + len(lines)
	}
	return agenttools.ReadResult{
		Path: resource, Kind: kind, Content: strings.Join(lines, ""),
		Offset: offset, ByteOffset: input.ByteOffset, Limit: len(lines), Total: total, Unit: "lines",
		Truncated: truncated, NextOffset: nextOffset,
	}, nil
}

func marshalJSONLine(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

func marshalRedactedJSONLine(value any, source Source) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		return "", err
	}
	return marshalJSONLine(redactTrajectoryValue(document, source))
}
