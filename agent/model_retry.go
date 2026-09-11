package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net"
	"time"
)

// ModelOutputState distinguishes a failed preview from a complete response.
type ModelOutputState string

const (
	ModelOutputNone     ModelOutputState = "none"
	ModelOutputPartial  ModelOutputState = "partial"
	ModelOutputComplete ModelOutputState = "complete"
)

type RetryAction string

const (
	RetryStop  RetryAction = "stop"
	RetryAgain RetryAction = "retry"
)

// RetryContext describes a failed provider call. Attempt is one-based and
// includes this call; retry policy cannot replace the request or its model.
type RetryContext struct {
	Attempt     int
	Err         error
	OutputState ModelOutputState
}

type RetryDecision struct {
	Action RetryAction
	Delay  time.Duration
	Reason string
}

// RetryConfig decides only whether a failed call should be repeated. The
// ExecutionPolicy owns the shared network retry and output repair budget.
// A nil config disables automatic failure retries.
type RetryConfig struct {
	Decide func(context.Context, RetryContext) RetryDecision
}

// TransientRetry is the standard failure policy: exponential backoff with
// jitter, a 30-second local cap, and any longer provider Retry-After delay.
// Provider adapters may expose Retryable and RetryDelay methods on errors.
func TransientRetry(ctx context.Context, attempt RetryContext) RetryDecision {
	stop := RetryDecision{Action: RetryStop}
	if attempt.Err == nil || ctx.Err() != nil || errors.Is(attempt.Err, context.Canceled) {
		return stop
	}
	reason := "network_unavailable"
	var classified interface{ Retryable() bool }
	var network net.Error
	switch {
	case errors.As(attempt.Err, &classified):
		if !classified.Retryable() {
			return stop
		}
		reason = "provider_unavailable"
	case errors.Is(attempt.Err, context.DeadlineExceeded), errors.Is(attempt.Err, io.ErrUnexpectedEOF), errors.Is(attempt.Err, io.EOF):
	case errors.As(attempt.Err, &network):
	default:
		return stop
	}
	delay := min(30*time.Second, time.Second<<min(5, max(0, attempt.Attempt-1)))
	delay = delay/2 + time.Duration(rand.Int64N(int64(delay/2)+1))
	var hint interface{ RetryDelay() time.Duration }
	if errors.As(attempt.Err, &hint) {
		delay = max(delay, hint.RetryDelay())
	}
	return RetryDecision{Action: RetryAgain, Delay: delay, Reason: reason}
}

// ModelOutput is a detached response and request snapshot for business review.
// Attempt is one-based within the current logical model response budget.
type ModelOutput struct {
	Attempt int
	Message *Message
	Request *ModelRequestSnapshot
}

type ModelOutputAction string

const (
	ModelOutputAccept ModelOutputAction = "accept"
	ModelOutputRepair ModelOutputAction = "repair"
)

// ModelOutputReview permits only acceptance or bounded, attributed feedback.
// Feedback replaces this middleware's previous feedback for the same response;
// it never replaces accepted history, options, or the model adapter.
type ModelOutputReview struct {
	Action   ModelOutputAction
	Feedback []ContextFragment
	Reason   string
}

const modelRepairFeedbackMaxBytes = 64 * 1024

func modelRepairMessages(review ModelOutputReview) ([]*Message, error) {
	var messages []*Message
	total := 0
	for _, fragment := range review.Feedback {
		if fragment.Source == "" || fragment.Purpose == "" || fragment.HardLimit <= 0 {
			return nil, errors.New("model output feedback requires source, purpose, and a positive hard limit")
		}
		total += len(fragment.Content)
		if len(fragment.Content) > fragment.HardLimit || total > modelRepairFeedbackMaxBytes {
			return nil, errors.New("model output feedback exceeds its byte limit")
		}
		role := fragment.Role
		if role == "" {
			role = User
		}
		if role != User && role != Assistant {
			return nil, errors.New("model output feedback must use user or assistant role")
		}
		messages = append(messages, &Message{Role: role, Content: fmt.Sprintf("[source=%s; purpose=%s]\n%s", fragment.Source, fragment.Purpose, fragment.Content)})
	}
	return messages, nil
}

// modelResponseRejected ends a preview without admitting it to the transcript.
// The following retry event explains the next attempt to display consumers.
type modelResponseRejected struct{ reason string }

func (err *modelResponseRejected) Error() string {
	return "model response was not accepted: " + err.reason
}

func decideModelRetry(ctx context.Context, policy *RetryConfig, attempt RetryContext) (RetryDecision, error) {
	if policy == nil || policy.Decide == nil {
		return RetryDecision{Action: RetryStop}, nil
	}
	decision := policy.Decide(ctx, attempt)
	switch decision.Action {
	case RetryStop:
		return decision, nil
	case RetryAgain:
		if decision.Delay < 0 {
			return RetryDecision{}, errors.New("model retry delay cannot be negative")
		}
		return decision, nil
	default:
		return RetryDecision{}, fmt.Errorf("unsupported model retry action %q", decision.Action)
	}
}

type modelAttemptResult struct {
	message     *Message
	failure     error
	outputState ModelOutputState
	review      ModelOutputAction
	reason      string
}

// executeModelAttempts is shared by the main loop and fixed-model side calls.
// Preparation/review errors stop immediately; only provider failures reach the
// failure policy. The callback cannot reset the budget after output repair.
func executeModelAttempts(ctx context.Context, maximum int, retry *RetryConfig,
	call func(int) (modelAttemptResult, error),
	repeating func(int, modelAttemptResult, RetryDecision) error,
) (*Message, error) {
	maximum = max(1, maximum)
	for attempt := 1; ; attempt++ {
		if err := admitRunWork(ctx); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := call(attempt)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		decision := RetryDecision{Action: RetryStop}
		if result.failure != nil {
			decision, err = decideModelRetry(ctx, retry, RetryContext{Attempt: attempt, Err: result.failure, OutputState: result.outputState})
			if err != nil {
				return nil, err
			}
		} else if result.review == ModelOutputRepair {
			decision = RetryDecision{Action: RetryAgain, Reason: result.reason}
		}
		if decision.Action == RetryStop {
			return result.message, result.failure
		}
		if attempt >= maximum {
			if result.failure != nil {
				return nil, result.failure
			}
			return nil, fmt.Errorf("model output rejected after %d attempts: %s", maximum, result.reason)
		}
		if repeating != nil {
			if err := repeating(attempt, result, decision); err != nil {
				return nil, err
			}
		}
		slog.InfoContext(ctx, "Retrying model request", "attempt", attempt, "max_attempts", maximum, "output_state", result.outputState, "delay", decision.Delay, "reason", decision.Reason)
		if decision.Delay > 0 {
			if err := waitContext(ctx, decision.Delay); err != nil {
				return nil, err
			}
		}
	}
}

// Complete returns one accepted response from a fixed request, buffering any
// stream so partial failed attempts never become the next attempt's output.
// Only ModelMaxAttempts and Retry from policy apply; this is a side call with
// its own budget, independent of the main response or total Agent run length.
func (snapshot *ModelRequestSnapshot) Complete(ctx context.Context, policy ExecutionPolicy) (*Message, error) {
	if snapshot == nil || snapshot.model == nil {
		return nil, errors.New("model request snapshot is unavailable")
	}
	return executeModelAttempts(ctx, policy.ModelMaxAttempts, policy.Retry, func(int) (modelAttemptResult, error) {
		result := modelAttemptResult{outputState: ModelOutputNone, review: ModelOutputAccept}
		if !snapshot.Streaming() {
			result.message, result.failure = awaitContextCall(ctx, func() (*Message, error) { return snapshot.Generate(ctx) }, nil, nil)
		} else {
			stream, err := awaitContextCall(ctx, func() (*StreamReader[*Message], error) { return snapshot.Stream(ctx) }, nil, func(stream *StreamReader[*Message]) {
				if stream != nil {
					stream.Close()
				}
			})
			if err != nil {
				result.failure = err
				return result, nil
			}
			if stream == nil {
				return result, errors.New("model side call returned a nil stream")
			}
			defer func() {
				if ctx.Err() == nil {
					stream.Close()
					return
				}
				safeGo(stream.Close, func(err error) { slog.Error("Close cancelled model side stream failed", "error", err) })
			}()
			assembler := NewMessageAssembler()
			for {
				chunk, err := awaitContextCall(ctx, stream.Recv, stream.Close, nil)
				if errors.Is(err, io.EOF) {
					result.message, result.failure = assembler.Message()
					break
				}
				if err != nil {
					result.failure = err
					break
				}
				result.outputState = ModelOutputPartial
				if err := assembler.Append(chunk); err != nil {
					return result, err
				}
			}
		}
		if result.failure == nil {
			if result.message == nil {
				return result, errors.New("model side call returned no response")
			}
			result.outputState = ModelOutputComplete
		}
		return result, nil
	}, nil)
}
