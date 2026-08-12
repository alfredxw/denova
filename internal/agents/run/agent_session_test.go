package agentrun

import (
	"reflect"
	"testing"

	"denova/config"

	agent "github.com/alfredxw/denova/agent"
)

func TestAgentSessionKeyRoundTripsEveryDenovaBinding(t *testing.T) {
	cases := []RuntimeBinding{
		{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "writing"},
		{AgentKind: AgentKindIDE, Mode: ModeAgentChat, Workspace: "/book", SessionID: "legacy-chat"},
		{AgentKind: AgentKindIDE, Mode: ModeAgentChat, ProjectID: "project", Workspace: "/mutable", SessionID: "ide-chat"},
		{AgentKind: AgentKindGeneral, Mode: ModeAgentChat, ProjectID: "project", Workspace: "/mutable", SessionID: "general-chat"},
		{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "branch"},
		{AgentKind: AgentKindConfigManager, Workspace: "/book", SessionID: "config"},
		{AgentKind: AgentKindHarnessOptimizer, SessionID: "optimizer"},
		{AgentKind: AgentKindImage, Workspace: "/book", SessionID: "image"},
		{AgentKind: AgentKindAutomation, Workspace: "/book", SessionID: "automation", TaskID: "task"},
		{AgentKind: config.AgentKindInteractiveDirector, Workspace: "/book", StoryID: "story", BranchID: "branch"},
	}
	seen := make(map[string]RuntimeBinding, len(cases))
	for _, original := range cases {
		key, err := original.AgentSessionKey()
		if err != nil {
			t.Fatalf("key for %#v: %v", original, err)
		}
		if key.Namespace == "" || key.ID == "" {
			t.Fatalf("incomplete key for %#v: %#v", original, key)
		}
		canonical := key.Namespace + "\x00" + key.ID
		if previous, exists := seen[canonical]; exists {
			t.Fatalf("distinct bindings collided: %#v and %#v -> %#v", previous, original, key)
		}
		seen[canonical] = original
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

func TestDenovaSessionSelectorsMatchOnlyTheirOwnedLanes(t *testing.T) {
	key := func(binding RuntimeBinding) agent.SessionKey {
		t.Helper()
		result, err := binding.AgentSessionKey()
		if err != nil {
			t.Fatalf("key for %#v: %v", binding, err)
		}
		return result
	}
	writing := key(RuntimeBinding{AgentKind: AgentKindIDE, Workspace: "/book", SessionID: "writing"})
	legacyChat := key(RuntimeBinding{AgentKind: AgentKindIDE, Mode: ModeAgentChat, Workspace: "/book", SessionID: "chat"})
	projectIDE := key(RuntimeBinding{AgentKind: AgentKindIDE, Mode: ModeAgentChat, ProjectID: "project", SessionID: "ide"})
	projectGeneral := key(RuntimeBinding{AgentKind: AgentKindGeneral, Mode: ModeAgentChat, ProjectID: "project", SessionID: "general"})
	game := key(RuntimeBinding{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "main"})
	director := key(RuntimeBinding{AgentKind: config.AgentKindInteractiveDirector, Workspace: "/book", StoryID: "story", BranchID: "main"})
	otherBranch := key(RuntimeBinding{AgentKind: AgentKindInteractiveStory, Workspace: "/book", StoryID: "story", BranchID: "fork"})

	foreground, err := ForegroundWorkspaceBindingSelectors("/book")
	if err != nil {
		t.Fatal(err)
	}
	if !matchesAnySelector(foreground, writing) || !matchesAnySelector(foreground, game) || !matchesAnySelector(foreground, director) {
		t.Fatalf("foreground selectors missed owned lanes: %#v", foreground)
	}
	if matchesAnySelector(foreground, legacyChat) || matchesAnySelector(foreground, projectIDE) {
		t.Fatalf("foreground selectors captured user conversation lanes: %#v", foreground)
	}

	project, err := ProjectBindingSelector("project")
	if err != nil {
		t.Fatal(err)
	}
	if !project.Matches(projectIDE) || !project.Matches(projectGeneral) || project.Matches(writing) {
		t.Fatalf("project selector crossed ownership boundary: %#v", project)
	}
	story, err := StoryBindingSelector("/book", "story", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !story.Matches(game) || !story.Matches(director) || story.Matches(otherBranch) {
		t.Fatalf("story selector crossed branch boundary: %#v", story)
	}
}

func matchesAnySelector(selectors []agent.SessionSelector, key agent.SessionKey) bool {
	for _, selector := range selectors {
		if selector.Matches(key) {
			return true
		}
	}
	return false
}

func TestProjectAgentSessionIdentitySurvivesWorkspaceRelink(t *testing.T) {
	before, err := AgentSessionKeyForOptions(Options{
		AgentKind: AgentKindIDE, Mode: ModeAgentChat, ProjectID: "project-1",
		Workspace: "/old/location", SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	after, err := AgentSessionKeyForOptions(Options{
		AgentKind: AgentKindIDE, Mode: ModeAgentChat, ProjectID: "project-1",
		Workspace: "/new/location", SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("project relink forked public Session identity: before=%#v after=%#v", before, after)
	}
	if _, exists := after.Attributes[bindingLabelWorkspace]; exists {
		t.Fatalf("mutable workspace leaked into project Session attributes: %#v", after.Attributes)
	}
}
