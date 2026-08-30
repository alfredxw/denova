package app

import (
	"context"
	"fmt"
	"log/slog"

	"denova/internal/interactive"
)

func (a *App) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	return a.interactiveService().CreateInteractiveBranch(storyID, req)
}

func (s *InteractiveAppService) CreateInteractiveBranch(storyID string, req interactive.CreateBranchRequest) (interactive.BranchSummary, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.BranchSummary{}, ErrNoWorkspace
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return interactive.BranchSummary{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.BranchSummary{}, err
	}
	return store.CreateBranch(storyID, req)
}

func (a *App) SwitchInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().SwitchInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) SwitchInteractiveBranch(storyID, branchID string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, "")
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	return store.SwitchBranch(storyID, branchID)
}

func (a *App) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	return a.interactiveService().SwitchInteractiveTurnVersion(storyID, req)
}

func (s *InteractiveAppService) SwitchInteractiveTurnVersion(storyID string, req interactive.SwitchTurnVersionRequest) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	return store.SwitchTurnVersion(storyID, req)
}

func (a *App) UpdateInteractiveTurnNarrative(storyID string, req interactive.UpdateTurnNarrativeRequest) (interactive.UpdateTurnNarrativeResult, error) {
	return a.interactiveService().UpdateInteractiveTurnNarrative(storyID, req)
}

func (s *InteractiveAppService) UpdateInteractiveTurnNarrative(storyID string, req interactive.UpdateTurnNarrativeRequest) (interactive.UpdateTurnNarrativeResult, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.UpdateTurnNarrativeResult{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, req.BranchID)
	if err != nil {
		return interactive.UpdateTurnNarrativeResult{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.UpdateTurnNarrativeResult{}, err
	}
	a := s.app
	a.mu.Lock()
	if err := fence.validateLocked(a); err != nil {
		a.mu.Unlock()
		return interactive.UpdateTurnNarrativeResult{}, err
	}
	result, err := store.UpdateTurnNarrative(storyID, req)
	a.mu.Unlock()
	if err != nil {
		slog.ErrorContext(context.Background(), fmt.Sprintf("[interactive-turn-edit] update failed story_id=%s branch_id=%s turn_id=%s err=%v", storyID, req.BranchID, req.TurnID, err))
		return interactive.UpdateTurnNarrativeResult{}, err
	}
	slog.WarnContext(context.Background(), fmt.Sprintf(
		"[interactive-turn-edit] narrative updated story_id=%s branch_id=%s turn_id=%s narrative_bytes=%d",
		storyID,
		result.Turn.BranchID,
		result.Turn.ID,
		len([]byte(result.Turn.Narrative)),
	))
	return result, nil
}

func (a *App) DeleteInteractiveBranch(storyID, branchID string) error {
	return a.interactiveService().DeleteInteractiveBranch(storyID, branchID)
}

func (s *InteractiveAppService) DeleteInteractiveBranch(storyID, branchID string) error {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return ErrNoWorkspace
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, branchID)
	if err != nil {
		return err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return err
	}
	if fence.chat == nil {
		return ErrNoWorkspace
	}
	if err := fence.chat.DeleteStoryBindings(context.Background(), fence.projectID, storyID, branchID); err != nil {
		return err
	}
	return store.DeleteBranch(storyID, branchID)
}

func (a *App) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	return a.interactiveService().InteractiveBranches(storyID)
}

func (s *InteractiveAppService) InteractiveBranches(storyID string) ([]interactive.BranchSummary, error) {
	store := s.store()
	if store == nil {
		return nil, ErrNoWorkspace
	}
	return store.Branches(storyID)
}

func (a *App) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	return a.interactiveService().AppendInteractiveTurn(storyID, branchID, user, narrative)
}

func (s *InteractiveAppService) AppendInteractiveTurn(storyID, branchID, user, narrative string) (interactive.TurnEvent, error) {
	s.admission.Lock()
	defer s.admission.Unlock()
	store := s.store()
	if store == nil {
		return interactive.TurnEvent{}, ErrNoWorkspace
	}
	storyCtx, err := store.StoryContext(storyID, branchID)
	if err != nil {
		return interactive.TurnEvent{}, err
	}
	fence, err := s.drainInteractiveBinding(context.Background(), storyID, storyCtx.Snapshot.BranchID)
	if err != nil {
		return interactive.TurnEvent{}, err
	}
	a := s.app
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := fence.validateLocked(a); err != nil {
		return interactive.TurnEvent{}, err
	}
	return store.AppendTurn(storyID, interactive.AppendTurnRequest{
		BranchID:  branchID,
		User:      user,
		Narrative: narrative,
	})
}
