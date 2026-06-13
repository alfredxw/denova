package agent

import "testing"

func TestRunOptionsCheckpointIDPrefersSession(t *testing.T) {
	options := RunOptions{
		AgentKind: AgentKindIDE,
		TaskID:    "task-1",
		SessionID: "session-1",
	}.normalized("")

	if got := options.checkpointID("run-1"); got != "ide:session:session-1" {
		t.Fatalf("checkpoint id = %q", got)
	}
}

func TestRunOptionsCheckpointIDFallsBackToTask(t *testing.T) {
	options := RunOptions{
		AgentKind: AgentKindInteractiveStory,
		TaskID:    "task-1",
	}.normalized("")

	if got := options.checkpointID("run-1"); got != "interactive_story:task:task-1" {
		t.Fatalf("checkpoint id = %q", got)
	}
}

func TestRunOptionsCheckpointIDFallsBackToRun(t *testing.T) {
	options := RunOptions{
		AgentKind: AgentKindUnknown,
	}.normalized("")

	if got := options.checkpointID("run-1"); got != "unknown:run:run-1" {
		t.Fatalf("checkpoint id = %q", got)
	}
}

func TestRunOptionsCheckpointIDEmptyWithoutStableInputs(t *testing.T) {
	options := RunOptions{}.normalized("")

	if got := options.checkpointID(""); got != "" {
		t.Fatalf("checkpoint id = %q", got)
	}
}
