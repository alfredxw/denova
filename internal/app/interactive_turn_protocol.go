package app

import "denova/internal/interactive"

type interactiveTurnPhase uint8

const (
	interactiveTurnCollecting interactiveTurnPhase = iota
	interactiveTurnSubmitted
	interactiveTurnCommitted
)

// interactiveTurnProtocol owns the lifecycle of one hidden Game Agent result.
// Callers synchronize access with interactiveConversation.mu.
type interactiveTurnProtocol struct {
	phase      interactiveTurnPhase
	submission *interactive.PreparedTurnSubmission
}

func (p *interactiveTurnProtocol) submit(submission *interactive.PreparedTurnSubmission) bool {
	if p == nil || submission == nil || p.phase != interactiveTurnCollecting {
		return false
	}
	p.submission = submission
	p.phase = interactiveTurnSubmitted
	return true
}

func (p *interactiveTurnProtocol) narrativeReady() bool {
	if p == nil || p.submission == nil {
		return false
	}
	switch p.phase {
	case interactiveTurnCollecting:
		return false
	case interactiveTurnSubmitted, interactiveTurnCommitted:
		return true
	}
	return false
}

func (p *interactiveTurnProtocol) turnResult() *interactive.TurnResult {
	if !p.narrativeReady() {
		return nil
	}
	result := p.submission.TurnResult()
	return &result
}

func (p *interactiveTurnProtocol) markCommitted() {
	if p != nil && p.phase == interactiveTurnSubmitted {
		p.phase = interactiveTurnCommitted
	}
}
