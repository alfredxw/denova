package agentruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	agentchat "denova/internal/agents/chat"
	"denova/internal/agents/goal"
	agentrun "denova/internal/agents/run"
)

const goalContinuationMessage = "Continue working autonomously on the active goal. Reassess the full objective and current workspace state, make the next meaningful progress, and call goal_finish only when the complete objective is achieved or genuinely blocked."

// GoalContinuationRequest derives an idempotent hidden NextTurn command from
// the exact parent operation and goal revision.
func GoalContinuationRequest(current goal.State, parent agentrun.OperationID, locale string) (string, agentchat.ChatRequest) {
	digest := sha256.Sum256([]byte(strings.Join([]string{current.ID, string(parent), strconv.FormatUint(current.Revision, 10)}, "\x00")))
	commandID := "goal-next-" + hex.EncodeToString(digest[:16])
	return commandID, agentchat.ChatRequest{
		CommandID: commandID, Message: goalContinuationMessage,
		Locale: strings.TrimSpace(locale), InputVisibility: agentrun.InputModelOnly,
	}
}
