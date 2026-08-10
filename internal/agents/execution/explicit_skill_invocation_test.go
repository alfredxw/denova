package execution

import (
	"context"
	agentchat "denova/internal/agents/chat"
	agentconversation "denova/internal/agents/conversation"
	agentrun "denova/internal/agents/run"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	agent "github.com/alfredxw/denova/agent"

	"denova/config"
	agentcontext "denova/internal/agents/context"
	"denova/internal/agents/prompts"
	"denova/internal/agents/session"
	novaskills "denova/internal/agents/skills"
	producttools "denova/internal/agents/tools"
)

func TestExplicitSkillsAreLoadedBeforeFirstModelCallFromAnywhereInMessage(t *testing.T) {
	skillsDir := t.TempDir()
	writeExplicitSkillFixture(t, skillsDir, "alpha", "Alpha instructions", "ALPHA_BODY")
	writeExplicitSkillFixture(t, skillsDir, "beta", "Beta instructions", "BETA_BODY")
	workspace := t.TempDir()
	dataDir := t.TempDir()
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: workspace, DenovaDir: dataDir}

	backend := novaskills.NewAgentBackend(
		novaskills.NewDirectories(cfg.SkillsDir, cfg.DataDir(), cfg.Workspace),
		config.AgentKindIDE,
		config.ResolveAgentSkillOverrides(cfg, config.AgentKindIDE),
	)
	skillTool, err := producttools.NewSkill(context.Background(), backend, agentcontext.ContextBudgetForAgent(cfg, config.AgentKindIDE).MaxFragmentBytes)
	if err != nil {
		t.Fatal(err)
	}
	collector := &explicitSkillEventCollector{}
	model := &explicitSkillCaptureModel{events: collector.Snapshot}
	builtAgent, err := agent.NewAgent(context.Background(), agent.AgentConfig{
		Name:        "explicit-skill-test",
		Description: "explicit skill preload integration test",
		Instruction: "test",
		Model:       model,
		Tools:       []agent.ToolDefinition{skillTool},
	})
	if err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.RunnerConfig{Agent: builtAgent, EnableStreaming: true})

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("explicit-skills")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	service := NewEphemeralRuntime()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	message := "先分析背景，接着请用 /alpha 处理；然后 /beta 收尾，最后再按 /alpha 核对。"

	outcome := runCycle(service,
		context.Background(),
		runner,
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "explicit-skills-anywhere", Message: message},
		agentrun.Options{
			AgentKind: config.AgentKindIDE, RootAgentName: "explicit-skill-test",
			Workspace: workspace, SessionID: "explicit-skills",
		},
		collector.Append,
	)

	if outcome.Status != agentrun.OutcomeCompleted || outcome.Error != nil {
		t.Fatalf("outcome = %#v, want completed", outcome)
	}
	input, eventsAtFirstCall, calls := model.Captured()
	if calls != 1 {
		t.Fatalf("model calls = %d, want 1", calls)
	}
	finalUser := lastUserMessageContent(input)
	for _, want := range []string{"# Skill: alpha", "ALPHA_BODY", "# Skill: beta", "BETA_BODY", message} {
		if !strings.Contains(finalUser, want) {
			t.Fatalf("first model input does not contain %q:\n%s", want, finalUser)
		}
	}
	if strings.Count(finalUser, "# Skill: alpha") != 1 {
		t.Fatalf("duplicate /alpha must load once:\n%s", finalUser)
	}
	if alpha, beta := strings.Index(finalUser, "# Skill: alpha"), strings.Index(finalUser, "# Skill: beta"); alpha < 0 || beta < alpha {
		t.Fatalf("skills are not injected in first-occurrence order:\n%s", finalUser)
	}

	wantVisible := []string{"tool_call:alpha", "tool_result:alpha", "tool_call:beta", "tool_result:beta"}
	if got := visibleSkillEventSequence(eventsAtFirstCall); strings.Join(got, ",") != strings.Join(wantVisible, ",") {
		t.Fatalf("skill events visible at first model call = %#v, want %#v", got, wantVisible)
	}
	assertExplicitSkillToolArgs(t, eventsAtFirstCall, []string{"alpha", "beta"})
	assertPersistedExplicitSkillCards(t, sess.History(), []string{"alpha", "beta"})
}

func TestResolveExplicitSkillInvocationsSupportsInteractiveStoryAndToolPolicy(t *testing.T) {
	skillsDir := t.TempDir()
	writeExplicitSkillFixtureForAgent(t, skillsDir, "scene-tone", "Scene tone", "SCENE_TONE_BODY", config.AgentKindInteractiveStory)
	writeExplicitSkillFixtureForAgent(t, skillsDir, "ide-only", "IDE only", "IDE_ONLY_BODY", config.AgentKindIDE)
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: t.TempDir(), DenovaDir: t.TempDir()}

	resolved, err := novaskills.ResolveConfiguredInvocations(
		context.Background(), cfg, config.AgentKindInteractiveStory,
		"继续故事，并在这里用 /scene-tone；不要加载 /ide-only。",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Name != "scene-tone" || !strings.Contains(resolved[0].Instructions, "SCENE_TONE_BODY") {
		t.Fatalf("interactive story skills = %#v", resolved)
	}

	if cfg.AgentTools.InteractiveStory == nil {
		cfg.AgentTools.InteractiveStory = config.AgentToolOverride{}
	}
	cfg.AgentTools.InteractiveStory[config.AgentToolSkills] = false
	resolved, err = novaskills.ResolveConfiguredInvocations(context.Background(), cfg, config.AgentKindInteractiveStory, "/scene-tone")
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 0 {
		t.Fatalf("disabled Skills must not preload: %#v", resolved)
	}
}

func TestIDEContextAnalysisIncludesTheSameExplicitSkillBodiesAsRuntime(t *testing.T) {
	skillsDir := t.TempDir()
	writeExplicitSkillFixture(t, skillsDir, "alpha", "Alpha instructions", "ALPHA_ANALYSIS_BODY")
	writeExplicitSkillFixture(t, skillsDir, "beta", "Beta instructions", "BETA_ANALYSIS_BODY")
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: t.TempDir(), DenovaDir: t.TempDir()}
	message := "检查 /alpha，然后也检查 /beta。"

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("explicit-skill-analysis")
	if err != nil {
		t.Fatal(err)
	}
	request := agentchat.ChatRequest{Message: message}
	conversation := agentconversation.NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)
	analysis, err := agentchat.BuildIDEContextAnalysis(cfg, nil, prompts.IDEStoryTeller{}, nil, nil, nil, request, conversation)
	if err != nil {
		t.Fatal(err)
	}
	if len(analysis.ContextMessages) == 0 {
		t.Fatal("context analysis has no model-visible messages")
	}
	final := analysis.ContextMessages[len(analysis.ContextMessages)-1].Content
	for _, want := range []string{"# Skill: alpha", "ALPHA_ANALYSIS_BODY", "# Skill: beta", "BETA_ANALYSIS_BODY", message} {
		if !strings.Contains(final, want) {
			t.Fatalf("context analysis does not contain %q:\n%s", want, final)
		}
	}
	var explicitParts int
	for _, part := range analysis.ContextParts {
		if part.Source == "turn.skill.explicit" {
			explicitParts++
		}
	}
	if explicitParts != 2 {
		t.Fatalf("explicit Skill context parts = %d, want 2: %#v", explicitParts, analysis.ContextParts)
	}
}

func TestExplicitSkillFailsClosedWhenTheTurnContextCannotFitIt(t *testing.T) {
	skillsDir := t.TempDir()
	writeExplicitSkillFixture(t, skillsDir, "alpha", "Alpha instructions", strings.Repeat("ALPHA_BODY", 100))
	// Leave enough room for the runtime envelope so the explicit Skill is the
	// fragment that crosses the complete assembled-context ceiling.
	maxTotalBytes := 1024
	cfg := &config.Config{SkillsDir: skillsDir, Workspace: t.TempDir(), DenovaDir: t.TempDir()}
	cfg.AgentContexts.IDE.MaxTotalInjectedBytes = &maxTotalBytes
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess, err := store.GetOrCreate("explicit-skill-budget")
	if err != nil {
		t.Fatal(err)
	}
	conversation := agentconversation.NewSessionConversationForAgent(sess, cfg, config.AgentKindIDE)

	service := NewEphemeralRuntime()
	t.Cleanup(func() { _ = service.Close(context.Background()) })
	outcome := runCycle(service,
		context.Background(),
		newRunControlTestRunner(t, &runControlFixedModel{message: agent.AssistantMessage("must not run", nil)}, false),
		conversation,
		nil,
		agentchat.ChatRequest{CommandID: "explicit-skill-budget", Message: "请使用 /alpha"},
		agentrun.Options{AgentKind: config.AgentKindIDE, RootAgentName: "explicit-skill-budget", Workspace: cfg.Workspace, SessionID: sess.ID},
		nil,
	)
	if outcome.Error == nil || !strings.Contains(outcome.Error.Error(), "context injected bytes exceed limit: source=turn.skill.explicit") {
		t.Fatalf("context budget outcome = %#v", outcome)
	}
}

type explicitSkillEventCollector struct {
	mu     sync.Mutex
	events []agentrun.Event
}

func (c *explicitSkillEventCollector) Append(event agentrun.Event) {
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
}

func (c *explicitSkillEventCollector) Snapshot() []agentrun.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]agentrun.Event(nil), c.events...)
}

type explicitSkillCaptureModel struct {
	mu                sync.Mutex
	events            func() []agentrun.Event
	input             []*agent.Message
	eventsAtFirstCall []agentrun.Event
	calls             int
}

func (m *explicitSkillCaptureModel) Generate(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.Message, error) {
	return m.capture(input), nil
}

func (m *explicitSkillCaptureModel) Stream(_ context.Context, input []*agent.Message, _ ...agent.ModelOption) (*agent.StreamReader[*agent.Message], error) {
	return agent.StreamReaderFromArray([]*agent.Message{m.capture(input)}), nil
}

func (m *explicitSkillCaptureModel) capture(input []*agent.Message) *agent.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.calls == 1 {
		m.input = cloneAgentMessages(input)
		m.eventsAtFirstCall = m.events()
	}
	return agent.AssistantMessage("done", nil)
}

func (m *explicitSkillCaptureModel) Captured() ([]*agent.Message, []agentrun.Event, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return cloneAgentMessages(m.input), append([]agentrun.Event(nil), m.eventsAtFirstCall...), m.calls
}

func cloneAgentMessages(messages []*agent.Message) []*agent.Message {
	cloned := make([]*agent.Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			cloned = append(cloned, nil)
			continue
		}
		cloned = append(cloned, message.Clone())
	}
	return cloned
}

func lastUserMessageContent(messages []*agent.Message) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if messages[index] != nil && messages[index].Role == agent.User {
			return messages[index].Content
		}
	}
	return ""
}

func visibleSkillEventSequence(events []agentrun.Event) []string {
	var sequence []string
	for _, event := range events {
		if event.Type != "tool_call" && event.Type != "tool_result" {
			continue
		}
		_, ok := event.Data.(map[string]interface{})
		if !ok || event.DataString("name") != "skill" {
			continue
		}
		name := ""
		if event.Type == "tool_call" {
			var args struct {
				Name string `json:"name"`
			}
			_ = json.Unmarshal([]byte(event.DataString("args")), &args)
			name = args.Name
		} else {
			content := event.DataString("content")
			name = strings.TrimPrefix(strings.SplitN(content, "\n", 2)[0], "# Skill: ")
		}
		sequence = append(sequence, event.Type+":"+name)
	}
	return sequence
}

func assertExplicitSkillToolArgs(t *testing.T, events []agentrun.Event, want []string) {
	t.Helper()
	var got []string
	for _, event := range events {
		if event.Type != "tool_call" {
			continue
		}
		_, ok := event.Data.(map[string]interface{})
		if !ok || event.DataString("name") != "skill" {
			continue
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(event.DataString("args")), &args); err != nil {
			t.Fatalf("invalid skill tool args: %v", err)
		}
		got = append(got, args.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("skill tool args = %#v, want %#v", got, want)
	}
}

func assertPersistedExplicitSkillCards(t *testing.T, history []session.HistoryEntry, want []string) {
	t.Helper()
	var got []string
	firstUserIndex := -1
	firstSkillIndex := -1
	for index, entry := range history {
		if firstUserIndex < 0 && entry.Role == "user" {
			firstUserIndex = index
		}
		if entry.Role != "tool_call" || entry.Name != "skill" {
			continue
		}
		if firstSkillIndex < 0 {
			firstSkillIndex = index
		}
		var args struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal([]byte(entry.Args), &args); err != nil {
			t.Fatalf("persisted skill args are invalid: %v", err)
		}
		if entry.Status != "success" || !strings.HasPrefix(entry.Result, "# Skill: "+args.Name) {
			t.Fatalf("persisted skill card = %#v", entry)
		}
		got = append(got, args.Name)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("persisted skill cards = %#v, want %#v", got, want)
	}
	if firstUserIndex < 0 || firstSkillIndex <= firstUserIndex {
		t.Fatalf("persisted history must keep the user message before Skill cards: %#v", history)
	}
}

func writeExplicitSkillFixture(t *testing.T, root, name, description, body string) {
	writeExplicitSkillFixtureForAgent(t, root, name, description, body, config.AgentKindIDE)
}

func writeExplicitSkillFixtureForAgent(t *testing.T, root, name, description, body, agentKind string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + name + "\ndescription: " + description + "\nagent: " + agentKind + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, novaskills.SkillFileName), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
