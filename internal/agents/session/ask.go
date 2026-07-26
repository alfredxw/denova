package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	askPendingSchema          = "ask.pending.v1"
	askResultSchema           = "ask.result.v1"
	askContinuationLostReason = "runtime_continuation_unavailable"
	maxAskQuestions           = 3
	minAskOptions             = 2
	maxAskOptions             = 3
	maxAskPayloadBytes        = 128 * 1024
	maxAskQuestionTextBytes   = 8 * 1024
	maxAskOptionTextBytes     = 4 * 1024
	maxAskCustomInputBytes    = 64 * 1024
	maxAskCancelReasonBytes   = 2 * 1024
	maxAskStableIDBytes       = 256
	reservedAskOtherOptionID  = "other"
)

var (
	ErrAskNotFound        = errors.New("ask interaction was not found")
	ErrAskNotPending      = errors.New("ask interaction is not pending")
	ErrAskAlreadyPending  = errors.New("another ask interaction is already pending")
	ErrAskAlreadyResolved = errors.New("ask interaction is already resolved")
	askStableIDPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
)

// AwaitAsk durably records a pending interaction, then waits without a default
// timeout. Replaying the same tool call is idempotent: a prior resolution is
// returned immediately and a prior pending record reattaches to the waiter.
func (s *Session) AwaitAsk(ctx context.Context, interaction AskInteraction) (AskResolution, error) {
	return s.AwaitAskWithPending(ctx, interaction, nil)
}

// AwaitAskWithPending invokes onPending only after the pending interaction is
// durably committed. Hosts use that ordering to publish a reconnectable UI
// event without presenting a question that cannot be answered by ID.
func (s *Session) AwaitAskWithPending(ctx context.Context, interaction AskInteraction, onPending func(AskInteraction)) (AskResolution, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	interaction, err := normalizeAskInteraction(interaction)
	if err != nil {
		return AskResolution{}, err
	}
	var waiter chan AskResolution
	var resolved *AskResolution
	err = s.withCanonicalMutation(ctx, "stage ask interaction", func() error {
		if existing := s.askByIDLocked(interaction.ID); existing != nil {
			if !sameAskRequest(*existing, interaction) {
				return fmt.Errorf("%w: id=%q payload changed", ErrAskAlreadyResolved, interaction.ID)
			}
			if existing.Status != AskPending {
				value := askResolutionFromInteraction(*existing)
				resolved = &value
				return nil
			}
			waiter = s.askWaiterLocked(interaction.ID)
			return nil
		}
		if pending := s.pendingAskLocked(""); pending != nil {
			return fmt.Errorf("%w: id=%q", ErrAskAlreadyPending, pending.ID)
		}
		if err := s.appendJournalRecordLocked(askRecord{Type: historyTypeAsk, AskInteraction: interaction}); err != nil {
			return err
		}
		copy := cloneAskInteraction(interaction)
		s.records = append(s.records, historyRecord{kind: historyTypeAsk, ask: &copy, createdAt: interaction.CreatedAt})
		advanceUpdatedAt(s, interaction.CreatedAt)
		waiter = s.askWaiterLocked(interaction.ID)
		return nil
	})
	if err != nil {
		return AskResolution{}, err
	}
	if resolved != nil {
		return *resolved, nil
	}
	if waiter == nil {
		return AskResolution{}, fmt.Errorf("ask interaction %q has no waiter", interaction.ID)
	}
	if onPending != nil {
		onPending(cloneAskInteraction(interaction))
	}

	select {
	case <-waiter:
		// The waiter channel is a broadcast signal, not a single-consumer result
		// queue. ResolveAsk commits the canonical result before closing it, so every
		// recovery attachment observes the exact same durable resolution.
		return s.resolvedAsk(interaction.ID)
	case <-ctx.Done():
		// A stopped task must not leave a permanently pending question. This is a
		// lifecycle cleanup, not the structured user-cancel result returned to a
		// live model call.
		_, _ = s.ResolveAsk(context.WithoutCancel(ctx), interaction.ID, AskCancelled, nil, "task_cancelled")
		return AskResolution{}, ctx.Err()
	}
}

// ResolveAsk atomically answers or cancels a pending interaction and wakes the
// exact blocked tool call. Any later submission returns the canonical terminal
// resolution without revalidating or replacing it.
func (s *Session) ResolveAsk(ctx context.Context, id, status string, answers []AskAnswer, cancelReason string) (AskResolution, error) {
	return s.resolveAsk(ctx, id, status, answers, cancelReason, false)
}

// ResolveAskFromHost accepts an interactive host answer while the exact tool
// call is live. If its waiter was lost after a cold restart, the pending Ask is
// atomically cancelled and that canonical cancellation is returned normally.
func (s *Session) ResolveAskFromHost(ctx context.Context, id, status string, answers []AskAnswer, cancelReason string) (AskResolution, error) {
	return s.resolveAsk(ctx, id, status, answers, cancelReason, true)
}

func (s *Session) resolveAsk(ctx context.Context, id, status string, answers []AskAnswer, cancelReason string, requireLiveWaiter bool) (AskResolution, error) {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	if id == "" {
		return AskResolution{}, fmt.Errorf("ask id is required")
	}
	if status != AskAnswered && status != AskCancelled {
		return AskResolution{}, fmt.Errorf("ask status must be %q or %q", AskAnswered, AskCancelled)
	}
	var resolution AskResolution
	var notify chan AskResolution
	err := s.withCanonicalMutation(ctx, "resolve ask interaction", func() error {
		interaction := s.askByIDLocked(id)
		if interaction == nil {
			return fmt.Errorf("%w: id=%q", ErrAskNotFound, id)
		}
		if interaction.Status != AskPending {
			resolution = askResolutionFromInteraction(*interaction)
			return nil
		}
		if requireLiveWaiter && s.askWaiters[id] == nil {
			candidate, err := buildAskResolution(*interaction, AskCancelled, nil, askContinuationLostReason)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if err := s.applyAskResolutionLocked(interaction, candidate, now); err != nil {
				return err
			}
			resolution = candidate
			delete(s.askWaiters, id)
			return nil
		}
		candidate, err := buildAskResolution(*interaction, status, answers, cancelReason)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := s.applyAskResolutionLocked(interaction, candidate, now); err != nil {
			return err
		}
		resolution = candidate
		notify = s.askWaiters[id]
		delete(s.askWaiters, id)
		return nil
	})
	if err != nil {
		return AskResolution{}, err
	}
	if notify != nil {
		close(notify)
	}
	return resolution, nil
}

// ReconcileStalePendingAsk terminates a pending Ask that belongs to one cold
// recovered runtime cycle and has no process-local waiter. The journal patch and
// in-memory transition are committed under the Session's canonical mutation
// lease, so a concurrent live attachment or host answer cannot observe a
// half-resolved interaction.
func (s *Session) ReconcileStalePendingAsk(ctx context.Context, identity AskCycleIdentity) (bool, error) {
	identity.CommandID = strings.TrimSpace(identity.CommandID)
	identity.OperationID = strings.TrimSpace(identity.OperationID)
	if identity.OperationID == "" || identity.Cycle <= 0 {
		return false, fmt.Errorf("ask recovery requires an operation id and positive cycle")
	}
	reconciled := false
	err := s.withCanonicalMutation(ctx, "reconcile stale ask interaction", func() error {
		interaction := s.pendingAskLocked("")
		if interaction == nil || s.askWaiters[interaction.ID] != nil || !askBelongsToCycle(*interaction, identity) {
			return nil
		}
		candidate, err := buildAskResolution(*interaction, AskCancelled, nil, askContinuationLostReason)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := s.applyAskResolutionLocked(interaction, candidate, now); err != nil {
			return err
		}
		delete(s.askWaiters, interaction.ID)
		reconciled = true
		return nil
	})
	return reconciled, err
}

func (s *Session) applyAskResolutionLocked(interaction *AskInteraction, resolution AskResolution, now time.Time) error {
	if err := s.appendJournalRecordLocked(askPatchRecord{
		Type: historyTypeAskPatch, TargetID: interaction.ID, Status: resolution.Status,
		Answers: cloneAskAnswerResults(resolution.Answers), CancelReason: resolution.CancelReason,
		ResolvedAt: now, UpdatedAt: now,
	}); err != nil {
		return err
	}
	interaction.Status = resolution.Status
	interaction.Answers = cloneAskAnswerResults(resolution.Answers)
	interaction.CancelReason = resolution.CancelReason
	interaction.ResolvedAt = &now
	advanceUpdatedAt(s, now)
	return nil
}

func askBelongsToCycle(interaction AskInteraction, identity AskCycleIdentity) bool {
	if interaction.AgentOperationID != "" && interaction.AgentOperationID != identity.OperationID {
		return false
	}
	if interaction.AgentCycle > 0 && interaction.AgentCycle != identity.Cycle {
		return false
	}
	return interaction.AgentCommandID == "" || identity.CommandID == "" || interaction.AgentCommandID == identity.CommandID
}

func (s *Session) resolvedAsk(id string) (AskResolution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	interaction := s.askByIDLocked(strings.TrimSpace(id))
	if interaction == nil {
		return AskResolution{}, fmt.Errorf("%w: id=%q", ErrAskNotFound, id)
	}
	if interaction.Status == AskPending {
		return AskResolution{}, fmt.Errorf("%w: id=%q", ErrAskNotPending, id)
	}
	return askResolutionFromInteraction(*interaction), nil
}

// PendingAsk returns the durable pending record regardless of whether its
// process-local model continuation is still alive. Recovery and host mutation
// paths use this view to terminalize orphaned interactions.
func (s *Session) PendingAsk(id string) *AskInteraction {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	interaction := s.pendingAskLocked(strings.TrimSpace(id))
	if interaction == nil {
		return nil
	}
	copy := cloneAskInteraction(*interaction)
	return &copy
}

// LivePendingAsk returns a pending interaction only while this Session owns
// the exact in-process waiter that will consume its result. UI projections must
// use this view: a durable pending record alone is not proof that answering can
// resume model execution after a cold restart.
func (s *Session) LivePendingAsk(id string) *AskInteraction {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	interaction := s.pendingAskLocked(strings.TrimSpace(id))
	if interaction == nil || s.askWaiters[interaction.ID] == nil {
		return nil
	}
	copy := cloneAskInteraction(*interaction)
	return &copy
}

func normalizeAskInteraction(interaction AskInteraction) (AskInteraction, error) {
	// AskInteraction is a public value input, but its slices and pointer fields
	// still alias caller-owned storage after the struct copy. Normalize only an
	// owned deep copy so concurrent retries may safely reuse the same request.
	interaction = cloneAskInteraction(interaction)
	interaction.Schema = askPendingSchema
	interaction.ID = strings.TrimSpace(interaction.ID)
	interaction.ToolCallID = strings.TrimSpace(interaction.ToolCallID)
	interaction.ProviderCallID = strings.TrimSpace(interaction.ProviderCallID)
	interaction.TaskID = strings.TrimSpace(interaction.TaskID)
	interaction.AgentKind = strings.TrimSpace(interaction.AgentKind)
	interaction.AgentCommandID = strings.TrimSpace(interaction.AgentCommandID)
	interaction.AgentOperationID = strings.TrimSpace(interaction.AgentOperationID)
	if interaction.AgentCycle < 0 {
		return AskInteraction{}, fmt.Errorf("ask agent cycle cannot be negative")
	}
	interaction.Status = AskPending
	interaction.Answers = nil
	interaction.CancelReason = ""
	interaction.ResolvedAt = nil
	if interaction.CreatedAt.IsZero() {
		interaction.CreatedAt = time.Now().UTC()
	}
	if err := validateAskStableID("ask id", interaction.ID); err != nil {
		return AskInteraction{}, err
	}
	if err := validateAskStableID("ask tool call id", interaction.ToolCallID); err != nil {
		return AskInteraction{}, err
	}
	if interaction.ProviderCallID != "" {
		if err := validateAskStableID("ask provider call id", interaction.ProviderCallID); err != nil {
			return AskInteraction{}, err
		}
	}
	if interaction.AgentKind == "" {
		return AskInteraction{}, fmt.Errorf("ask agent kind is required")
	}
	if len(interaction.Questions) < 1 || len(interaction.Questions) > maxAskQuestions {
		return AskInteraction{}, fmt.Errorf("ask requires 1-%d questions", maxAskQuestions)
	}
	questionIDs := make(map[string]struct{}, len(interaction.Questions))
	for index := range interaction.Questions {
		question := &interaction.Questions[index]
		question.ID = strings.TrimSpace(question.ID)
		question.Question = strings.TrimSpace(question.Question)
		question.RecommendedOptionID = strings.TrimSpace(question.RecommendedOptionID)
		if err := validateAskStableID(fmt.Sprintf("questions[%d].id", index), question.ID); err != nil {
			return AskInteraction{}, err
		}
		if _, duplicate := questionIDs[question.ID]; duplicate {
			return AskInteraction{}, fmt.Errorf("duplicate ask question id %q", question.ID)
		}
		questionIDs[question.ID] = struct{}{}
		if question.Question == "" || len(question.Question) > maxAskQuestionTextBytes {
			return AskInteraction{}, fmt.Errorf("questions[%d].question must contain 1-%d bytes", index, maxAskQuestionTextBytes)
		}
		if len(question.Options) == 0 {
			if question.MultiSelect || question.RecommendedOptionID != "" {
				return AskInteraction{}, fmt.Errorf("free-text question %q cannot be multi-select or recommended", question.ID)
			}
			continue
		}
		if len(question.Options) < minAskOptions || len(question.Options) > maxAskOptions {
			return AskInteraction{}, fmt.Errorf("question %q requires %d-%d options", question.ID, minAskOptions, maxAskOptions)
		}
		optionIDs := make(map[string]struct{}, len(question.Options))
		for optionIndex := range question.Options {
			option := &question.Options[optionIndex]
			option.ID = strings.TrimSpace(option.ID)
			option.Label = strings.TrimSpace(option.Label)
			option.Description = strings.TrimSpace(option.Description)
			if err := validateAskStableID(fmt.Sprintf("questions[%d].options[%d].id", index, optionIndex), option.ID); err != nil {
				return AskInteraction{}, err
			}
			if strings.EqualFold(option.ID, reservedAskOtherOptionID) {
				return AskInteraction{}, fmt.Errorf("option id %q is reserved for the host-provided Other choice", option.ID)
			}
			if _, duplicate := optionIDs[option.ID]; duplicate {
				return AskInteraction{}, fmt.Errorf("duplicate option id %q in question %q", option.ID, question.ID)
			}
			optionIDs[option.ID] = struct{}{}
			if option.Label == "" || len(option.Label)+len(option.Description) > maxAskOptionTextBytes {
				return AskInteraction{}, fmt.Errorf("option %q text must contain 1-%d bytes", option.ID, maxAskOptionTextBytes)
			}
		}
		if question.RecommendedOptionID != "" {
			if _, ok := optionIDs[question.RecommendedOptionID]; !ok {
				return AskInteraction{}, fmt.Errorf("recommended option %q does not exist in question %q", question.RecommendedOptionID, question.ID)
			}
		}
	}
	encoded, err := json.Marshal(interaction.Questions)
	if err != nil {
		return AskInteraction{}, err
	}
	if len(encoded) > maxAskPayloadBytes {
		return AskInteraction{}, fmt.Errorf("ask question payload exceeds %d bytes", maxAskPayloadBytes)
	}
	interaction.Questions = cloneAskQuestions(interaction.Questions)
	return interaction, nil
}

func buildAskResolution(interaction AskInteraction, status string, answers []AskAnswer, cancelReason string) (AskResolution, error) {
	result := AskResolution{Schema: askResultSchema, ID: interaction.ID, Status: status}
	if status == AskCancelled {
		if len(answers) != 0 {
			return AskResolution{}, fmt.Errorf("cancelled ask cannot include answers")
		}
		result.CancelReason = truncateUTF8ByBytes(strings.TrimSpace(cancelReason), maxAskCancelReasonBytes)
		if result.CancelReason == "" {
			result.CancelReason = "user_cancelled"
		}
		return result, nil
	}
	if len(answers) != len(interaction.Questions) {
		return AskResolution{}, fmt.Errorf("ask requires exactly one answer for each of %d questions", len(interaction.Questions))
	}
	byQuestion := make(map[string]AskAnswer, len(answers))
	for _, answer := range answers {
		answer.QuestionID = strings.TrimSpace(answer.QuestionID)
		answer.CustomInput = strings.TrimSpace(answer.CustomInput)
		if len(answer.CustomInput) > maxAskCustomInputBytes {
			return AskResolution{}, fmt.Errorf("custom answer for %q exceeds %d bytes", answer.QuestionID, maxAskCustomInputBytes)
		}
		if _, duplicate := byQuestion[answer.QuestionID]; duplicate {
			return AskResolution{}, fmt.Errorf("duplicate answer for question %q", answer.QuestionID)
		}
		byQuestion[answer.QuestionID] = answer
	}
	for _, question := range interaction.Questions {
		answer, ok := byQuestion[question.ID]
		if !ok {
			return AskResolution{}, fmt.Errorf("missing answer for question %q", question.ID)
		}
		answerResult := AskAnswerResult{QuestionID: question.ID, Question: question.Question, CustomInput: answer.CustomInput}
		if len(question.Options) == 0 {
			if len(answer.SelectedOptionIDs) != 0 || answer.CustomInput == "" {
				return AskResolution{}, fmt.Errorf("free-text question %q requires custom_input and no selected options", question.ID)
			}
			result.Answers = append(result.Answers, answerResult)
			continue
		}
		selected := make(map[string]struct{}, len(answer.SelectedOptionIDs))
		for _, rawID := range answer.SelectedOptionIDs {
			optionID := strings.TrimSpace(rawID)
			if optionID == "" {
				return AskResolution{}, fmt.Errorf("question %q contains an empty option id", question.ID)
			}
			if _, duplicate := selected[optionID]; duplicate {
				return AskResolution{}, fmt.Errorf("question %q selected option %q more than once", question.ID, optionID)
			}
			selected[optionID] = struct{}{}
		}
		if len(selected) == 0 || (!question.MultiSelect && len(selected) != 1) {
			return AskResolution{}, fmt.Errorf("question %q requires %s", question.ID, map[bool]string{true: "one or more options", false: "exactly one option"}[question.MultiSelect])
		}
		for _, option := range question.Options {
			if _, ok := selected[option.ID]; ok {
				answerResult.SelectedOptions = append(answerResult.SelectedOptions, AskSelectedOption{ID: option.ID, Label: option.Label})
				delete(selected, option.ID)
			}
		}
		if _, other := selected[reservedAskOtherOptionID]; other {
			if answer.CustomInput == "" {
				return AskResolution{}, fmt.Errorf("question %q selected Other without custom_input", question.ID)
			}
			answerResult.SelectedOptions = append(answerResult.SelectedOptions, AskSelectedOption{ID: reservedAskOtherOptionID, Label: "Other"})
			delete(selected, reservedAskOtherOptionID)
		} else if answer.CustomInput != "" {
			return AskResolution{}, fmt.Errorf("question %q supplied custom_input without selecting Other", question.ID)
		}
		if len(selected) != 0 {
			unknown := make([]string, 0, len(selected))
			for optionID := range selected {
				unknown = append(unknown, optionID)
			}
			slices.Sort(unknown)
			return AskResolution{}, fmt.Errorf("question %q selected unknown option(s): %s", question.ID, strings.Join(unknown, ", "))
		}
		result.Answers = append(result.Answers, answerResult)
	}
	if result.Answers == nil {
		result.Answers = []AskAnswerResult{}
	}
	return result, nil
}

func validateAskStableID(field, value string) error {
	if value == "" || len(value) > maxAskStableIDBytes || !askStableIDPattern.MatchString(value) {
		return fmt.Errorf("%s must be 1-%d bytes using letters, numbers, '.', '_', ':', or '-'", field, maxAskStableIDBytes)
	}
	return nil
}

func (s *Session) askWaiterLocked(id string) chan AskResolution {
	if s.askWaiters == nil {
		s.askWaiters = make(map[string]chan AskResolution)
	}
	if waiter := s.askWaiters[id]; waiter != nil {
		return waiter
	}
	waiter := make(chan AskResolution, 1)
	s.askWaiters[id] = waiter
	return waiter
}

func (s *Session) pendingAskLocked(id string) *AskInteraction {
	if interaction := s.askByIDLocked(id); interaction != nil && interaction.Status == AskPending {
		return interaction
	}
	return nil
}

func (s *Session) askByIDLocked(id string) *AskInteraction {
	for index := len(s.records) - 1; index >= 0; index-- {
		record := &s.records[index]
		if record.kind != historyTypeAsk || record.ask == nil {
			continue
		}
		if id == "" || record.ask.ID == id {
			return record.ask
		}
	}
	if s.projection != nil && s.projection.PendingAsk != nil && (id == "" || s.projection.PendingAsk.ID == id) {
		return s.projection.PendingAsk
	}
	return nil
}

func askResolutionFromInteraction(interaction AskInteraction) AskResolution {
	return AskResolution{
		Schema: askResultSchema, ID: interaction.ID, Status: interaction.Status,
		Answers: cloneAskAnswerResults(interaction.Answers), CancelReason: interaction.CancelReason,
	}
}

func sameAskRequest(left, right AskInteraction) bool {
	return left.ID == right.ID && left.ToolCallID == right.ToolCallID && left.ProviderCallID == right.ProviderCallID &&
		left.TaskID == right.TaskID && left.AgentKind == right.AgentKind && left.AgentCommandID == right.AgentCommandID &&
		left.AgentOperationID == right.AgentOperationID && left.AgentCycle == right.AgentCycle &&
		slices.EqualFunc(left.Questions, right.Questions, sameAskQuestion)
}

func sameAskQuestion(left, right AskQuestion) bool {
	return left.ID == right.ID && left.Question == right.Question && left.MultiSelect == right.MultiSelect &&
		left.RecommendedOptionID == right.RecommendedOptionID && slices.Equal(left.Options, right.Options)
}

func sameAskResolution(left, right AskResolution) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}

func cloneAskInteraction(value AskInteraction) AskInteraction {
	value.Questions = cloneAskQuestions(value.Questions)
	value.Answers = cloneAskAnswerResults(value.Answers)
	if value.ResolvedAt != nil {
		resolvedAt := *value.ResolvedAt
		value.ResolvedAt = &resolvedAt
	}
	return value
}

func cloneAskQuestions(values []AskQuestion) []AskQuestion {
	result := append([]AskQuestion(nil), values...)
	for index := range result {
		result[index].Options = append([]AskOption(nil), values[index].Options...)
	}
	return result
}

func cloneAskAnswerResults(values []AskAnswerResult) []AskAnswerResult {
	result := append([]AskAnswerResult(nil), values...)
	for index := range result {
		result[index].SelectedOptions = append([]AskSelectedOption(nil), values[index].SelectedOptions...)
	}
	return result
}
