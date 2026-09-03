package agent

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	agentsession "github.com/alfredxw/denova/agent/session"
)

type scopedDeleteStore struct {
	agentsession.Store

	mu      sync.Mutex
	deleted []agentsession.Key
}

func (store *scopedDeleteStore) List(ctx context.Context, selector agentsession.Selector) ([]agentsession.Key, error) {
	if selector.All {
		return nil, errors.New("unexpected global Agent Session catalog scan")
	}
	return store.Store.List(ctx, selector)
}

func (store *scopedDeleteStore) Delete(ctx context.Context, key agentsession.Key) error {
	if err := store.Store.Delete(ctx, key); err != nil {
		return err
	}
	store.mu.Lock()
	store.deleted = append(store.deleted, key)
	store.mu.Unlock()
	return nil
}

func TestDeleteSessionsDiscoversDescendantsWithoutGlobalCatalogScan(t *testing.T) {
	ctx := context.Background()
	base := agentsession.Memory()
	store := &scopedDeleteStore{Store: base}
	root := NamedSession("root")
	childAttributes, err := ChildSessionAttributes(root)
	if err != nil {
		t.Fatal(err)
	}
	childAttributes["agent"] = "researcher"
	child := SessionKey{Namespace: "task.researcher", ID: "child", Attributes: childAttributes}
	grandchildAttributes, err := ChildSessionAttributes(child)
	if err != nil {
		t.Fatal(err)
	}
	grandchildAttributes["agent"] = "critic"
	grandchild := SessionKey{Namespace: "task.critic", ID: "grandchild", Attributes: grandchildAttributes}
	unrelated := NamedSession("unrelated")

	for _, key := range []SessionKey{root, child, grandchild, unrelated} {
		log, openErr := base.Open(ctx, key)
		if openErr != nil {
			t.Fatalf("open %#v: %v", key, openErr)
		}
		if closeErr := log.Close(); closeErr != nil {
			t.Fatalf("close %#v: %v", key, closeErr)
		}
	}

	owner, err := New(ctx, Definition{Name: "delete-tree", Model: &lifecycleModel{}}, WithSessionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close(ctx)
	if err := owner.DeleteSessions(ctx, SessionSelector{Namespace: root.Namespace, ID: root.ID}); err != nil {
		t.Fatal(err)
	}

	remaining, err := base.List(ctx, agentsession.Selector{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(remaining, []agentsession.Key{unrelated}) {
		t.Fatalf("remaining Sessions = %#v, want only %#v", remaining, unrelated)
	}
	store.mu.Lock()
	deleted := append([]agentsession.Key(nil), store.deleted...)
	store.mu.Unlock()
	if !reflect.DeepEqual(deleted, []agentsession.Key{grandchild, child, root}) {
		t.Fatalf("deletion order = %#v", deleted)
	}
}
