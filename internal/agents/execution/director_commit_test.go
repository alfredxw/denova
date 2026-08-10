package execution

import (
	"context"
	"sync"
	"testing"

	"denova/config"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentinteractive "denova/internal/agents/interactive"
	agentrun "denova/internal/agents/run"

	agent "github.com/alfredxw/denova/agent"
	runstate "github.com/alfredxw/denova/agent/runtime"
)

func TestDirectorConversationKeepsRuntimeOpenUntilOutputCommitReceipt(t *testing.T) {
	service := newDomainCommitBarrierRuntime(t)
	participant := &directorCommitBarrierProbe{
		commitStarted: make(chan struct{}),
		releaseCommit: make(chan struct{}),
	}
	conversation := agentinteractive.NewDirectorConversation(agentinteractive.DirectorConversationOptions{
		Instruction:  agentconversation.InstructionOptions{Instruction: "更新导演规划"},
		DomainCommit: participant,
	})
	options := agentrun.Options{
		AgentKind: config.AgentKindInteractiveDirector, Workspace: "director-workspace",
		StoryID: "story-1", BranchID: "main", MaintenanceTask: "director_plan_update",
	}
	runner := newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("done", nil)}, true)
	done := make(chan agentrun.Outcome, 1)
	runOutcomeTestGoroutine(done, "Director barrier test", func() agentrun.Outcome {
		return runCycle(service,
			context.Background(), runner, conversation, nil,
			agentchat.ChatRequest{CommandID: "director-runtime-root", Message: "更新导演规划"}, options, nil,
		)
	})

	<-participant.commitStarted
	runtimeHandle := openDomainCommitBarrierBinding(t, service, options)
	observation, err := runtimeHandle.ObserveFromNow(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundOutputIntent := false
	for _, commit := range observation.Snapshot.DomainCommits {
		if commit.Identity.Stage == runstate.DomainCommitOutput && commit.Revision == "" {
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
	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil {
		t.Fatalf("Director runtime outcome = %#v, want completed", outcome)
	}
}

type directorCommitBarrierProbe struct {
	mu            sync.Mutex
	identity      agentrun.CycleIdentity
	receipt       agentrun.DomainCommitReceipt
	commitStarted chan struct{}
	releaseCommit chan struct{}
	startOnce     sync.Once
}

func (p *directorCommitBarrierProbe) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	p.mu.Lock()
	p.identity = identity
	p.mu.Unlock()
}

func (p *directorCommitBarrierProbe) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	p.mu.Lock()
	identity := p.identity
	p.mu.Unlock()
	return agentrun.DomainCommitIntent{Identity: identity, Stage: stage, Hash: "director-output-hash"}, true, nil
}

func (p *directorCommitBarrierProbe) CommitAgentCycleStage(_ context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if stage != agentrun.DomainCommitOutput || !agentrun.OutcomeMayCommitDomain(outcome) {
		return nil
	}
	p.startOnce.Do(func() { close(p.commitStarted) })
	<-p.releaseCommit
	p.mu.Lock()
	p.receipt = agentrun.DomainCommitReceipt{
		Identity: p.identity, Stage: stage, Hash: "director-output-hash", Revision: "director-revision-1",
	}
	p.mu.Unlock()
	return nil
}

func (p *directorCommitBarrierProbe) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.receipt, stage == agentrun.DomainCommitOutput && p.receipt.Revision != ""
}
