package agent

import (
	"context"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
	sessionfile "github.com/alfredxw/denova/agent/session/file"
)

// forwardingSessionStore is a normal Store decorator. Agent must not select a
// different physical journal merely because the Store's outer Go type changed.
type forwardingSessionStore struct{ agentsession.Store }

type forwardingSessionLog struct{ agentsession.Log }

func (store forwardingSessionStore) Open(ctx context.Context, key agentsession.Key) (agentsession.Log, error) {
	log, err := store.Store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	// Deliberately expose only the public Log interface. Private built-in
	// checkpoint/index acceleration may disappear behind a normal decorator,
	// but the authoritative canonical Log must remain unchanged.
	return forwardingSessionLog{Log: log}, nil
}

func TestBuiltInFileStoreDecoratorDoesNotForkDurableSessionLane(t *testing.T) {
	tests := []struct {
		name       string
		firstStore func(*sessionfile.Store) agentsession.Store
		coldStore  func(*sessionfile.Store) agentsession.Store
	}{
		{
			name:       "direct_to_decorator",
			firstStore: func(store *sessionfile.Store) agentsession.Store { return store },
			coldStore:  func(store *sessionfile.Store) agentsession.Store { return forwardingSessionStore{Store: store} },
		},
		{
			name:       "decorator_to_direct",
			firstStore: func(store *sessionfile.Store) agentsession.Store { return forwardingSessionStore{Store: store} },
			coldStore:  func(store *sessionfile.Store) agentsession.Store { return store },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, err := sessionfile.NewWithOptions(t.TempDir(), sessionfile.Options{CheckpointTailRecords: 1})
			if err != nil {
				t.Fatal(err)
			}
			definition := func(model BaseChatModel) Definition {
				return Definition{
					Key: "file-store-decorator", Model: model,
					ModelIdentity: CapabilityIdentity{Kind: "model.file-store-decorator-test", Version: 1},
				}
			}
			firstModel := &lifecycleModel{responses: []*Message{AssistantMessage("persisted", nil)}}
			firstOwner, err := New(context.Background(), definition(firstModel), WithSessionStore(test.firstStore(store)))
			if err != nil {
				t.Fatal(err)
			}
			firstSession, err := firstOwner.Session(context.Background(), NamedSession("decorated-file-store"))
			if err != nil {
				t.Fatal(err)
			}
			firstRun, err := firstSession.Run(context.Background(), Input{
				Text: "persist this exact command", IdempotencyKey: "decorated-file-store-command",
			})
			if err != nil {
				t.Fatal(err)
			}
			if result, waitErr := firstRun.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
				t.Fatalf("first result=%#v error=%v", result, waitErr)
			}
			if err := firstOwner.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			firstOwner = nil

			coldModel := &lifecycleModel{}
			coldOwner, err := New(context.Background(), definition(coldModel), WithSessionStore(test.coldStore(store)))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = coldOwner.Close(context.Background()) })
			coldSession, err := coldOwner.Session(context.Background(), NamedSession("decorated-file-store"))
			if err != nil {
				t.Fatal(err)
			}
			replayed, err := coldSession.Run(context.Background(), Input{
				Text: "persist this exact command", IdempotencyKey: "decorated-file-store-command",
			})
			if err != nil {
				t.Fatal(err)
			}
			if !replayed.Replayed() {
				t.Fatal("cold owner did not replay the durable command receipt")
			}
			if result, waitErr := replayed.Wait(context.Background()); waitErr != nil || result.Status != ResultCompleted {
				t.Fatalf("replayed result=%#v error=%v", result, waitErr)
			}
			if calls := coldModel.calls(); len(calls) != 0 {
				t.Fatalf("cold replay called the model %d times", len(calls))
			}
			if err := coldOwner.Close(context.Background()); err != nil {
				t.Fatal(err)
			}
			coldOwner = nil
		})
	}
}
