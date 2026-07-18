package automation

import (
	"strings"
	"testing"
)

func TestBuiltinTaskTemplatesProvideLocalizedWorkspaceDrafts(t *testing.T) {
	zh := BuiltinTaskTemplates("zh-CN")
	en := BuiltinTaskTemplates("en-US")
	if len(zh) != 2 || len(en) != 2 {
		t.Fatalf("template count zh=%d en=%d, want 2", len(zh), len(en))
	}

	continueWriting := templateByID(zh, TemplateContinueWriting)
	if continueWriting == nil {
		t.Fatal("continue-writing template missing")
	}
	if continueWriting.Defaults.Enabled || continueWriting.Defaults.WriteMode != WriteModeConfirmWrite || continueWriting.Defaults.WriteScope != WriteScopeFile {
		t.Fatalf("unexpected continue-writing defaults: %#v", continueWriting.Defaults)
	}
	if len(continueWriting.TargetKinds) != 1 || continueWriting.TargetKinds[0] != TargetKindWorkspace {
		t.Fatalf("continue-writing targets = %#v", continueWriting.TargetKinds)
	}
	if !strings.Contains(continueWriting.Defaults.Prompt, "续写下一章") {
		t.Fatalf("Chinese prompt not localized: %q", continueWriting.Defaults.Prompt)
	}

	review := templateByID(en, TemplateReview)
	if review == nil {
		t.Fatal("review template missing")
	}
	if review.Defaults.Name != "Automatic Review" || !strings.Contains(review.Defaults.Prompt, "new chapters") {
		t.Fatalf("English review template not localized: %#v", review)
	}
	if len(review.Defaults.Triggers) != 1 || review.Defaults.Triggers[0].Type != TriggerTypeChapterBatch || review.Defaults.Triggers[0].ChapterBatchSize != 5 {
		t.Fatalf("unexpected review trigger defaults: %#v", review.Defaults.Triggers)
	}
}

func templateByID(templates []TaskTemplate, id string) *TaskTemplate {
	for i := range templates {
		if templates[i].ID == id {
			return &templates[i]
		}
	}
	return nil
}
