package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func compactionValidationSnapshot(messages []*Message, stable int) *ModelRequestSnapshot {
	return (&ModelCall{
		Model: &lifecycleModel{}, Messages: messages,
		stablePrefixMessages: stable,
	}).Snapshot()
}

func TestCompactionPostValidationDistinguishesRecoveryBandAndHardPublishBand(t *testing.T) {
	before := compactionValidationSnapshot([]*Message{
		SystemMessage(strings.Repeat("stable ", 100)),
		UserMessage(strings.Repeat("old context ", 500)),
		AssistantMessage(strings.Repeat("old answer ", 500), nil),
		UserMessage("continue"),
	}, 1)
	degradedAfter := compactionValidationSnapshot([]*Message{
		SystemMessage(strings.Repeat("stable ", 100)),
		SystemMessage(strings.Repeat("checkpoint ", 210)),
		UserMessage("continue"),
	}, 2)
	afterTokens := estimateCompactionRequestTokens(degradedAfter.Messages(), nil)
	window := max(afterTokens+1, int(float64(afterTokens)/.80))
	plan := CompactionPlan{
		SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{
			ContextWindowTokens: window, Threshold: .90, RecoveryBand: .80,
			HardLimitBytes: 8 << 20,
		},
	}
	metrics, err := validateCompactionProjection(before, degradedAfter, plan)
	if err != nil || !metrics.Degraded || metrics.RecoveryBandMet ||
		metrics.ProjectedTokensAfter >= int(float64(window)*.90) {
		t.Fatalf("degraded metrics=%#v err=%v", metrics, err)
	}

	healthyAfter := compactionValidationSnapshot([]*Message{
		SystemMessage(strings.Repeat("stable ", 100)),
		SystemMessage("short checkpoint"),
		UserMessage("continue"),
	}, 2)
	healthy, err := validateCompactionProjection(before, healthyAfter, plan)
	if err != nil || healthy.Degraded || !healthy.RecoveryBandMet {
		t.Fatalf("healthy metrics=%#v err=%v", healthy, err)
	}

	hardPlan := plan
	hardPlan.Validation.ContextWindowTokens = max(1, afterTokens)
	if metrics, err := validateCompactionProjection(before, degradedAfter, hardPlan); !errors.Is(err, ErrContextLimit) || metrics.ProjectedTokensAfter < int(float64(afterTokens)*.90) {
		t.Fatalf("hard-band metrics=%#v err=%v", metrics, err)
	}
}

func TestCompactionPostValidationRejectsNoProgressAndInsignificantProgress(t *testing.T) {
	before := compactionValidationSnapshot([]*Message{UserMessage("small history")}, 0)
	larger := compactionValidationSnapshot([]*Message{SystemMessage(strings.Repeat("larger checkpoint ", 20))}, 1)
	plan := CompactionPlan{SourceFrom: 0, SourceTo: 1, Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20}}
	if _, err := validateCompactionProjection(before, larger, plan); err == nil || !strings.Contains(err.Error(), "no progress") {
		t.Fatalf("no-progress error = %v", err)
	}

	largeBefore := compactionValidationSnapshot([]*Message{UserMessage(strings.Repeat("history ", 100))}, 0)
	slightlySmaller := compactionValidationSnapshot([]*Message{SystemMessage(strings.Repeat("checkpoint ", 50))}, 1)
	progress := estimateCompactionRequestTokens(largeBefore.Messages(), nil) - estimateCompactionRequestTokens(slightlySmaller.Messages(), nil)
	plan.Validation.MinimumChangeTokens = progress + 1
	if _, err := validateCompactionProjection(largeBefore, slightlySmaller, plan); err == nil || !strings.Contains(err.Error(), "required minimum") {
		t.Fatalf("minimum-progress error = %v progress=%d", err, progress)
	}
}

func TestInteractiveCompactionCalibratesTruePostContextAfterStableReinjection(t *testing.T) {
	before := compactionValidationSnapshot([]*Message{
		UserMessage(strings.Repeat("original history ", 300)),
		AssistantMessage("previous answer", nil),
		UserMessage("continue"),
	}, 0)
	after := compactionValidationSnapshot([]*Message{
		UserMessage(strings.Repeat("resident lore remains exact ", 80)),
		SystemMessage("bounded checkpoint"),
		UserMessage("continue"),
	}, 2)
	localBefore := estimateCompactionRequestTokens(before.Messages(), nil)
	localAfter := estimateCompactionRequestTokens(after.Messages(), nil)
	const reserve = 78
	calibratedAfter := localAfter*2 + reserve
	window := (calibratedAfter*100 + 83) / 84
	plan := CompactionPlan{
		SourceFrom: 0, SourceTo: 2,
		Metrics: CompactionMetrics{
			ObservedPromptTokens: localBefore * 2, ObservedEstimateTokens: localBefore,
		},
		Validation: CompactionValidationPolicy{
			ContextWindowTokens: window, ReservedTokens: reserve,
			Threshold: .90, RecoveryBand: .80, HardLimitBytes: 8 << 20,
		},
	}
	metrics, err := validateCompactionProjection(before, after, plan)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.EstimatedTokensAfter != localAfter || metrics.ProjectedTokensAfter != calibratedAfter {
		t.Fatalf("post-Compaction calibration local=%d calibrated=%d metrics=%#v", localAfter, calibratedAfter, metrics)
	}
	if metrics.RecoveryBandMet || !metrics.Degraded {
		t.Fatalf("provider-calibrated ~84%% projection was misclassified: %#v", metrics)
	}
}

type expandingCompactionManager struct{}

func (expandingCompactionManager) Identity() CapabilityIdentity {
	return CapabilityIdentity{Kind: "compaction.expanding-post-validation-test", Version: 1}
}

func (expandingCompactionManager) SummaryLimitBytes() int { return 64 << 10 }

func (expandingCompactionManager) Plan(_ context.Context, request CompactionPlanRequest) (CompactionPlan, error) {
	if len(request.Messages) < 2 {
		return CompactionPlan{Action: CompactionNone}, nil
	}
	return CompactionPlan{
		Action: CompactionCreate, SourceFrom: 0, SourceTo: 2,
		Validation: CompactionValidationPolicy{HardLimitBytes: 8 << 20},
	}, nil
}

func (expandingCompactionManager) Compact(context.Context, CompactionCompactRequest) (CompactionCheckpoint, error) {
	return CompactionCheckpoint{Summary: strings.Repeat("expanded checkpoint ", 500), TokenEstimate: 2_000}, nil
}

func TestAutomaticAndManualCompactionRejectUnpublishablePostProjection(t *testing.T) {
	model := &lifecycleModel{responses: []*Message{
		AssistantMessage("first answer", nil), AssistantMessage("second answer", nil),
	}}
	owner, err := New(context.Background(), Definition{Model: model, Compaction: expandingCompactionManager{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Close(context.Background()) })
	session, err := owner.Session(context.Background(), NamedSession("post-compaction-validation"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := session.Run(context.Background(), Input{Text: "first", IdempotencyKey: "post-validation-first"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := first.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("first=%#v err=%v", result, waitErr)
	}
	second, err := session.Run(context.Background(), Input{Text: "second", IdempotencyKey: "post-validation-second"})
	if err != nil {
		t.Fatal(err)
	}
	if result, waitErr := second.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
		t.Fatalf("second=%#v err=%v", result, waitErr)
	}
	foundFailure := false
	for event := range second.Events() {
		if failure, ok := event.Payload.(CompactionFailed); ok {
			foundFailure = true
			if failure.Metrics.ProjectedTokensAfter <= failure.Metrics.ProjectedTokensBefore {
				t.Fatalf("failure metrics did not describe rejected post-projection: %#v", failure)
			}
		}
	}
	if !foundFailure || len(model.calls()) != 2 {
		t.Fatalf("automatic failure=%v provider calls=%d", foundFailure, len(model.calls()))
	}
	if state, present, stateErr := session.compactionState(context.Background()); stateErr != nil || present {
		t.Fatalf("rejected automatic Compaction became durable: %#v present=%v err=%v", state, present, stateErr)
	}

	if result, compactErr := session.Compact(context.Background(), CompactionRequest{
		Force: true, IdempotencyKey: "post-validation-manual",
	}); compactErr == nil || result.Changed || !strings.Contains(compactErr.Error(), "no progress") {
		t.Fatalf("manual rejected Compaction=%#v err=%v", result, compactErr)
	}
	if state, present, stateErr := session.compactionState(context.Background()); stateErr != nil || present {
		t.Fatalf("rejected manual Compaction became durable: %#v present=%v err=%v", state, present, stateErr)
	}
}
