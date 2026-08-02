package change

import (
	"fmt"
	"sort"
	"strings"
)

const maxWorkspaceMutationReportedIssues = maxWorkspaceMutationEdits

const (
	editIssueDuplicateID           = "duplicate_id"
	editIssueEmptyOldString        = "empty_old_string"
	editIssueNoChange              = "no_change"
	editIssueFragmentTooLarge      = "fragment_too_large"
	editIssueScanLimit             = "scan_limit"
	editIssueNotFound              = "not_found"
	editIssueNotUnique             = "not_unique"
	editIssueReplacementLimit      = "replacement_limit"
	editIssueTotalReplacementLimit = "total_replacement_limit"
	editIssueOverlap               = "overlap"
)

type plannedSpan struct {
	editIndex  int
	start      int
	end        int
	afterStart int
	afterEnd   int
}

type editValidationIssue struct {
	EditIndex int            `json:"edit_index"`
	EditID    string         `json:"edit_id,omitempty"`
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Details   map[string]any `json:"details,omitempty"`
}

type editValidationIssues struct {
	items []editValidationIssue
	total int
}

func (issues *editValidationIssues) add(issue editValidationIssue) {
	issues.total++
	if len(issues.items) < maxWorkspaceMutationReportedIssues {
		issues.items = append(issues.items, issue)
	}
}

func (issues editValidationIssues) err(path string) *Error {
	if issues.total == 0 {
		return nil
	}
	details := map[string]any{
		"path":              path,
		"issues":            issues.items,
		"issue_count":       issues.total,
		"workspace_mutated": false,
	}
	if issues.total > len(issues.items) {
		details["issues_truncated"] = true
	}
	return newError(
		ErrorCodeInvalidEdit,
		fmt.Sprintf("%d edit item issue(s) found; first issue: %s", issues.total, issues.items[0].Message),
		details,
	)
}

func planTextEdits(path, base string, requested []TextEdit, autoAccept bool) (string, []AppliedEdit, error) {
	if len(requested) == 0 {
		return "", nil, newError(ErrorCodeInvalidEdit, "at least one edit is required", map[string]any{
			"path": path, "workspace_mutated": false,
		})
	}
	if len(base) > maxWorkspaceMutationFileBytes {
		return "", nil, newError(ErrorCodeInvalidEdit, "workspace file exceeds the mutation limit", map[string]any{
			"path": path, "max_bytes": maxWorkspaceMutationFileBytes, "workspace_mutated": false,
		})
	}
	if len(requested) > maxWorkspaceMutationEdits {
		return "", nil, newError(ErrorCodeInvalidEdit, "too many edits in one mutation", map[string]any{
			"path": path, "max_edits": maxWorkspaceMutationEdits, "workspace_mutated": false,
		})
	}

	reviewStatus := ReviewStatusPending
	if autoAccept {
		reviewStatus = ReviewStatusAccepted
	}
	applied := make([]AppliedEdit, len(requested))
	spans := make([]plannedSpan, 0, len(requested))
	seenIDs := make(map[string]int, len(requested))
	issues := editValidationIssues{}
	scannedBytes := 0

	for index, edit := range requested {
		editID := strings.TrimSpace(edit.ID)
		if editID == "" {
			editID = newID("edit")
		}
		applied[index] = AppliedEdit{
			ID: editID, OldString: edit.OldString, NewString: edit.NewString,
			ReplaceAll: edit.ReplaceAll, ReviewStatus: reviewStatus,
		}
		if otherIndex, exists := seenIDs[editID]; exists {
			issues.add(editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueDuplicateID, Message: "duplicate edit id",
				Details: map[string]any{"other_edit_index": otherIndex},
			})
			continue
		}
		seenIDs[editID] = index

		var issue *editValidationIssue
		switch {
		case edit.OldString == "":
			issue = &editValidationIssue{EditIndex: index, EditID: editID, Code: editIssueEmptyOldString, Message: "old_string must not be empty"}
		case edit.OldString == edit.NewString:
			issue = &editValidationIssue{EditIndex: index, EditID: editID, Code: editIssueNoChange, Message: "new_string must differ from old_string"}
		case len(edit.OldString) > maxWorkspaceMutationFragmentBytes || len(edit.NewString) > maxWorkspaceMutationFragmentBytes:
			issue = &editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueFragmentTooLarge, Message: "edit text exceeds the mutation fragment limit",
				Details: map[string]any{"max_bytes": maxWorkspaceMutationFragmentBytes},
			}
		case len(base) > maxWorkspaceMutationScanBytes-scannedBytes:
			issue = &editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueScanLimit, Message: "combined edit search exceeds the mutation scan limit",
				Details: map[string]any{"max_scan_bytes": maxWorkspaceMutationScanBytes},
			}
		}
		if issue != nil {
			issues.add(*issue)
			continue
		}

		scannedBytes += len(base)
		matchLimit := 2
		if edit.ReplaceAll {
			matchLimit = maxWorkspaceMutationReplacements + 1
		}
		matches := literalMatches(base, edit.OldString, matchLimit)
		switch {
		case len(matches) == 0:
			issue = &editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueNotFound, Message: "old_string was not found",
				Details: map[string]any{"match_count": 0},
			}
		case len(matches) > 1 && !edit.ReplaceAll:
			issue = &editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueNotUnique, Message: "old_string is not unique",
				Details: map[string]any{"match_count_at_least": len(matches)},
			}
		case edit.ReplaceAll && len(matches) > maxWorkspaceMutationReplacements:
			issue = &editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueReplacementLimit, Message: "replace_all exceeds the replacement limit",
				Details: map[string]any{"max_replacements": maxWorkspaceMutationReplacements, "match_count_at_least": len(matches)},
			}
		}
		if issue != nil {
			issues.add(*issue)
			continue
		}
		if !edit.ReplaceAll {
			matches = matches[:1]
		}
		if len(spans)+len(matches) > maxWorkspaceMutationReplacements {
			issues.add(editValidationIssue{
				EditIndex: index, EditID: editID, Code: editIssueTotalReplacementLimit, Message: "mutation exceeds the total replacement limit",
				Details: map[string]any{
					"max_replacements":     maxWorkspaceMutationReplacements,
					"planned_replacements": len(spans), "requested_replacements": len(matches),
				},
			})
			continue
		}
		for _, start := range matches {
			spans = append(spans, plannedSpan{editIndex: index, start: start, end: start + len(edit.OldString)})
		}
	}

	sort.SliceStable(spans, func(i, j int) bool {
		if spans[i].start == spans[j].start {
			return spans[i].end < spans[j].end
		}
		return spans[i].start < spans[j].start
	})
	if len(spans) > 1 {
		furthest := spans[0]
		for _, span := range spans[1:] {
			if span.start < furthest.end {
				issues.add(editValidationIssue{
					EditIndex: span.editIndex, EditID: applied[span.editIndex].ID,
					Code: editIssueOverlap, Message: "edit ranges overlap",
					Details: map[string]any{
						"other_edit_index": furthest.editIndex,
						"other_edit_id":    applied[furthest.editIndex].ID,
					},
				})
			}
			if span.end > furthest.end {
				furthest = span
			}
		}
	}
	if err := issues.err(path); err != nil {
		return "", nil, err
	}

	resultBytes := int64(len(base))
	delta := int64(0)
	for index := range spans {
		span := &spans[index]
		newText := applied[span.editIndex].NewString
		change := int64(len(newText) - (span.end - span.start))
		resultBytes += change
		if resultBytes < 0 || resultBytes > maxWorkspaceMutationFileBytes {
			return "", nil, newError(ErrorCodeInvalidEdit, "edited file exceeds the workspace mutation file limit", map[string]any{
				"path": path, "max_bytes": maxWorkspaceMutationFileBytes, "workspace_mutated": false,
			})
		}
		span.afterStart = span.start + int(delta)
		span.afterEnd = span.afterStart + len(newText)
		delta += change
		applied[span.editIndex].Hunks = append(applied[span.editIndex].Hunks, Hunk{
			ID:          newID("hunk"),
			BeforeStart: span.start,
			BeforeEnd:   span.end,
			AfterStart:  span.afterStart,
			AfterEnd:    span.afterEnd,
		})
	}
	var result strings.Builder
	result.Grow(int(resultBytes))
	cursor := 0
	for _, span := range spans {
		result.WriteString(base[cursor:span.start])
		result.WriteString(applied[span.editIndex].NewString)
		cursor = span.end
	}
	result.WriteString(base[cursor:])
	return result.String(), applied, nil
}

func literalMatches(content, needle string, limit int) []int {
	matches := make([]int, 0, min(limit, 2))
	for offset := 0; offset <= len(content)-len(needle); {
		index := strings.Index(content[offset:], needle)
		if index < 0 {
			break
		}
		start := offset + index
		matches = append(matches, start)
		if len(matches) >= limit {
			break
		}
		offset = start + len(needle)
	}
	return matches
}
