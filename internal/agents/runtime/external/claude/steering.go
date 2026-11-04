package claude

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"

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
	output       *streamOutput
	host         external.Host
	steeringHost external.SteeringHost
	controls     *external.Steering
	bridge       *toolBridge
	write        func([]byte) error

	pending *pendingSteer
}

func (run *streamRun) steer(ctx context.Context) error {
	guidance, present, err := run.controls.Next(ctx)
	if err != nil || !present {
		return err
	}
	if run.steeringHost == nil || !run.bridge.holdToolsForSteer() {
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
				if err := run.steer(ctx); err != nil {
					runErr = errors.Join(runErr, fmt.Errorf("steer Claude: %w", err))
					cancel()
				}
			}
			if run.pending == nil && ctx.Err() == nil {
				steering = run.controls.Changed
			}
		}
		select {
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
