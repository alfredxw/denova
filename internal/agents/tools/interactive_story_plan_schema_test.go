package tools

import (
	"context"
	"testing"

	"denova/internal/interactive"
)

func TestSubmitInteractiveTurnPlanUpdateSchemaUsesDocumentOrSectionVariants(t *testing.T) {
	tool, err := newSubmitInteractiveTurnTool("test", func(context.Context, interactive.TurnSubmissionInput) (interactive.TurnSubmissionReceipt, error) {
		return interactive.TurnSubmissionReceipt{}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	info, err := tool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schema, err := info.ParamsOneOf.ToJSONSchema()
	if err != nil {
		t.Fatal(err)
	}
	planUpdate, ok := schema.Properties.Get("plan_update")
	if !ok || planUpdate == nil || len(planUpdate.OneOf) != 2 {
		t.Fatalf("plan_update schema should expose two disjoint mutation modes: %#v", planUpdate)
	}
}
