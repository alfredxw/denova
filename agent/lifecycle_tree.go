package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// A tree command lives in each participating Session journal. RunIDs records
// admission at the stop boundary; it is not a second task registry.
type persistedTreeControl struct {
	persistedSessionControl
	TreeID     string   `json:"tree_id"`
	RunIDs     []string `json:"run_ids,omitempty"`
	superseded bool
}

func (control persistedTreeControl) sealed() bool {
	return control.Kind == "suspend_tree" || control.Kind == "abort_tree"
}

func (session *Session) acceptTreeControl(ctx context.Context, kind, treeID, commandID, reason string) (persistedTreeControl, error) {
	session.agent.admissionMu.Lock()
	defer session.agent.admissionMu.Unlock()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.closed {
		return persistedTreeControl{}, ErrSessionClosed
	}
	hash, err := hashCanonical(struct{ Kind, TreeID, Reason string }{kind, treeID, reason})
	if err != nil {
		return persistedTreeControl{}, err
	}
	if previous, found := session.controlReceipts[commandID]; found {
		if previous.Hash != hash {
			return persistedTreeControl{}, ErrIdempotencyConflict
		}
		if session.treeControl.Receipt.CommandID == commandID {
			return session.treeControl, nil
		}
		return persistedTreeControl{TreeID: treeID, superseded: true, persistedSessionControl: persistedSessionControl{
			persistedControlReceipt: previous, Kind: kind, Reason: reason,
		}}, nil
	}
	if session.inputs[commandID] != nil {
		return persistedTreeControl{}, ErrIdempotencyConflict
	}
	if err := ValidateIdempotencyKey(commandID); err != nil {
		return persistedTreeControl{}, err
	}
	control := persistedTreeControl{TreeID: treeID, persistedSessionControl: persistedSessionControl{
		persistedControlReceipt: persistedControlReceipt{Receipt: CommandReceipt{CommandID: commandID, Cursor: session.cursor + 1}, Hash: hash},
		Kind:                    kind, Reason: reason,
	}}
	if session.active != nil && !session.active.isSettled() {
		control.Receipt.RunID = session.active.id
		control.RunIDs = append(control.RunIDs, session.active.id)
	}
	for _, run := range session.pending {
		if !run.isSettled() {
			control.RunIDs = append(control.RunIDs, run.id)
		}
	}
	if err := session.appendRecordLocked(ctx, sessionControlRecord, control); err != nil {
		return persistedTreeControl{}, err
	}
	session.treeControl = control
	session.controlReceipts[commandID], session.cursor = control.persistedControlReceipt, control.Receipt.Cursor
	return control, nil
}

// SuspendTree is the host task-stop boundary. It fences admission before
// discovering descendants and closes the root writer only after every child.
// Individual Session.SuspendAndClose never implicitly traverses descendants.
func (owner *Agent) SuspendTree(ctx context.Context, key SessionKey, request SuspendRequest) (Suspension, error) {
	root, err := owner.Session(ctx, key)
	if err != nil {
		return Suspension{}, err
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = newPublicID("suspend-tree")
	}
	if request.Reason == "" {
		request.Reason = "Agent task tree suspended"
	}
	if request.RunID != "" {
		root.mu.RLock()
		matches := root.active != nil && root.active.id == request.RunID
		root.mu.RUnlock()
		if !matches {
			return Suspension{}, ErrNoActiveRun
		}
	}
	control, err := root.acceptTreeControl(ctx, "suspend_tree", request.IdempotencyKey, request.IdempotencyKey, request.Reason)
	if err != nil {
		return Suspension{}, err
	}
	if control.superseded {
		return Suspension{Session: root.Key(), RunID: control.Receipt.RunID, Status: ResultSuspended, Receipt: control.Receipt}, nil
	}
	request.IdempotencyKey = control.TreeID + ":session"
	result, err := root.suspendAndClose(ctx, request, func() error {
		keys, err := owner.descendantKeys(context.Background(), key)
		if err != nil {
			return err
		}
		var failures error
		for _, childKey := range keys {
			child, openErr := owner.Session(context.Background(), childKey)
			if openErr != nil {
				failures = errors.Join(failures, openErr)
				continue
			}
			child.mu.RLock()
			independent := child.lastControl.Kind == "suspend" && child.treeControl.TreeID != control.TreeID
			child.mu.RUnlock()
			if independent {
				failures = errors.Join(failures, child.closeWriter())
				continue
			}
			_, stopErr := child.acceptTreeControl(context.Background(), "suspend_tree", control.TreeID, control.TreeID, request.Reason)
			if stopErr == nil {
				_, stopErr = child.SuspendAndClose(context.Background(), SuspendRequest{Reason: request.Reason, IdempotencyKey: control.TreeID + ":session"})
			}
			failures = errors.Join(failures, stopErr)
		}
		return failures
	})
	result.Receipt = control.Receipt
	return result, err
}

// ResumeTree continues only unfinished Runs named by the original tree stop.
// Descendants rejoin first so normal parent completion reconciliation works.
func (owner *Agent) ResumeTree(ctx context.Context, key SessionKey, request ResumeRequest) (*Run, error) {
	root, err := owner.Session(ctx, key)
	if err != nil {
		return nil, err
	}
	root.mu.RLock()
	control := root.treeControl
	root.mu.RUnlock()
	if control.Kind != "suspend_tree" && control.Kind != "resume_tree" {
		return root.ResumeRun(ctx, request)
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = newPublicID("resume-tree")
	}
	root.mu.RLock()
	previous, repeated := root.controlReceipts[request.IdempotencyKey]
	root.mu.RUnlock()
	if repeated {
		hash, err := hashCanonical(struct{ Kind, TreeID, Reason string }{"resume_tree", control.TreeID, ""})
		if err != nil {
			return nil, err
		}
		if previous.Hash != hash {
			return nil, ErrIdempotencyConflict
		}
	}
	keys, err := owner.descendantKeys(ctx, key)
	if err != nil {
		return nil, err
	}
	for _, childKey := range keys {
		child, err := owner.Session(ctx, childKey)
		if err != nil {
			return nil, err
		}
		child.mu.RLock()
		childControl := child.treeControl
		child.mu.RUnlock()
		if childControl.TreeID != control.TreeID || childControl.Kind != "suspend_tree" {
			child.mu.RLock()
			idle := child.active == nil
			child.mu.RUnlock()
			if idle {
				if err := child.closeWriter(); err != nil {
					return nil, err
				}
			}
			continue
		}
		if len(childControl.RunIDs) != 0 {
			if _, err := child.resumeRun(ctx, ResumeRequest{RunID: childControl.RunIDs[0], IdempotencyKey: request.IdempotencyKey + ":run"}, control.TreeID); err != nil && !errors.Is(err, ErrRunSettled) {
				return nil, err
			}
		}
		if _, err := child.acceptTreeControl(ctx, "resume_tree", control.TreeID, request.IdempotencyKey, ""); err != nil {
			return nil, err
		}
		if len(childControl.RunIDs) == 0 {
			if err := child.closeWriter(); err != nil {
				return nil, err
			}
		}
	}
	if request.RunID == "" {
		request.RunID = control.Receipt.RunID
	}
	run, err := root.resumeRun(ctx, ResumeRequest{RunID: request.RunID, IdempotencyKey: request.IdempotencyKey + ":run"}, control.TreeID)
	if err != nil {
		return nil, err
	}
	if _, err := root.acceptTreeControl(ctx, "resume_tree", control.TreeID, request.IdempotencyKey, ""); err != nil {
		return nil, err
	}
	return run, nil
}

// AbortTree shares the admission fence and descendant-first writer shutdown.
// Cancellation is durable; opening an interrupted cancellation cannot resume it.
func (owner *Agent) AbortTree(ctx context.Context, key SessionKey, request AbortRequest) (CommandReceipt, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	root, err := owner.Session(ctx, key)
	if err != nil {
		return CommandReceipt{}, err
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = newPublicID("abort-tree")
	}
	if request.Reason == "" {
		request.Reason = "Agent task tree aborted"
	}
	control, err := root.acceptTreeControl(ctx, "abort_tree", request.IdempotencyKey, request.IdempotencyKey, request.Reason)
	if err != nil {
		return CommandReceipt{}, err
	}
	if control.superseded {
		return control.Receipt, nil
	}
	done := make(chan error, 1)
	safeGo(func() {
		keys, err := owner.descendantKeys(context.Background(), key)
		if err != nil {
			done <- err
			return
		}
		var failures error
		for _, childKey := range keys {
			child, err := owner.Session(context.Background(), childKey)
			if err == nil {
				_, err = child.acceptTreeControl(context.Background(), "abort_tree", control.TreeID, control.TreeID, request.Reason)
			}
			if err == nil {
				err = child.closeForTree(control.TreeID)
			}
			failures = errors.Join(failures, err)
		}
		if failures == nil {
			failures = root.closeForTree(control.TreeID)
		}
		done <- failures
	}, func(err error) { slog.Error("Agent task tree cancellation failed", "error", err); done <- err })
	select {
	case err := <-done:
		return control.Receipt, err
	case <-ctx.Done():
		return control.Receipt, ctx.Err()
	}
}

func (owner *Agent) descendantKeys(ctx context.Context, root SessionKey) ([]SessionKey, error) {
	queue := []SessionKey{root}
	seen := make(map[string]bool)
	for index := 0; index < len(queue); index++ {
		attributes, err := ChildSessionAttributes(queue[index])
		if err != nil {
			return nil, err
		}
		children, err := owner.ListSessions(ctx, SessionSelector{Attributes: attributes})
		if err != nil {
			return nil, err
		}
		owner.mu.RLock()
		for _, session := range owner.sessions {
			if (SessionSelector{Attributes: attributes}).Matches(session.key) {
				children = append(children, session.Key())
			}
		}
		owner.mu.RUnlock()
		for _, child := range children {
			canonical, err := agentsession.CanonicalKey(child)
			if err != nil {
				return nil, err
			}
			if !seen[canonical] {
				seen[canonical] = true
				queue = append(queue, child)
			}
		}
	}
	keys := queue[1:]
	sort.Slice(keys, func(i, j int) bool { return sessionTreeDepth(keys[i]) > sessionTreeDepth(keys[j]) })
	return keys, nil
}

// treeSealed is called under admissionMu. Parent identity is immutable; only
// the journals decide whether admission is open, including after a restart.
func (owner *Agent) treeSealed(ctx context.Context, key SessionKey, permit string) (bool, error) {
	for depth := 0; depth <= agentsession.MaxAttributes; depth++ {
		session, err := owner.Session(ctx, key)
		if err != nil {
			return false, err
		}
		session.mu.RLock()
		control := session.treeControl
		session.mu.RUnlock()
		if control.sealed() && (permit == "" || permit != control.TreeID) {
			return true, nil
		}
		if strings.TrimSpace(key.Attributes[ParentSessionAttribute]) == "" {
			return false, nil
		}
		key, err = ParentSessionKey(key)
		if err != nil {
			return false, err
		}
	}
	return false, errors.New("Agent task parent chain exceeds the identity limit")
}

func resumeTreePermit(ctx context.Context) string {
	run, _ := ctx.Value(canonicalRunKey{}).(*Run)
	if run == nil {
		return ""
	}
	return run.treeResumeID
}

func admitRunWork(ctx context.Context) error {
	run, _ := ctx.Value(canonicalRunKey{}).(*Run)
	if run == nil {
		return nil
	}
	return run.admitWork(ctx)
}

func (run *Run) admitWork(ctx context.Context) error {
	owner := run.session.agent
	owner.admissionMu.Lock()
	defer owner.admissionMu.Unlock()
	sealed, err := owner.treeSealed(ctx, run.session.key, run.treeResumeID)
	if err != nil {
		return err
	}
	if !sealed {
		return nil
	}
	run.mu.Lock()
	run.suspendReason = fmt.Sprintf("Agent task tree admission is suspended for Run %s", run.id)
	run.mu.Unlock()
	return context.Canceled
}
