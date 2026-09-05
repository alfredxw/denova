package change

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEditDiagnosticsLocateAmbiguousMatches(t *testing.T) {
	for _, test := range []struct {
		name, base, old string
		locations       [][2]int
	}{
		{"repeated sections", "## First\n\nsame\n## Second\n\nsame\n## Third\n\nsame", "same", [][2]int{{3, 1}, {6, 1}}},
		{"same Unicode line", "头：same / same", "same", [][2]int{{1, 3}, {1, 10}}},
		{"multiline match", "same\nagain\n\nsame\nagain", "same\nagain", [][2]int{{1, 1}, {4, 1}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			after, applied, err := planTextEdits("state.md", test.base, []TextEdit{{OldString: test.old, NewString: "updated"}}, false)
			var changeErr *Error
			if !errors.As(err, &changeErr) || after != "" || applied != nil {
				t.Fatalf("ambiguous edit produced a plan: after=%q applied=%v err=%v", after, applied, err)
			}
			issue := changeErr.Details["issues"].([]editValidationIssue)[0]
			encoded, err := json.Marshal(issue.Details["matches"])
			if err != nil {
				t.Fatal(err)
			}
			var matches []struct {
				Line             int    `json:"line"`
				Column           int    `json:"column"`
				ContextStartLine int    `json:"context_start_line"`
				Context          string `json:"context"`
				ContextTruncated bool   `json:"context_truncated"`
			}
			if err := json.Unmarshal(encoded, &matches); err != nil {
				t.Fatal(err)
			}
			locations := make([][2]int, len(matches))
			for i, match := range matches {
				locations[i] = [2]int{match.Line, match.Column}
				if match.Context != test.base || match.ContextStartLine != 1 || match.ContextTruncated {
					t.Fatalf("short source context changed: %+v", match)
				}
			}
			if !reflect.DeepEqual(locations, test.locations) || issue.Details["match_count_at_least"] != 2 {
				t.Fatalf("match locations=%v details=%v", locations, issue.Details)
			}
			if suggestion, _ := issue.Details["suggestion"].(string); !strings.Contains(suggestion, "expand old_string") {
				t.Fatalf("missing disambiguation guidance: %v", issue.Details)
			}
		})
	}
}

func TestEditDiagnosticsBoundUnicodeContext(t *testing.T) {
	base := "Heading\n" + strings.Repeat("长", 500) + "needle" + strings.Repeat("行", 500) + "\nneedle"
	_, _, err := planTextEdits("state.md", base, []TextEdit{{OldString: "needle", NewString: "updated"}}, false)
	var changeErr *Error
	if !errors.As(err, &changeErr) {
		t.Fatal(err)
	}
	issue := changeErr.Details["issues"].([]editValidationIssue)[0]
	encoded, _ := json.Marshal(issue.Details["matches"])
	var matches []struct {
		Line             int    `json:"line"`
		ContextStartLine int    `json:"context_start_line"`
		Context          string `json:"context"`
		ContextTruncated bool   `json:"context_truncated"`
	}
	if err := json.Unmarshal(encoded, &matches); err != nil || len(matches) != 2 {
		t.Fatalf("missing bounded match contexts: %s err=%v", encoded, err)
	}
	for i, match := range matches {
		if match.Line != i+2 || match.ContextStartLine != 2 || !match.ContextTruncated ||
			len(match.Context) > 512 || !utf8.ValidString(match.Context) || !strings.Contains(match.Context, "needle") || !strings.Contains(base, match.Context) {
			t.Fatalf("invalid bounded source context: %+v", match)
		}
	}
}

func TestEditDiagnosticsExplainMissingTextWithoutChangingBatch(t *testing.T) {
	after, applied, err := planTextEdits("state.md", "existing text", []TextEdit{
		{OldString: "existing", NewString: "updated"},
		{OldString: "missing", NewString: "replacement"},
	}, false)
	var changeErr *Error
	if !errors.As(err, &changeErr) || after != "" || applied != nil || changeErr.Details["workspace_mutated"] != false {
		t.Fatalf("invalid batch produced a plan: after=%q applied=%v err=%v", after, applied, err)
	}
	issue := changeErr.Details["issues"].([]editValidationIssue)[0]
	suggestion, _ := issue.Details["suggestion"].(string)
	if issue.EditIndex != 1 || issue.Code != editIssueNotFound || !strings.Contains(suggestion, "Read the target file again") || !strings.Contains(suggestion, "whitespace") {
		t.Fatalf("missing exact-text recovery guidance: %+v", issue)
	}
	if !strings.Contains(err.Error(), `"state.md"`) || !strings.Contains(err.Error(), "edits[1]: old_string was not found") || !strings.Contains(err.Error(), "No changes applied") {
		t.Fatalf("plain error must locate the failed item and state the atomic outcome: %v", err)
	}
	after, applied, err = planTextEdits("state.md", "existing text", []TextEdit{
		{OldString: "existing", NewString: "updated"},
		{OldString: "text", NewString: "replacement"},
	}, false)
	if err != nil || after != "updated replacement" || len(applied) != 2 {
		t.Fatalf("corrected batch did not apply together: after=%q applied=%v err=%v", after, applied, err)
	}
}
