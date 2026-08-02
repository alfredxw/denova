package change

import (
	"errors"
	"testing"
)

func TestPlanTextEditsReportsEveryIndependentItemIssue(t *testing.T) {
	result, applied, err := planTextEdits("ideas.md", "same same target", []TextEdit{
		{OldString: "", NewString: "value"},
		{OldString: "target", NewString: "target"},
		{OldString: "missing", NewString: "present"},
		{OldString: "same", NewString: "one"},
	}, false)
	if result != "" || applied != nil {
		t.Fatalf("invalid batch produced a plan: result=%q applied=%#v", result, applied)
	}
	var changeErr *Error
	if !errors.As(err, &changeErr) || changeErr.Code != ErrorCodeInvalidEdit {
		t.Fatalf("validation error = %#v", err)
	}
	if got := changeErr.Details["issue_count"]; got != 4 {
		t.Fatalf("issue_count = %#v", got)
	}
	issues, ok := changeErr.Details["issues"].([]editValidationIssue)
	if !ok || len(issues) != 4 {
		t.Fatalf("issues = %#v", changeErr.Details["issues"])
	}
	wantCodes := []string{
		editIssueEmptyOldString,
		editIssueNoChange,
		editIssueNotFound,
		editIssueNotUnique,
	}
	for index, issue := range issues {
		if issue.EditIndex != index || issue.Code != wantCodes[index] || issue.EditID == "" {
			t.Fatalf("issues[%d] = %#v", index, issue)
		}
	}
	if mutated, ok := changeErr.Details["workspace_mutated"].(bool); !ok || mutated {
		t.Fatalf("workspace_mutated = %#v", changeErr.Details["workspace_mutated"])
	}
}

func TestPlanTextEditsReportsEveryOverlapAgainstContainingRange(t *testing.T) {
	result, applied, err := planTextEdits("ideas.md", "abcdef", []TextEdit{
		{ID: "outer", OldString: "abcdef", NewString: "all"},
		{ID: "left", OldString: "bc", NewString: "BC"},
		{ID: "right", OldString: "de", NewString: "DE"},
	}, false)
	if result != "" || applied != nil {
		t.Fatalf("overlapping batch produced a plan: result=%q applied=%#v", result, applied)
	}
	var changeErr *Error
	if !errors.As(err, &changeErr) {
		t.Fatalf("overlap error = %#v", err)
	}
	issues, ok := changeErr.Details["issues"].([]editValidationIssue)
	if !ok || len(issues) != 2 {
		t.Fatalf("overlap issues = %#v", changeErr.Details["issues"])
	}
	for offset, issue := range issues {
		if issue.Code != editIssueOverlap || issue.EditIndex != offset+1 || issue.Details["other_edit_id"] != "outer" {
			t.Fatalf("overlap issues[%d] = %#v", offset, issue)
		}
	}
}
