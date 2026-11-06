package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"time"

	"denova/internal/agents/runtime/external"
	"github.com/google/uuid"
)

type pendingSteer struct {
	id             string
	guidance       external.Guidance
	prepared       external.PreparedSteer
	previousResult bool
}

type streamRun struct {
	input        external.Input
	initialInput []byte
	output       *streamOutput
	host         external.Host
	steeringHost external.SteeringHost
	controls     *external.Steering
	bridge       *toolBridge
	write        func([]byte) error

	pending *pendingSteer
}

func (run *streamRun) steer(ctx context.Context, waitingForUser bool) error {
	guidance, present, err := run.controls.Next(ctx)
	if err != nil || !present {
		return err
	}
	if waitingForUser || run.steeringHost == nil || !run.bridge.holdToolsForSteer() {
		run.controls.Interrupt()
		return nil
	}
	guidance.StartsNewReply = true
	prepared, err := run.steeringHost.PrepareSteer(ctx, guidance)
	if err != nil {
		return err
	}
	id := uuid.NewString()
	body, err := encodeInput(prepared.Input, id)
	if err != nil {
		return err
	}
	run.pending = &pendingSteer{id: id, guidance: guidance, prepared: prepared}
	return run.write(body)
}

func (run *streamRun) read(ctx context.Context, cancel context.CancelFunc, reader io.Reader, stdin io.Closer) (runErr error) {
	controlCtx, stopControls := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { stopControls(); workers.Wait() }()
	type controlResult struct {
		id   string
		body []byte
		err  error
	}
	completed := make(chan controlResult, 64)
	pending := map[string]context.CancelCauseFunc{}
	var initialization <-chan time.Time
	if run.initialInput != nil {
		timer := time.NewTimer(infrastructureTimeout)
		defer timer.Stop()
		initialization = timer.C
	}
	if run.steeringHost != nil {
		run.output.toolIDs = map[string]bool{}
	}
	readCtx, stopReading := context.WithCancel(context.WithoutCancel(ctx))
	defer stopReading()
	frames := scanRunFrames(readCtx, reader)
	recheck := false
	corrupt := false
	for {
		var steering <-chan struct{}
		if run.controls != nil && run.output.initialized && !run.output.terminal && run.pending == nil && ctx.Err() == nil {
			if recheck {
				recheck = false
				if err := run.steer(ctx, len(pending) != 0); err != nil {
					runErr = errors.Join(runErr, fmt.Errorf("steer Claude: %w", err))
					cancel()
				}
			}
			if run.pending == nil && ctx.Err() == nil {
				steering = run.controls.Changed
			}
		}
		select {
		case <-initialization:
			initialization = nil
			runErr = errors.Join(runErr, errors.New("Claude initialization timed out"))
			cancel()
		case result := <-completed:
			finish, exists := pending[result.id]
			if !exists {
				continue
			}
			finish(nil)
			delete(pending, result.id)
			if result.err != nil {
				body, _ := json.Marshal(map[string]any{"type": "control_response", "response": map[string]any{"subtype": "error", "request_id": result.id, "error": result.err.Error()}})
				result.body = append(body, '\n')
			}
			if err := run.write(result.body); err != nil && ctx.Err() == nil {
				runErr = errors.Join(runErr, err)
				cancel()
			}
		case <-steering:
			recheck = true
		case next, ok := <-frames:
			if !ok {
				if corrupt {
					run.output.terminal = false
				}
				if run.pending != nil {
					run.output.terminal = false
					if ctx.Err() == nil {
						runErr = errors.Join(runErr, errors.New("Claude exited before acknowledging guidance"))
					}
				}
				return runErr
			}
			if next.err != nil {
				corrupt = true
				runErr = errors.Join(runErr, next.err)
				cancel()
				continue
			}
			f := next.frame
			switch f.Type {
			case "control_response":
				if f.Response.RequestID == "denova-initialize" && run.initialInput != nil {
					initialization = nil
					if f.Response.Subtype != "success" {
						runErr = errors.Join(runErr, errors.New("Claude rejected initialization"))
						cancel()
					} else if err := run.write(run.initialInput); err != nil {
						runErr = errors.Join(runErr, err)
						cancel()
					}
					run.initialInput = nil
				}
				continue
			case "control_cancel_request":
				if finish := pending[f.RequestID]; finish != nil {
					finish(external.ErrInteractionWithdrawn)
					delete(pending, f.RequestID)
				}
				continue
			case "control_request":
				if f.RequestID == "" || len(pending) >= cap(completed) || pending[f.RequestID] != nil {
					runErr = errors.Join(runErr, errors.New("invalid Claude control request"))
					cancel()
					continue
				}
				requestCtx, finish := context.WithCancelCause(controlCtx)
				pending[f.RequestID] = finish
				workers.Add(1)
				go func() {
					defer workers.Done()
					result := controlResult{id: f.RequestID}
					defer func() {
						if value := recover(); value != nil {
							result.err = fmt.Errorf("native interaction panic: %v", value)
							slog.Error("Native interaction panicked", "error", result.err)
						}
						select {
						case completed <- result:
						case <-controlCtx.Done():
						}
					}()
					result.body, result.err = run.control(requestCtx, f)
				}()
				continue
			}
			if run.pending != nil && f.Parent == "" {
				pending := run.pending
				switch {
				case f.Type == "user" && f.IsReplay && f.UUID == pending.id && ctx.Err() == nil:
					if err := pending.prepared.Commit(context.WithoutCancel(ctx)); err != nil {
						runErr = errors.Join(runErr, fmt.Errorf("commit Claude guidance: %w", err))
						cancel()
						continue
					}
					run.controls.Delivered(pending.guidance)
					run.bridge.acceptSteer(run.output.toolIDs)
					clear(run.output.toolIDs)
					run.pending = nil
					recheck = true
				case f.Type == "result":
					if pending.previousResult || f.InputUUID == pending.id || slices.Contains(f.InputUUIDs, pending.id) {
						runErr = errors.Join(runErr, errors.New("Claude finished guidance without acknowledging it"))
						cancel()
					}
					pending.previousResult = true
				}
			}
			if err := run.output.feedFrame(f, run.host); err != nil {
				runErr = errors.Join(runErr, err)
				cancel()
			}
			if run.output.terminal {
				if run.pending != nil && ctx.Err() == nil {
					// priority=now ends the previous response before the correction
					// is acknowledged. Keep its output and usage, and keep stdin open.
					run.output.terminal = false
					run.output.replyStart = len(run.output.order)
				} else {
					_ = stdin.Close()
				}
			}
		}
	}
}

type runFrame struct {
	frame streamFrame
	err   error
}

// The reader drains stdout on cancellation so the CLI can report settlement.
func scanRunFrames(ctx context.Context, reader io.Reader) <-chan runFrame {
	frames := make(chan runFrame)
	send := func(frame runFrame) bool {
		select {
		case frames <- frame:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		defer close(frames)
		defer func() {
			if value := recover(); value != nil {
				send(runFrame{err: fmt.Errorf("read Claude stream: %v", value)})
			}
		}()
		scanner := bufio.NewScanner(reader)
		// Match the external input budget, including image-bearing tool mirrors.
		scanner.Buffer(make([]byte, 64<<10), 32<<20)
		for scanner.Scan() {
			if len(bytes.TrimSpace(scanner.Bytes())) == 0 {
				continue
			}
			var frame streamFrame
			err := json.Unmarshal(scanner.Bytes(), &frame)
			if err != nil {
				err = fmt.Errorf("decode Claude stream: %w", err)
			}
			if !send(runFrame{frame: frame, err: err}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			send(runFrame{err: err})
		}
	}()
	return frames
}
