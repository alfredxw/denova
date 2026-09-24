package platform

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"denova/internal/agents/canonicalstore"
	agent "github.com/alfredxw/denova/agent"
)

func TestPrivateGameAgentUsesSelectedPluginWithWriteGrant(t *testing.T) {
	for _, grantWrite := range []bool{false, true} {
		name := "read-only"
		if grantWrite {
			name = "authorized-write"
		}
		t.Run(name, func(t *testing.T) {
			m, projectID := testManager(t)
			plugin := testSource(t, m, projectID, "tools", "tool", "test.shared-tools", Plugin)
			_, directory, err := m.Development(plugin.ID)
			if err != nil {
				t.Fatal(err)
			}
			var tool ToolDefinition
			if err := readJSON(filepath.Join(directory, "probe.json"), &tool); err != nil {
				t.Fatal(err)
			}
			tool.Effect = "write"
			if err := writeJSON(filepath.Join(directory, "probe.json"), tool); err != nil {
				t.Fatal(err)
			}
			candidate, err := m.CheckDevelopment(plugin.ID)
			if err != nil {
				t.Fatal(err)
			}
			testInstall(t, m, candidate)
			game := testSource(t, m, projectID, "game", "agent", "test.npc-plugin", Game)
			_, directory, err = m.Development(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			var manifest Manifest
			if err := readJSON(filepath.Join(directory, Game.manifestFile()), &manifest); err != nil {
				t.Fatal(err)
			}
			manifest.Requires = []Dependency{{PluginID: "test.shared-tools", VersionRange: "=1.0.0", Contributions: []string{"probes"}}}
			manifest.Game.Uses.Toolsets = []string{"test.shared-tools/probes"}
			manifest.Permissions.Required = append(manifest.Permissions.Required, "tools.invoke")
			if grantWrite {
				manifest.Permissions.Required = append(manifest.Permissions.Required, "tools.write")
			}
			if err := writeJSON(filepath.Join(directory, Game.manifestFile()), manifest); err != nil {
				t.Fatal(err)
			}
			definition := AgentDefinition{Instructions: "Use the selected tool and return its result.", ModelSlot: "writer", Toolsets: []string{"test.shared-tools/probes"}}
			if err := writeJSON(filepath.Join(directory, "character.json"), definition); err != nil {
				t.Fatal(err)
			}
			candidate, err = m.CheckDevelopment(game.ID)
			if err != nil {
				t.Fatal(err)
			}
			release := testInstall(t, m, candidate)
			store, err := canonicalstore.New(m.root, m.registry)
			if err != nil {
				t.Fatal(err)
			}
			m.ConfigureAgents(store, func(context.Context, string) (agent.BaseChatModel, agent.CapabilityIdentity, error) {
				return pluginCallingModel{}, agent.CapabilityIdentity{Kind: "test.plugin_model", Version: 1}, nil
			})
			instance, err := m.CreateInstance(CreateInstance{GameID: release.Manifest.ID, ReleaseID: release.Ref.ReleaseID, Title: name, ProjectID: projectID, Models: map[string]string{"local:writer": "test"}})
			if err != nil {
				t.Fatal(err)
			}
			opened, err := m.OpenInstance(context.Background(), instance.ID, OpenOptions{ParentOrigin: "http://127.0.0.1:15173"})
			if err != nil {
				t.Fatal(err)
			}
			status, data := testRequest(t, opened.Connection, "POST", "/agents/sessions", "", EnsureAgentSession{ProjectID: projectID, Definition: "local:character", Key: "npc"})
			if status != 201 {
				t.Fatalf("session: %d %s", status, data)
			}
			var session AgentSession
			if err := json.Unmarshal(data, &session); err != nil {
				t.Fatal(err)
			}
			status, data = testRequest(t, opened.Connection, "POST", "/agents/sessions/"+session.Ref.SessionID+"/runs", "", map[string]any{"commandId": "turn-1", "input": map[string]string{"text": "Use the tool."}})
			if status != 202 {
				t.Fatalf("run: %d %s", status, data)
			}
			var result RunResult
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				_, data = testRequest(t, opened.Connection, "GET", "/agents/runs/"+result.Run.RunID, "", nil)
				if err := json.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
				if result.Status != "accepted" && result.Status != "running" {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			used := strings.Contains(result.Text, `"value":3`)
			if result.Status != "completed" || used != grantWrite {
				t.Fatalf("permission result: %s", data)
			}
			status, data = testRequest(t, opened.Connection, "POST", "/tools/test.shared-tools/probe/invoke", "", map[string]any{"input": map[string]string{"text": "abc"}})
			if grantWrite && status != 200 || !grantWrite && status != 403 {
				t.Fatalf("direct invocation permission: %d %s", status, data)
			}
		})
	}
}
