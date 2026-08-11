package agentrun

import (
	"reflect"
	"testing"

	"denova/config"
)

func TestAgentSessionKeyRoundTripsEveryDenovaBinding(t *testing.T) {
	cases := []RuntimeBinding{
		{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "writing"},
		{AgentKind: AgentKindIDE, Mode: ModeAgentChat, Workspace: "/book", SessionID: "legacy-chat"},
		{AgentKind: AgentKindIDE, Mode: ModeAgentChat, ProjectID: "project", Workspace: "/mutable", SessionID: "ide-chat"},
		{AgentKind: AgentKindGeneral, Mode: ModeAgentChat, ProjectID: "project", Workspace: "/mutable", SessionID: "general-chat"},
		{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "branch"},
		{AgentKind: AgentKindConfigManager, Workspace: "/book", SessionID: "config"},
		{AgentKind: AgentKindImage, Workspace: "/book", SessionID: "image"},
		{AgentKind: AgentKindAutomation, Workspace: "/book", SessionID: "automation", TaskID: "task"},
		{AgentKind: config.AgentKindInteractiveDirector, Workspace: "/book", StoryID: "story", BranchID: "branch"},
	}
	for _, original := range cases {
		key, err := original.AgentSessionKey()
		if err != nil {
			t.Fatalf("key for %#v: %v", original, err)
		}
		if key.Namespace == "" || key.ID == "" {
			t.Fatalf("incomplete key for %#v: %#v", original, key)
		}
		restored, err := RuntimeBindingFromAgentSessionKey(key)
		if err != nil {
			t.Fatalf("restore %#v: %v", key, err)
		}
		// Project-scoped AgentChat intentionally discards mutable workspace paths.
		if original.ProjectID != "" {
			original.Workspace = ""
		}
		if !reflect.DeepEqual(restored, original) {
			t.Fatalf("round trip got=%#v want=%#v", restored, original)
		}
	}
}
