package tools

import (
	"context"
	"sync"
	"testing"

	"denova/internal/configresources"
)

func TestNarrativeStyleConcurrentUpdateAndDeleteHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	adapter := newNarrativeStyleResource(t.TempDir())
	created := applyConfigMutationForTest(t, adapter, configresources.Mutation{
		Operation: configresources.ApplyCreate,
		Resource:  "narrative_style",
		Value: map[string]any{
			"id": "cas-narrative", "name": "Original",
			"slots": []any{map[string]any{"id": "identity", "name": "Identity", "target": "system", "enabled": true, "content": "Original."}},
		},
	})

	start := make(chan struct{})
	errorsByOperation := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	go func() {
		ready.Done()
		<-start
		_, err := adapter.Apply(ctx, configresources.Mutation{
			Operation: configresources.ApplyUpdate, Resource: "narrative_style", ID: created.ID, Revision: created.Revision,
			Value: map[string]any{
				"id": created.ID, "name": "Updated",
				"slots": []any{map[string]any{"id": "identity", "name": "Identity", "target": "system", "enabled": true, "content": "Updated."}},
			},
		})
		errorsByOperation <- err
	}()
	go func() {
		ready.Done()
		<-start
		_, err := adapter.Apply(ctx, configresources.Mutation{
			Operation: configresources.ApplyDelete, Resource: "narrative_style", ID: created.ID, Revision: created.Revision,
		})
		errorsByOperation <- err
	}()
	ready.Wait()
	close(start)

	assertOneConfigMutationWinner(t, errorsByOperation)
}

func TestImagePresetConcurrentCASUpdatesHaveOneWinner(t *testing.T) {
	ctx := context.Background()
	adapter := newImagePresetResource(t.TempDir())
	created := applyConfigMutationForTest(t, adapter, configresources.Mutation{
		Operation: configresources.ApplyCreate, Resource: "image_preset",
		Value: map[string]any{"id": "cas-image", "name": "Original", "prompt": "original light"},
	})

	start := make(chan struct{})
	errorsByOperation := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, prompt := range []string{"warm light", "cold light"} {
		prompt := prompt
		go func() {
			ready.Done()
			<-start
			_, err := adapter.Apply(ctx, configresources.Mutation{
				Operation: configresources.ApplyUpdate, Resource: "image_preset", ID: created.ID, Revision: created.Revision,
				Value: map[string]any{"id": created.ID, "name": prompt, "prompt": prompt},
			})
			errorsByOperation <- err
		}()
	}
	ready.Wait()
	close(start)

	assertOneConfigMutationWinner(t, errorsByOperation)
}

func applyConfigMutationForTest(t *testing.T, adapter configresources.Adapter, mutation configresources.Mutation) configMutationReceipt {
	t.Helper()
	value, err := adapter.Apply(context.Background(), mutation)
	if err != nil {
		t.Fatal(err)
	}
	receipt, ok := value.(configMutationReceipt)
	if !ok {
		t.Fatalf("mutation receipt type = %T", value)
	}
	if receipt.ID == "" || receipt.Revision == "" {
		t.Fatalf("mutation receipt = %#v", receipt)
	}
	return receipt
}

func assertOneConfigMutationWinner(t *testing.T, errorsByOperation <-chan error) {
	t.Helper()
	successes := 0
	for count := 0; count < 2; count++ {
		if err := <-errorsByOperation; err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CAS mutations committed %d times, want exactly 1", successes)
	}
}
