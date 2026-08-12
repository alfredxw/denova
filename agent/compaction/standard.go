package compaction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agent "github.com/alfredxw/denova/agent"
)

type SummaryRequest struct {
	Session      agent.SessionView
	Run          agent.RunView
	Messages     []*agent.Message
	ModelRequest []*agent.Message
	Plan         agent.CompactionPlan
}

type Summary struct {
	Content       string
	TokenEstimate int
}

type Summarizer interface {
	Identity() agent.CapabilityIdentity
	Summarize(context.Context, SummaryRequest) (Summary, error)
}

type SummarizerFunc struct {
	Capability agent.CapabilityIdentity
	Func       func(context.Context, SummaryRequest) (Summary, error)
}

func (summarizer SummarizerFunc) Identity() agent.CapabilityIdentity { return summarizer.Capability }

func (summarizer SummarizerFunc) Summarize(ctx context.Context, request SummaryRequest) (Summary, error) {
	if summarizer.Func == nil {
		return Summary{}, errors.New("Compaction Summarizer function is nil")
	}
	return summarizer.Func(ctx, request)
}

type StandardConfig struct {
	Summarizer        Summarizer
	TriggerBytes      int
	KeepRecentBytes   int
	HardLimitBytes    int
	SummaryLimitBytes int
}

type standardManager struct {
	config   StandardConfig
	identity agent.CapabilityIdentity
}

func Standard(config StandardConfig) (agent.CompactionManager, error) {
	if config.Summarizer == nil {
		return nil, errors.New("standard Compaction requires a Summarizer")
	}
	if err := validateIdentity(config.Summarizer.Identity()); err != nil {
		return nil, fmt.Errorf("Compaction Summarizer: %w", err)
	}
	if config.TriggerBytes <= 0 {
		config.TriggerBytes = 2 << 20
	}
	if config.KeepRecentBytes <= 0 {
		config.KeepRecentBytes = 512 << 10
	}
	if config.HardLimitBytes <= 0 {
		return nil, errors.New("Compaction HardLimitBytes must be positive")
	}
	if config.SummaryLimitBytes <= 0 || config.SummaryLimitBytes > config.HardLimitBytes {
		return nil, errors.New("Compaction SummaryLimitBytes must be positive and no larger than HardLimitBytes")
	}
	if config.KeepRecentBytes >= config.TriggerBytes || config.TriggerBytes >= config.HardLimitBytes {
		return nil, errors.New("Compaction requires KeepRecentBytes < TriggerBytes < HardLimitBytes")
	}
	encoded, _ := json.Marshal(struct {
		Summarizer        agent.CapabilityIdentity
		TriggerBytes      int
		KeepRecentBytes   int
		HardLimitBytes    int
		SummaryLimitBytes int
	}{config.Summarizer.Identity(), config.TriggerBytes, config.KeepRecentBytes, config.HardLimitBytes, config.SummaryLimitBytes})
	digest := sha256.Sum256(encoded)
	return &standardManager{config: config, identity: agent.CapabilityIdentity{
		Kind: "compaction.standard", Version: 1, ConfigHash: hex.EncodeToString(digest[:]),
	}}, nil
}

func (manager *standardManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *standardManager) SummaryLimitBytes() int {
	if manager == nil {
		return 0
	}
	return manager.config.SummaryLimitBytes
}

func (manager *standardManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	bytes := messageBytes(request.ModelRequest)
	if len(request.ModelRequest) == 0 {
		bytes = messageBytes(request.Messages)
	}
	if !request.Force && bytes <= manager.config.TriggerBytes {
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	if bytes > manager.config.HardLimitBytes && len(request.Messages) < 2 {
		return agent.CompactionPlan{}, fmt.Errorf("%w: %d bytes exceed the %d-byte Compaction limit", agent.ErrContextLimit, bytes, manager.config.HardLimitBytes)
	}
	keep := 0
	sourceTo := len(request.Messages)
	for sourceTo > 1 && keep < manager.config.KeepRecentBytes {
		sourceTo--
		keep += messageBytes(request.Messages[sourceTo : sourceTo+1])
	}
	if sourceTo <= 0 {
		if bytes > manager.config.HardLimitBytes {
			return agent.CompactionPlan{}, fmt.Errorf("%w: final model request cannot be reduced below the %d-byte limit", agent.ErrContextLimit, manager.config.HardLimitBytes)
		}
		return agent.CompactionPlan{Action: agent.CompactionNone}, nil
	}
	return agent.CompactionPlan{Action: agent.CompactionCreate, SourceFrom: 0, SourceTo: sourceTo}, nil
}

func (manager *standardManager) Compact(ctx context.Context, request agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	if request.Plan.SourceFrom < 0 || request.Plan.SourceTo > len(request.Messages) || request.Plan.SourceTo <= request.Plan.SourceFrom {
		return agent.CompactionCheckpoint{}, errors.New("Compaction source range is invalid")
	}
	summary, err := manager.config.Summarizer.Summarize(ctx, SummaryRequest{
		Session: request.Session, Run: request.Run,
		Messages:     cloneMessages(request.Messages[request.Plan.SourceFrom:request.Plan.SourceTo]),
		ModelRequest: cloneMessages(request.ModelRequest), Plan: request.Plan,
	})
	if err != nil {
		return agent.CompactionCheckpoint{}, err
	}
	summary.Content = strings.TrimSpace(summary.Content)
	if summary.Content == "" || summary.TokenEstimate < 0 {
		return agent.CompactionCheckpoint{}, errors.New("Compaction Summarizer returned an invalid result")
	}
	if len(summary.Content) > manager.config.SummaryLimitBytes {
		return agent.CompactionCheckpoint{}, fmt.Errorf("%w: Compaction summary is %d bytes and exceeds the target Agent limit %d", agent.ErrContextLimit, len(summary.Content), manager.config.SummaryLimitBytes)
	}
	return agent.CompactionCheckpoint{Summary: summary.Content, TokenEstimate: summary.TokenEstimate}, nil
}

type disabledManager struct {
	hardLimit    int
	summaryLimit int
	identity     agent.CapabilityIdentity
}

func Disabled(hardLimitBytes, summaryLimitBytes int) agent.CompactionManager {
	return &disabledManager{hardLimit: hardLimitBytes, summaryLimit: summaryLimitBytes, identity: agent.CapabilityIdentity{
		Kind: "compaction.disabled", Version: 1, ConfigHash: fmt.Sprintf("input:%d;summary:%d", hardLimitBytes, summaryLimitBytes),
	}}
}

func (manager *disabledManager) Identity() agent.CapabilityIdentity { return manager.identity }

func (manager *disabledManager) SummaryLimitBytes() int {
	if manager == nil {
		return 0
	}
	return manager.summaryLimit
}

func (manager *disabledManager) Plan(_ context.Context, request agent.CompactionPlanRequest) (agent.CompactionPlan, error) {
	bytes := messageBytes(request.Messages)
	if bytes > manager.hardLimit {
		return agent.CompactionPlan{}, fmt.Errorf("%w: %d bytes exceed disabled Compaction limit %d", agent.ErrContextLimit, bytes, manager.hardLimit)
	}
	return agent.CompactionPlan{Action: agent.CompactionNone}, nil
}

func (*disabledManager) Compact(context.Context, agent.CompactionCompactRequest) (agent.CompactionCheckpoint, error) {
	return agent.CompactionCheckpoint{}, agent.ErrCapabilityUnsupported
}

func messageBytes(messages []*agent.Message) int {
	encoded, _ := json.Marshal(messages)
	return len(encoded)
}

func cloneMessages(messages []*agent.Message) []*agent.Message {
	result := make([]*agent.Message, len(messages))
	for index, message := range messages {
		result[index] = message.Clone()
	}
	return result
}

func validateIdentity(identity agent.CapabilityIdentity) error {
	if strings.TrimSpace(identity.Kind) == "" || identity.Version == 0 {
		return errors.New("capability identity is incomplete")
	}
	return nil
}

var _ agent.CompactionManager = (*standardManager)(nil)
var _ agent.CompactionManager = (*disabledManager)(nil)
