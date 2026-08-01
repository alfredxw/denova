package compaction

import (
	"reflect"
	"testing"
)

func TestContextCompactionCheckpointRoundTripsEveryDurableField(t *testing.T) {
	checkpoint := NewCheckpoint("ide", Result{
		Phase: "mid_run", TriggerReason: "compaction_capacity_reserve",
		EstimatedTokensBefore: 1800, ObservedPromptTokens: 1900, ObservedEstimateTokens: 1700,
		TokensBefore: 2000, TokensAfter: 400, ContextWindowTokens: 2400,
		Strategy: "summary", Threshold: 0.85, RecoveryBand: 0.75,
		Epoch: 3, Summary: "bounded checkpoint", TargetRatio: 0.22, RetainedTurns: 2,
		CandidateFingerprint: "sha256:candidate", CandidateGeneration: 7,
	})

	value := reflect.ValueOf(checkpoint)
	typeOfCheckpoint := value.Type()
	for index := range value.NumField() {
		if value.Field(index).IsZero() {
			t.Fatalf("test fixture must populate durable field %s", typeOfCheckpoint.Field(index).Name)
		}
	}

	restored := ResultFromCheckpoint(checkpoint)
	if got := NewCheckpoint("ide", restored); got != checkpoint {
		t.Fatalf("checkpoint round trip changed durable fields:\ngot  %#v\nwant %#v", got, checkpoint)
	}
	wantRecoveryTarget := int(float64(checkpoint.ContextWindowTokens) * checkpoint.Threshold * checkpoint.RecoveryBand)
	if restored.RecoveryTargetTokens != wantRecoveryTarget || !restored.RecoveryBandMet || restored.Degraded {
		t.Fatalf("derived recovery state was not reconstructed: %#v", restored)
	}
}
