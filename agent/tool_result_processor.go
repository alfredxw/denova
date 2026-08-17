package agent

import (
	"context"
	"errors"
	"fmt"
)

// ToolResultProcessRequest is the immutable post-execution view supplied by
// modelToolLoop. Processors may project, materialize, and attach recovery metadata, but
// cannot change the already-approved tool arguments or execute the tool again.
type ToolResultProcessRequest struct {
	ToolName       string
	Arguments      string
	ExecutionID    string
	ProviderCallID string
	Definition     ToolDefinitionSnapshot
	Result         ToolResult
}

// ToolResultProcessor is the fixed post-tool result seam. It runs after the
// approved endpoint and all execution middleware, and before normalization,
// event publication, transcript persistence, cleanup, and compaction.
type ToolResultProcessor interface {
	Identity() CapabilityIdentity
	Process(context.Context, ToolResultProcessRequest) (ToolResult, error)
}

type toolResultProcessorChain struct {
	processors []ToolResultProcessor
	identity   CapabilityIdentity
}

// ChainToolResultProcessors composes processors in source order. Every
// processor observes the previous processor's result; partial results are
// retained when a processor returns an error so a durable diagnostic can still
// be paired with the assistant tool call.
func ChainToolResultProcessors(processors ...ToolResultProcessor) (ToolResultProcessor, error) {
	resolved := make([]ToolResultProcessor, 0, len(processors))
	identities := make([]CapabilityIdentity, 0, len(processors))
	for index, processor := range processors {
		if processor == nil {
			continue
		}
		identity := processor.Identity()
		if err := identity.validate(fmt.Sprintf("ToolResultProcessor %d", index)); err != nil {
			return nil, err
		}
		resolved = append(resolved, processor)
		identities = append(identities, identity)
	}
	if len(resolved) == 0 {
		return nil, errors.New("ToolResultProcessor chain is empty")
	}
	hash, err := hashCanonical(identities)
	if err != nil {
		return nil, err
	}
	return &toolResultProcessorChain{
		processors: resolved,
		identity: CapabilityIdentity{
			Kind: "tool_result_processor.chain", Version: 1, ConfigHash: hash,
		},
	}, nil
}

func (chain *toolResultProcessorChain) Identity() CapabilityIdentity {
	if chain == nil {
		return CapabilityIdentity{}
	}
	return chain.identity
}

func (chain *toolResultProcessorChain) Process(ctx context.Context, request ToolResultProcessRequest) (ToolResult, error) {
	if chain == nil {
		return request.Result, errors.New("ToolResultProcessor chain is nil")
	}
	result := request.Result
	for _, processor := range chain.processors {
		request.Result = result
		processed, err := processor.Process(ctx, request)
		result = processed
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func identityOfToolResultProcessor(processor ToolResultProcessor) CapabilityIdentity {
	if processor == nil {
		return CapabilityIdentity{Kind: "tool_result_processor.none", Version: 1}
	}
	return processor.Identity()
}
