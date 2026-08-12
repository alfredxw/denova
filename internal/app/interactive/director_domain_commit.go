package interactiveapp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	agent "github.com/alfredxw/denova/agent"

	agentrun "denova/internal/agents/run"
	"denova/internal/interactive"
)

// interactiveDirectorPlanCommit is the game-domain adapter at the durable
// harness output-commit seam. The model may build and finalize an in-memory
// Patch draft, but only this adapter can publish it to the canonical store.
type interactiveDirectorPlanCommit struct {
	store        *interactive.Store
	storyID      string
	branchID     string
	sourceTurnID string
	token        interactive.DirectorPlanRunToken
	draft        *interactive.DirectorPlanUpdateDraft
	draftMu      *sync.Mutex

	mu              sync.Mutex
	identity        agentrun.CycleIdentity
	agentOutputHash string
	pending         *interactive.DirectorPlanDomainCommitIntent
	receipt         *interactive.DirectorPlanDomainCommitReceipt
}

func newInteractiveDirectorPlanCommit(store *interactive.Store, storyID, branchID, sourceTurnID string, token interactive.DirectorPlanRunToken, draft *interactive.DirectorPlanUpdateDraft, draftMu *sync.Mutex) *interactiveDirectorPlanCommit {
	if draftMu == nil {
		draftMu = &sync.Mutex{}
	}
	return &interactiveDirectorPlanCommit{
		store: store, storyID: strings.TrimSpace(storyID), branchID: strings.TrimSpace(branchID),
		sourceTurnID: strings.TrimSpace(sourceTurnID), token: token, draft: draft, draftMu: draftMu,
	}
}

func (c *interactiveDirectorPlanCommit) BindAgentCycleIdentity(identity agentrun.CycleIdentity) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.identity = identity
	c.agentOutputHash = ""
	c.pending = nil
	c.receipt = nil
	c.mu.Unlock()
}

func (c *interactiveDirectorPlanCommit) PendingAgentCycleCommit(stage agentrun.DomainCommitStage) (agentrun.DomainCommitIntent, bool, error) {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitIntent{}, false, nil
	}
	c.mu.Lock()
	identity := c.identity
	agentOutputHash := c.agentOutputHash
	c.mu.Unlock()
	if strings.TrimSpace(string(identity.CommandID)) == "" || strings.TrimSpace(string(identity.OperationID)) == "" || identity.Cycle <= 0 {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("互动导演输出缺少 durable cycle identity")
	}
	if c.draft == nil {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("互动导演 Patch 草稿不可用")
	}

	c.draftMu.Lock()
	docs, finalized := c.draft.FinalDocs()
	decision, decided := c.draft.Decision()
	c.draftMu.Unlock()
	if !finalized || !decided {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("导演规划未通过 submit_director_plan_update finalize Patch 草稿")
	}
	summary, err := json.Marshal(decision)
	if err != nil {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("序列化导演规划决策失败: %w", err)
	}
	intent, err := interactive.NewDirectorPlanDomainCommitIntent(
		interactive.DirectorPlanDomainCommitIdentity{
			CommandID: string(identity.CommandID), OperationID: string(identity.OperationID), Cycle: identity.Cycle,
		},
		agentOutputHash,
		c.token,
		c.sourceTurnID,
		string(summary),
		docs,
	)
	if err != nil {
		return agentrun.DomainCommitIntent{}, false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.identity != identity {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("互动导演 durable cycle identity 在提交准备期间发生变化")
	}
	if c.pending != nil && c.pending.Hash != intent.Hash {
		return agentrun.DomainCommitIntent{}, false, fmt.Errorf("互动导演输出提交内容在授权前发生变化")
	}
	c.pending = &intent
	return agentrun.DomainCommitIntent{Identity: identity, Stage: stage, Hash: intent.Hash}, true, nil
}

func (c *interactiveDirectorPlanCommit) CommitDirectorCanonicalOutput(
	ctx context.Context,
	request agent.OutputCommitRequest,
) (agent.OutputCommitReceipt, error) {
	if c == nil || request.Identity.Stage != agent.CommitOutput {
		return agent.OutputCommitReceipt{}, fmt.Errorf("互动导演 canonical output 请求无效")
	}
	identity := agentrun.CycleIdentity{
		CommandID:   agentrun.CommandID(request.Identity.CommandID),
		OperationID: agentrun.OperationID(request.Identity.RunID),
		Cycle:       request.Identity.Cycle,
	}
	c.mu.Lock()
	if c.identity != identity {
		c.mu.Unlock()
		return agent.OutputCommitReceipt{}, fmt.Errorf("互动导演 canonical output identity 与已绑定 cycle 不一致")
	}
	agentOutputHash := strings.TrimSpace(request.Hash)
	c.agentOutputHash = agentOutputHash
	c.mu.Unlock()
	if agentOutputHash == "" {
		return agent.OutputCommitReceipt{}, fmt.Errorf("互动导演 canonical output 缺少 Agent hash")
	}
	if _, pending, err := c.PendingAgentCycleCommit(agentrun.DomainCommitOutput); err != nil {
		return agent.OutputCommitReceipt{}, err
	} else if !pending {
		return agent.OutputCommitReceipt{}, fmt.Errorf("互动导演没有待提交的 canonical output")
	}
	if err := c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted}); err != nil {
		return agent.OutputCommitReceipt{}, err
	}
	receipt, ok := c.LastAgentCycleCommitReceipt(agentrun.DomainCommitOutput)
	if !ok || strings.TrimSpace(receipt.Revision) == "" {
		return agent.OutputCommitReceipt{}, fmt.Errorf("互动导演 canonical output 没有 durable receipt")
	}
	return agent.OutputCommitReceipt{Revision: receipt.Revision}, nil
}

func (c *interactiveDirectorPlanCommit) ReconcileDirectorCanonicalOutput(
	ctx context.Context,
	request agent.ReconcileRequest,
) (agent.ReconcileResult, error) {
	if c == nil || request.Identity.Stage != agent.CommitOutput {
		return agent.ReconcileResult{}, fmt.Errorf("互动导演 canonical output reconcile 请求无效")
	}
	if err := ctx.Err(); err != nil {
		return agent.ReconcileResult{}, err
	}
	receipt, found, err := c.store.FindDirectorPlanDomainCommit(
		c.storyID,
		c.branchID,
		interactive.DirectorPlanDomainCommitIdentity{
			CommandID: request.Identity.CommandID, OperationID: request.Identity.RunID, Cycle: request.Identity.Cycle,
		},
		request.Hash,
	)
	if err != nil || !found {
		return agent.ReconcileResult{Found: found}, err
	}
	return agent.ReconcileResult{Found: true, Revision: receipt.Revision}, nil
}

func (c *interactiveDirectorPlanCommit) CommitAgentCycleStage(_ context.Context, stage agentrun.DomainCommitStage, outcome agentrun.Outcome) error {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return nil
	}
	if outcome.Status != agentrun.OutcomeCompleted && outcome.Status != agentrun.OutcomePreempted {
		c.mu.Lock()
		if c.receipt != nil {
			c.mu.Unlock()
			return nil
		}
		c.pending = nil
		c.receipt = nil
		c.mu.Unlock()
		return nil
	}
	c.mu.Lock()
	pending := c.pending
	c.mu.Unlock()
	if pending == nil {
		return fmt.Errorf("互动导演输出提交未经过 durable intent 授权")
	}
	if c.store == nil {
		return fmt.Errorf("互动导演存储不可用")
	}
	receipt, err := c.store.CommitDirectorPlanRun(*pending)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.receipt = &receipt
	c.mu.Unlock()
	return nil
}

func (c *interactiveDirectorPlanCommit) LastAgentCycleCommitReceipt(stage agentrun.DomainCommitStage) (agentrun.DomainCommitReceipt, bool) {
	if c == nil || stage != agentrun.DomainCommitOutput {
		return agentrun.DomainCommitReceipt{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.receipt == nil || c.pending == nil {
		return agentrun.DomainCommitReceipt{}, false
	}
	return agentrun.DomainCommitReceipt{
		Identity: c.identity, Stage: stage, Hash: c.receipt.Hash, Revision: c.receipt.Revision,
	}, true
}

func (c *interactiveDirectorPlanCommit) committedResult() (interactive.DirectorPlan, string, bool) {
	if c == nil {
		return interactive.DirectorPlan{}, "", false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.receipt == nil || c.pending == nil {
		return interactive.DirectorPlan{}, "", false
	}
	return c.receipt.Plan, c.pending.Summary, true
}

// commitCustomGenerator preserves the App's test seam without weakening the
// production path. Only a generator explicitly marked custom can use this
// direct adapter; production always receives identity from the durable actor.
func (c *interactiveDirectorPlanCommit) commitCustomGenerator(ctx context.Context) error {
	if c == nil {
		return fmt.Errorf("互动导演输出提交不可用")
	}
	identity, err := interactiveDirectorCustomCycleIdentity(c.token, c.sourceTurnID)
	if err != nil {
		return err
	}
	c.BindAgentCycleIdentity(identity)
	c.mu.Lock()
	c.agentOutputHash = "custom:" + string(identity.CommandID)
	c.mu.Unlock()
	if _, pending, err := c.PendingAgentCycleCommit(agentrun.DomainCommitOutput); err != nil {
		return err
	} else if !pending {
		return fmt.Errorf("互动导演自定义生成器没有待提交输出")
	}
	return c.CommitAgentCycleStage(ctx, agentrun.DomainCommitOutput, agentrun.Outcome{Status: agentrun.OutcomeCompleted})
}

func interactiveDirectorCustomCycleIdentity(token interactive.DirectorPlanRunToken, sourceTurnID string) (agentrun.CycleIdentity, error) {
	commandID, err := DirectorCommandID(token, sourceTurnID, "custom_generator")
	if err != nil {
		return agentrun.CycleIdentity{}, err
	}
	operationID, err := DirectorCommandID(token, sourceTurnID, "custom_generator_operation")
	if err != nil {
		return agentrun.CycleIdentity{}, err
	}
	operationID = strings.Replace(operationID, "interactive-director:", "interactive-director-operation:", 1)
	return agentrun.CycleIdentity{
		CommandID: agentrun.CommandID(commandID), OperationID: agentrun.OperationID(operationID), Cycle: 1,
	}, nil
}
