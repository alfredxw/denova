package agents

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	agent "github.com/alfredxw/denova/agent"
	agentcontext "github.com/alfredxw/denova/agent/context"

	"denova/config"
	"denova/internal/agents/session"
)

func TestCompactMessagesPreservesNativeSystemDeveloperAndLeadingPrefixOrder(t *testing.T) {
	system := agent.SystemMessage("native system")
	developer := &agent.Message{Role: agent.RoleType("developer"), Content: "developer policy"}
	leading := agent.UserMessage("resident lore")
	leading.Extra = map[string]any{agentcontext.MessageExtraPlacement: string(agentcontext.PlacementLeadingMessage)}
	messages := []*agent.Message{
		system, developer, leading,
		agent.UserMessage("old request"), agent.AssistantMessage("old answer", nil),
		agent.UserMessage("latest request"),
	}

	compacted := compactMessagesForModel(messages, "checkpoint", 2, 1)
	if len(compacted) < 5 || compacted[0] != system || compacted[1] != developer || compacted[2] != leading {
		t.Fatalf("protected prefix reordered or lost: %#v", compacted)
	}
	if !IsContextCompactionSummaryMessage(compacted[3]) || compacted[len(compacted)-1].Content != "latest request" {
		t.Fatalf("compacted body is wrong: %#v", compacted)
	}
}

func TestCompactedProjectionMatchesDurableTailAcrossSourceBoundary(t *testing.T) {
	hugeNthOldTurn := strings.Repeat("fact at the Nth retained old turn ", 700)
	messages := []*agent.Message{
		agent.UserMessage("old turn one"),
		agent.AssistantMessage("old answer one", nil),
		agent.UserMessage(hugeNthOldTurn),
		agent.AssistantMessage("old answer two", nil),
		agent.UserMessage("current user remains outside the source boundary"),
	}

	transient, payload := compactMessagesForModelThroughSource(messages, "checkpoint", "", 1, 1, 4)
	durableTail := compactedMessagesAfterSource(messages, 0, 4, 1)
	durable := append([]*agent.Message{NewContextCompactionSummaryMessage(1, payload)}, durableTail...)
	if !reflect.DeepEqual(transient, durable) {
		t.Fatalf("transient/durable projections differ:\ntransient=%#v\ndurable=%#v", transient, durable)
	}
	if EstimateContextTokens(transient, nil) != EstimateContextTokens(durable, nil) {
		t.Fatalf("projection token counts differ: transient=%d durable=%d", EstimateContextTokens(transient, nil), EstimateContextTokens(durable, nil))
	}
	if !containsMessageContent(transient[1:], hugeNthOldTurn) || containsMessageContent(transient[1:], "old turn one") ||
		!containsMessageContent(transient[1:], "current user remains outside") {
		t.Fatalf("source tail or appended current input was lost: %#v", messageContents(transient))
	}
}

func TestTwoCompactionsKeepFactAfterRetainedTurnAgesOut(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	const retainedOnlyFact = "RETAINED-ONLY-FACT: the brass key opens the observatory"
	calls := 0
	summarizeContextForCompaction = func(
		_ context.Context,
		_ *config.Config,
		_ string,
		existingCheckpoint string,
		source []*agent.Message,
		_ string,
		_ int,
		_ contextCompactionPolicy,
		_ func(int, string),
	) (string, int, error) {
		calls++
		switch calls {
		case 1:
			if !containsMessageContent(source, retainedOnlyFact) {
				t.Fatalf("first checkpoint source omitted the retained-tail fact: %#v", messageContents(source))
			}
			return "## Confirmed facts and sources\n" + retainedOnlyFact, 1000, nil
		case 2:
			if !strings.Contains(existingCheckpoint, retainedOnlyFact) {
				t.Fatalf("second checkpoint did not receive the prior durable fact: %q", existingCheckpoint)
			}
			return existingCheckpoint + "\n\n## Current state\nA new turn completed.", 1000, nil
		default:
			t.Fatalf("unexpected summary call %d", calls)
			return "", 0, nil
		}
	}
	cfg := &config.Config{}
	firstInput := []*agent.Message{
		agent.UserMessage(strings.Repeat("first old turn ", 900)),
		agent.AssistantMessage(strings.Repeat("first old answer ", 900), nil),
		agent.UserMessage(retainedOnlyFact + "\n" + strings.Repeat("second old turn ", 900)),
		agent.AssistantMessage(strings.Repeat("second old answer ", 900), nil),
	}
	firstProjection, firstResult, err := PrepareContextCompaction(contextCompactionColdTestContext(), cfg, config.AgentKindIDE, ContextCompactionInput{
		Messages: firstInput, SourceMessages: firstInput, Force: true, KeepLatestUser: true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	newTurn := []*agent.Message{
		agent.UserMessage(strings.Repeat("new third turn ", 900)),
		agent.AssistantMessage(strings.Repeat("new third answer ", 900), nil),
	}
	secondInput := append(append([]*agent.Message(nil), firstProjection...), newTurn...)
	secondProjection, secondResult, err := PrepareContextCompaction(contextCompactionColdTestContext(), cfg, config.AgentKindIDE, ContextCompactionInput{
		Messages: secondInput, SourceMessages: newTurn, ExistingCheckpoint: firstResult.Summary,
		Force: true, KeepLatestUser: true,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || !strings.Contains(secondResult.Summary, retainedOnlyFact) {
		t.Fatalf("fact did not survive checkpoint merge: calls=%d summary=%q", calls, secondResult.Summary)
	}
	if len(secondProjection) < 2 || containsMessageContent(secondProjection[1:], retainedOnlyFact) {
		t.Fatalf("retained old turn did not age out of the verbatim tail: %#v", messageContents(secondProjection))
	}
	prompt := buildCacheSafeCompactionPrompt(
		contextCompactionPolicy{AgentKind: config.AgentKindIDE, RetainedTurns: 1}, "", "", 100, 1000,
		[]int{0, 1}, nil,
	)
	if !strings.Contains(prompt, "entire canonical source range, including that retained tail") ||
		strings.Contains(prompt, "without duplicating that retained tail") {
		t.Fatalf("checkpoint prompt does not cover facts marked by SourceEnd:\n%s", prompt)
	}
}

func TestCompactMessagesReinjectsSanitizedProtectedArtifactReceipt(t *testing.T) {
	result := agent.ToolErrorResult("SECRET RAW RESULT", "display-only diagnostic")
	result.Artifacts = []agent.ToolArtifactRef{{
		ID: "artifact-1", ReadablePath: ".denova/artifacts/session/call-old.log",
		ContentType: "text/plain", EstimatedBytes: 42_000, EstimatedTokens: 10_500, Complete: true,
		SHA256: strings.Repeat("a", 64),
	}}
	decision := processorTestDecision(agent.ToolResultProtected)
	decision.ProviderCallID = "call-old"
	processed, err := processToolResult(
		context.Background(), decision,
		`{"path":"lore/cast.md","authorization":"Bearer do-not-persist"}`,
		result, processorTestProcessingPolicy(16*1024),
	)
	if err != nil {
		t.Fatal(err)
	}
	if processed.ProtectedReceipt == nil {
		t.Fatal("live processor omitted the protected continuity receipt")
	}
	toolMessage := agent.ToolMessage(processed, "call-old", agent.WithToolName("read"))
	messages := []*agent.Message{
		agent.SystemMessage("system"),
		agent.UserMessage("old request"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-old", Type: "function", Function: agent.FunctionCall{Name: "read"}}}),
		toolMessage,
		agent.AssistantMessage("old answer", nil),
		agent.UserMessage("latest request"),
	}

	compacted := compactMessagesForModel(messages, "checkpoint", 1, 1)
	checkpoint := compacted[1].Content
	for _, expected := range []string{"call-old", "lore/cast.md", ".denova/artifacts/session/call-old.log", "outcome_receipt", "[redacted from retained tool context]"} {
		if !strings.Contains(checkpoint, expected) {
			t.Fatalf("protected receipt missing %q: %s", expected, checkpoint)
		}
	}
	if strings.Contains(checkpoint, "SECRET RAW RESULT") || strings.Contains(checkpoint, "do-not-persist") ||
		strings.Contains(checkpoint, strings.Repeat("a", 64)) || strings.Contains(checkpoint, `"sha256"`) {
		t.Fatalf("raw protected result leaked into receipt context: %s", checkpoint)
	}
	if roundTrip := toolMessage.EffectiveToolResult(); roundTrip.ProtectedReceipt == nil ||
		roundTrip.ProtectedReceipt.Outcome != processed.ProtectedReceipt.Outcome {
		t.Fatalf("protected receipt did not survive processor -> message round trip: %#v", roundTrip.ProtectedReceipt)
	}
	if _, err := NormalizeModelContextMessages(compacted); err != nil {
		t.Fatalf("re-injected checkpoint broke provider protocol: %v", err)
	}
}

func TestProtectedWorkspaceReceiptOmitsRevisionMaterial(t *testing.T) {
	result := agent.TextToolResult("workspace change applied")
	result.Details = json.RawMessage(`{"schema":"workspace_change.tool_result.v1","status":"applied","workspace":"/workspace/book-a","change_group_id":"group-1","review_thread_id":"review-1","change_set_id":"change-1","path":"chapters/ch01.md","base_revision":"sha256:before-secret","revision":"sha256:after-secret","review_status":"pending","apply_state":"applied"}`)
	manifest := ToolManifest{Name: "write", ToolDescriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace,
		Recovery: agent.ToolRecoveryReconcilable, ResultRetention: agent.ToolResultProtected,
		MaxResultBytes: 16 * 1024,
	}}
	projected := projectProtectedToolResultReceipt(manifest, `{"path":"chapters/ch01.md"}`, result)
	if projected.ProtectedReceipt == nil {
		t.Fatal("workspace mutation did not produce a protected receipt")
	}
	messages := []*agent.Message{
		agent.UserMessage("update the chapter"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-write", Type: "function", Function: agent.FunctionCall{Name: "write"}}}),
		agent.ToolMessage(projected, "call-write", agent.WithToolName("write")),
		agent.AssistantMessage("updated", nil),
		agent.UserMessage("continue"),
	}
	checkpoint := compactMessagesForModel(messages, "checkpoint", 1, 1)[0].Content
	for _, forbidden := range []string{"base_revision", "before-secret", "after-secret", `"revision"`} {
		if strings.Contains(checkpoint, forbidden) {
			t.Fatalf("workspace revision material leaked into checkpoint (%q): %s", forbidden, checkpoint)
		}
	}
	for _, required := range []string{"change-1", "group-1", "chapters/ch01.md"} {
		if !strings.Contains(checkpoint, required) {
			t.Fatalf("workspace continuity identity missing %q: %s", required, checkpoint)
		}
	}
}

func TestProtectedReceiptRedactsSensitiveTargetAcrossModelCompaction(t *testing.T) {
	const secret = "do-not-retain-987"
	sensitiveTarget := "https://example.test/docs?access_token=" + secret + "&view=full"
	result := agent.TextToolResult("page opened")
	result.Metadata.Target = sensitiveTarget
	manifest := ToolManifest{Name: "browser", ToolDescriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWeb, Recovery: agent.ToolRecoveryReadOnly,
		ResultRetention: agent.ToolResultProtected, MaxResultBytes: 16 * 1024,
	}}

	projected := projectProtectedToolResultReceipt(manifest, `{}`, result)
	if projected.ProtectedReceipt == nil {
		t.Fatal("protected web result did not produce a receipt")
	}
	if projected.Metadata.Target != sensitiveTarget {
		t.Fatalf("live result target changed: %q", projected.Metadata.Target)
	}
	if !json.Valid([]byte(projected.ProtectedReceipt.Outcome)) ||
		strings.Contains(projected.ProtectedReceipt.Outcome, secret) ||
		strings.Contains(projected.ProtectedReceipt.Outcome, sensitiveTarget) {
		t.Fatalf("sensitive target leaked into receipt JSON: %s", projected.ProtectedReceipt.Outcome)
	}
	var receipt retainedToolReceipt
	if err := json.Unmarshal([]byte(projected.ProtectedReceipt.Outcome), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Target != retainedContextRedactedValue {
		t.Fatalf("receipt target = %q, want redaction marker", receipt.Target)
	}

	messages := []*agent.Message{
		agent.UserMessage("inspect the page"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-browser", Type: "function",
			Function: agent.FunctionCall{Name: "browser", Arguments: `{}`},
		}}),
		agent.ToolMessage(projected, "call-browser", agent.WithToolName("browser")),
		agent.AssistantMessage("page inspected", nil),
		agent.UserMessage("continue"),
	}
	compacted := compactMessagesForModel(messages, "checkpoint", 1, 1)
	modelContext := strings.Join(messageContents(compacted), "\n")
	if strings.Contains(modelContext, secret) || strings.Contains(modelContext, sensitiveTarget) ||
		!strings.Contains(modelContext, retainedContextRedactedValue) {
		t.Fatalf("sensitive target crossed the model-context boundary: %s", modelContext)
	}
}

func TestProtectedReceiptKeepsNonSensitiveTargetWithinFieldBudget(t *testing.T) {
	longTarget := "https://example.test/resources/" + strings.Repeat("章节/", 2_000) + "TAIL_SENTINEL"
	result := agent.TextToolResult("page opened")
	result.Metadata.Target = longTarget
	manifest := ToolManifest{Name: "browser", ToolDescriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWeb, Recovery: agent.ToolRecoveryReadOnly,
		ResultRetention: agent.ToolResultProtected, MaxResultBytes: 16 * 1024,
	}}

	projected := projectProtectedToolResultReceipt(manifest, `{}`, result)
	if projected.ProtectedReceipt == nil {
		t.Fatal("protected web result did not produce a receipt")
	}
	var receipt retainedToolReceipt
	if err := json.Unmarshal([]byte(projected.ProtectedReceipt.Outcome), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Target) > retainedContextStringBytes || !strings.HasPrefix(receipt.Target, "https://example.test/resources/") ||
		!strings.HasSuffix(receipt.Target, retainedTargetTruncationMarker) || strings.Contains(receipt.Target, "TAIL_SENTINEL") {
		t.Fatalf("non-sensitive target was not usefully bounded: bytes=%d target=%q", len(receipt.Target), receipt.Target)
	}
	if !utf8.ValidString(receipt.Target) {
		t.Fatalf("bounded target is not valid UTF-8: %q", receipt.Target)
	}
	if projected.Metadata.Target != longTarget {
		t.Fatal("receipt projection mutated the live result target")
	}
}

func TestProtectedReceiptOmitsSensitiveAttachmentPathAcrossModelCompaction(t *testing.T) {
	const secret = "artifact-secret-321"
	sensitivePath := ".denova/attachments/export-token=" + secret + ".txt"
	hostPath := ".denova/artifacts/scope-" + strings.Repeat("a", 32) + "/call-" + strings.Repeat("b", 32) + ".log"
	result := agent.TextToolResult("export completed")
	result.Artifacts = []agent.ToolArtifactRef{
		{
			ID: "sensitive-attachment", Purpose: agent.ToolArtifactPurposeAttachment,
			ReadablePath: sensitivePath, ContentType: "text/plain", EstimatedBytes: 128, Complete: true,
		},
		{
			ID: "host-attachment", Purpose: agent.ToolArtifactPurposeAttachment,
			ReadablePath: hostPath, ContentType: "text/plain", EstimatedBytes: 256, Complete: true,
		},
	}
	manifest := ToolManifest{Name: "export", ToolDescriptor: agent.ToolDescriptor{
		Source: agent.ToolSourceWrite, MutationScope: agent.ToolMutationWorkspace,
		Recovery: agent.ToolRecoveryReconcilable, ResultRetention: agent.ToolResultProtected,
		MaxResultBytes: 16 * 1024,
	}}

	projected := projectProtectedToolResultReceipt(manifest, `{}`, result)
	if projected.ProtectedReceipt == nil {
		t.Fatal("artifact-backed result did not produce a protected receipt")
	}
	var receipt retainedToolReceipt
	if err := json.Unmarshal([]byte(projected.ProtectedReceipt.Outcome), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.Artifacts) != 1 || receipt.Artifacts[0].ReadablePath != hostPath {
		t.Fatalf("retained artifact refs = %#v, want only the safe host path", receipt.Artifacts)
	}
	if len(projected.Artifacts) != 2 || projected.Artifacts[0].ReadablePath != sensitivePath {
		t.Fatal("receipt projection mutated live artifact metadata")
	}
	if strings.Contains(projected.ProtectedReceipt.Outcome, secret) ||
		strings.Contains(projected.ProtectedReceipt.Outcome, sensitivePath) {
		t.Fatalf("sensitive attachment path leaked into receipt JSON: %s", projected.ProtectedReceipt.Outcome)
	}

	messages := []*agent.Message{
		agent.UserMessage("export the draft"),
		agent.AssistantMessage("", []agent.ToolCall{{
			ID: "call-export", Type: "function",
			Function: agent.FunctionCall{Name: "export", Arguments: `{}`},
		}}),
		agent.ToolMessage(projected, "call-export", agent.WithToolName("export")),
		agent.AssistantMessage("exported", nil),
		agent.UserMessage("continue"),
	}
	modelContext := strings.Join(messageContents(compactMessagesForModel(messages, "checkpoint", 1, 1)), "\n")
	if strings.Contains(modelContext, secret) || strings.Contains(modelContext, sensitivePath) ||
		!strings.Contains(modelContext, hostPath) {
		t.Fatalf("artifact path projection is unsafe or incomplete: %s", modelContext)
	}
}

func TestProtectedReceiptUsesCanonicalCheckpointPayloadAcrossPublishReload(t *testing.T) {
	previous := summarizeContextForCompaction
	defer func() { summarizeContextForCompaction = previous }()
	summarizeContextForCompaction = func(_ context.Context, _ *config.Config, _ string, _ string, _ []*agent.Message, _ string, _ int, _ contextCompactionPolicy, _ func(int, string)) (string, int, error) {
		return "## Current state\nThe protected read failed; use its durable artifact.", 1000, nil
	}

	raw := agent.ToolErrorResult("SECRET RAW RESULT", "display-only diagnostic")
	raw.Artifacts = []agent.ToolArtifactRef{{
		ID: "artifact-reload", ReadablePath: ".denova/artifacts/session/call-reload.log",
		ContentType: "text/plain", EstimatedBytes: 42_000, EstimatedTokens: 10_500, Complete: true,
	}}
	decision := processorTestDecision(agent.ToolResultProtected)
	decision.ProviderCallID = "call-reload"
	processed, err := processToolResult(
		context.Background(), decision,
		`{"path":"lore/cast.md","authorization":"Bearer do-not-persist"}`,
		raw, processorTestProcessingPolicy(16*1024),
	)
	if err != nil {
		t.Fatal(err)
	}
	messages := []*agent.Message{
		agent.UserMessage(strings.Repeat("large old request ", 1200)),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "call-reload", Type: "function", Function: agent.FunctionCall{Name: "read"}}}),
		agent.ToolMessage(processed, "call-reload", agent.WithToolName("read")),
		agent.AssistantMessage(strings.Repeat("large old answer ", 1200), nil),
		agent.UserMessage("latest request"),
	}
	cfg := &config.Config{}
	transient, result, err := PrepareContextCompaction(contextCompactionColdTestContext(), cfg, config.AgentKindIDE, ContextCompactionInput{
		Messages: messages, SourceMessages: messages, Force: true, KeepLatestUser: true,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Triggered || len(transient) < 2 || !strings.Contains(transient[0].Content, result.Summary) {
		t.Fatalf("canonical payload was not used by the transient checkpoint: result=%#v messages=%#v", result, transient)
	}
	for _, expected := range []string{"call-reload", "lore/cast.md", ".denova/artifacts/session/call-reload.log", "[redacted from retained tool context]"} {
		if !strings.Contains(result.Summary, expected) {
			t.Fatalf("canonical checkpoint payload missing %q: %s", expected, result.Summary)
		}
	}
	if strings.Contains(result.Summary, "SECRET RAW RESULT") || strings.Contains(result.Summary, "do-not-persist") {
		t.Fatalf("protected secret leaked into canonical checkpoint: %s", result.Summary)
	}
	if strings.Count(result.Summary, protectedCompactionReceiptTitle) != 1 {
		t.Fatalf("canonical checkpoint has duplicate receipt blocks: %s", result.Summary)
	}

	directory := t.TempDir()
	store, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("protected-receipt-reload")
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.AppendContextMessages(messages...); err != nil {
		t.Fatal(err)
	}
	if _, err := sess.AppendContextCompaction(session.ContextCompaction{
		AgentKind: config.AgentKindIDE, Epoch: 1, Summary: result.Summary,
		SourceStartIndex: 0, SourceEndIndex: len(messages), SourceMessageCount: len(messages), RetainedTurns: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reloadedStore, err := session.NewStore(directory)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reloadedStore.GetOrCreate("protected-receipt-reload")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reloaded.SnapshotContext(config.AgentKindIDE)
	if err != nil {
		t.Fatal(err)
	}
	reloadedMessages := NewSessionConversationForAgent(reloaded, cfg, config.AgentKindIDE).modelHistory(snapshot)
	if len(reloadedMessages) < 2 || !strings.Contains(reloadedMessages[0].Content, result.Summary) {
		t.Fatalf("durable checkpoint receipt did not survive reload: %#v", reloadedMessages)
	}
	if strings.Contains(reloadedMessages[0].Content, "SECRET RAW RESULT") || strings.Contains(reloadedMessages[0].Content, "do-not-persist") {
		t.Fatalf("reload exposed protected raw data: %s", reloadedMessages[0].Content)
	}
	recompactedPayload := contextCompactionCheckpointPayload(result.Summary, result.Summary, reloadedMessages, reloadedMessages[1:])
	_, receiptBlock := splitProtectedCompactionReceiptBlock(recompactedPayload)
	if strings.Count(recompactedPayload, protectedCompactionReceiptTitle) != 1 ||
		len(strings.Split(strings.TrimSpace(receiptBlock), "\n")) != 1 {
		t.Fatalf("re-compaction duplicated the durable receipt block: %s", recompactedPayload)
	}
}

func TestProtectedReceiptSelectionPrefersUnresolvedThenLatest(t *testing.T) {
	messages := []*agent.Message{agent.UserMessage("old")}
	for index := 0; index < protectedCompactionReceiptLimit+4; index++ {
		status := agent.ToolResultSuccess
		if index == 0 {
			status = agent.ToolResultError
		}
		messages = append(messages, &agent.Message{
			Role: agent.ToolRole, ToolCallID: "call-" + string(rune('A'+index)), ToolName: "read",
			ToolResult: &agent.ToolResultSummary{Status: status, ResultRetention: agent.ToolResultProtected},
		})
	}
	context := protectedToolReceiptContext(messages, nil)
	if !strings.Contains(context, `"call_id":"call-A"`) || !strings.Contains(context, `"selection":"unresolved_then_latest"`) {
		t.Fatalf("selection did not retain unresolved receipt and omission policy: %s", context)
	}
	if strings.Contains(context, `"call_id":"call-B"`) {
		t.Fatalf("oldest resolved receipt should be omitted before newer receipts: %s", context)
	}
}

func TestCompactionCandidateIdentityUsesPersistedPostProjectionBaseline(t *testing.T) {
	tool := agent.ToolMessage(agent.ToolResult{
		ModelContent: "bounded", Status: agent.ToolResultSuccess,
		ResultRetention: agent.ToolResultProtected,
		ProtectedReceipt: &agent.ToolResultProtectedReceipt{
			SanitizedArguments: `{}`,
			Outcome:            `{"schema":"tool_result.receipt.v2","status":"success"}`,
		},
	}, "old-call", agent.WithToolName("write"))
	messages := []*agent.Message{
		agent.UserMessage("old"),
		agent.AssistantMessage("", []agent.ToolCall{{ID: "old-call", Type: "function", Function: agent.FunctionCall{Name: "write"}}}),
		tool,
		agent.AssistantMessage("done", nil),
		agent.UserMessage("latest"),
	}
	projected := compactMessagesForModel(messages, "checkpoint", 1, 1)
	baselineFingerprint, baselineGeneration := ContextCompactionCandidateIdentity(projected, 0)
	reloadedFingerprint, reloadedGeneration := ContextCompactionCandidateIdentity(projected, 999_999)
	if baselineFingerprint != reloadedFingerprint || baselineGeneration != reloadedGeneration {
		t.Fatalf("persisted projection changed candidate identity: %q/%d vs %q/%d",
			baselineFingerprint, baselineGeneration, reloadedFingerprint, reloadedGeneration)
	}
	if !ContextCompactionNoProgressLatched(700, 1000, 0.85, 0.80, 20, 100,
		baselineFingerprint, baselineGeneration, reloadedFingerprint, reloadedGeneration) {
		t.Fatal("unchanged degraded post-projection candidate did not latch")
	}
	grown := append(append([]*agent.Message(nil), projected...), agent.ToolMessage(agent.TextToolResult("new"), "new-call", agent.WithToolName("read")))
	grownFingerprint, grownGeneration := ContextCompactionCandidateIdentity(grown, 0)
	if ContextCompactionNoProgressLatched(700, 1000, 0.85, 0.80, 20, 100,
		baselineFingerprint, baselineGeneration, grownFingerprint, grownGeneration) {
		t.Fatal("new tool candidate did not release degraded latch")
	}
}
