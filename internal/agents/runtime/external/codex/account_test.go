package codex

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"denova/internal/agents/runtime/external"
)

func TestAccountCheckUsesLocalCredentials(t *testing.T) {
	for _, test := range []struct{ name, response, status string }{
		{"signed out", `{"account":null,"requiresOpenaiAuth":true}`, "auth_required"},
		{"CLI login", `{"account":{"type":"chatgpt"},"requiresOpenaiAuth":true}`, "ready"},
		{"API key", `{"account":{"type":"apiKey"},"requiresOpenaiAuth":true}`, "ready"},
		{"custom provider", `{"account":null,"requiresOpenaiAuth":false}`, "ready"},
		{"unsupported account", `{"account":{"type":"unknown"},"requiresOpenaiAuth":true}`, "incompatible"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newProtocolFixture(t)
			done := make(chan error, 1)
			go func() {
				defer func() {
					if value := recover(); value != nil {
						done <- fmt.Errorf("account fixture panic: %v", value)
					}
				}()
				state, err := fixture.client.Check(t.Context())
				if err == nil && state.Status != test.status {
					err = fmt.Errorf("status=%s, want %s", state.Status, test.status)
				}
				done <- err
			}()
			msg := fixture.receive(t)
			if msg.Method != "account/read" || string(msg.Params) != `{"refreshToken":false}` {
				t.Fatalf("unexpected account probe: %+v", msg)
			}
			if err := json.NewEncoder(fixture.server).Encode(packet{ID: msg.ID, Result: json.RawMessage(test.response)}); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if state := fixture.client.Status(); state.Status != test.status {
				t.Fatalf("cached state=%#v", state)
			}
		})
	}
}

func TestAccountProtocolModelsAndNotifications(t *testing.T) {
	fixture := newProtocolFixture(t)
	done := make(chan error, 1)
	go func() {
		defer func() {
			if value := recover(); value != nil {
				done <- fmt.Errorf("model fixture panic: %v", value)
			}
		}()
		models, err := fixture.client.Models(t.Context())
		want := external.Models{Items: []external.Model{{ID: "executable-slug", DisplayName: "Fixture model", Efforts: []string{"high"}, DefaultEffort: "high"}}, DefaultID: "executable-slug"}
		if err == nil && !reflect.DeepEqual(models, want) {
			err = fmt.Errorf("models=%#v, want %#v", models, want)
		}
		done <- err
	}()
	msg := fixture.receive(t)
	if msg.Method != "model/list" {
		t.Fatal(msg.Method)
	}
	body := `{"data":[{"id":"picker-id","model":"executable-slug","displayName":"Fixture model","isDefault":true,"hidden":false,"defaultReasoningEffort":"high","supportedReasoningEfforts":[{"reasoningEffort":"high"}]}],"nextCursor":null}`
	if err := json.NewEncoder(fixture.server).Encode(packet{ID: msg.ID, Result: json.RawMessage(body)}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	for _, notification := range []struct{ body, status string }{
		{`{"authMode":"chatgpt"}`, "ready"},
		{`{"authMode":null}`, "auth_required"},
	} {
		fixture.client.dispatch(packet{Method: "account/updated", Params: json.RawMessage(notification.body)})
		if state := fixture.client.Status(); state.Status != notification.status {
			t.Fatalf("notification state=%#v", state)
		}
	}
}
