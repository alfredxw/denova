package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"denova/config"
	"denova/internal/agentruntime"
	"denova/internal/book"
)

func TestGenerateInteractiveDirectorWithToolsRequiresOwnedRuntime(t *testing.T) {
	workspace := t.TempDir()
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		nil,
		&config.Config{Workspace: workspace},
		book.NewState(workspace),
		InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if err == nil || !strings.Contains(err.Error(), "运行时") {
		t.Fatalf("missing App-owned runtime error = %v", err)
	}
}

func TestGenerateInteractiveDirectorWithToolsRejectsMissingStoryState(t *testing.T) {
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		NewEphemeralChatService(),
		&config.Config{},
		nil,
		InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if err == nil || !strings.Contains(err.Error(), "故事状态") {
		t.Fatalf("missing story state error = %v", err)
	}
}

func TestGenerateInteractiveDirectorWithToolsRequiresCommandIDBeforeBuildingAgent(t *testing.T) {
	workspace := t.TempDir()
	_, err := GenerateInteractiveDirectorWithTools(
		context.Background(),
		NewEphemeralChatService(),
		&config.Config{Workspace: workspace},
		book.NewState(workspace),
		InteractiveStoryToolContext{StoryID: "story-1", BranchID: "main"},
		"更新导演规划",
	)
	if !errors.Is(err, agentruntime.ErrInvalidCommand) || !strings.Contains(err.Error(), "command_id") {
		t.Fatalf("missing Director command_id error = %v", err)
	}
}

func TestSingleInstructionConversationKeepsRuntimeOpenUntilDirectorOutputCommitReceipt(t *testing.T) {
	service := newHarnessBarrierChatService(t)
	participant := &directorCommitBarrierProbe{
		commitStarted: make(chan struct{}),
		releaseCommit: make(chan struct{}),
	}
	conversation := &singleInstructionConversation{instruction: "更新导演规划", domainCommit: participant}
	options := RunOptions{
		AgentKind: config.AgentKindInteractiveDirector, Workspace: "director-workspace",
		StoryID: "story-1", BranchID: "main", MaintenanceTask: "director_plan_update",
	}
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: schema.AssistantMessage("done", nil)}, true)
	done := make(chan RunOutcome, 1)
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				done <- RunOutcome{Status: RunOutcomeFailed, Error: fmt.Errorf("Director barrier test panic: %v", recovered)}
			}
		}()
		done <- service.RunWithOptions(context.Background(), runner, conversation, nil, ChatRequest{CommandID: "director-runtime-root", Message: "更新导演规划"}, options, nil)
	}()

	<-participant.commitStarted
	harness := openHarnessBarrierBinding(t, service, options)
	observation, err := harness.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundOutputIntent := false
	for _, commit := range observation.Snapshot.DomainCommits {
		if commit.Identity.Stage == agentruntime.DomainCommitOutput && commit.Revision == "" {
			foundOutputIntent = true
		}
	}
	if !foundOutputIntent {
		t.Fatalf("Director commit callback ran before durable output authorization: %#v", observation.Snapshot.DomainCommits)
	}
	select {
	case outcome := <-done:
		t.Fatalf("Director runtime settled before canonical receipt: %#v", outcome)
	default:
	}
	close(participant.releaseCommit)
	outcome := <-done
	if outcome.Status != RunOutcomeCompleted || outcome.Error != nil {
		t.Fatalf("Director runtime outcome = %#v, want completed", outcome)
	}
}

type directorCommitBarrierProbe struct {
	mu            sync.Mutex
	identity      HarnessCycleIdentity
	receipt       HarnessDomainCommitReceipt
	commitStarted chan struct{}
	releaseCommit chan struct{}
	startOnce     sync.Once
}

func (p *directorCommitBarrierProbe) BindAgentCycleIdentity(identity HarnessCycleIdentity) {
	p.mu.Lock()
	p.identity = identity
	p.mu.Unlock()
}

func (p *directorCommitBarrierProbe) PendingAgentCycleCommit(stage HarnessDomainCommitStage) (HarnessDomainCommitIntent, bool, error) {
	if stage != HarnessDomainCommitOutput {
		return HarnessDomainCommitIntent{}, false, nil
	}
	p.mu.Lock()
	identity := p.identity
	p.mu.Unlock()
	return HarnessDomainCommitIntent{Identity: identity, Stage: stage, Hash: "director-output-hash"}, true, nil
}

func (p *directorCommitBarrierProbe) CommitAgentCycleStage(_ context.Context, stage HarnessDomainCommitStage, outcome RunOutcome) error {
	if stage != HarnessDomainCommitOutput || !runOutcomeMayCommitDomain(outcome) {
		return nil
	}
	p.startOnce.Do(func() { close(p.commitStarted) })
	<-p.releaseCommit
	p.mu.Lock()
	p.receipt = HarnessDomainCommitReceipt{
		Identity: p.identity, Stage: stage, Hash: "director-output-hash", Revision: "director-revision-1",
	}
	p.mu.Unlock()
	return nil
}

func (p *directorCommitBarrierProbe) LastAgentCycleCommitReceipt(stage HarnessDomainCommitStage) (HarnessDomainCommitReceipt, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receipt, stage == HarnessDomainCommitOutput && p.receipt.Revision != ""
}
