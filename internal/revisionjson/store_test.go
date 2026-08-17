package revisionjson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"denova/internal/revisionfile"
)

type testValue struct {
	Name string `json:"name"`
}

func testStore(path string) Store[testValue] {
	return NewStore(path, Codec[testValue]{
		Decode: func(data []byte) (testValue, error) {
			var value testValue
			return value, json.Unmarshal(data, &value)
		},
		Encode: func(value testValue) ([]byte, error) {
			if value.Name == "encode-error" {
				return nil, errors.New("injected encode failure")
			}
			data, err := json.Marshal(value)
			return append(data, '\n'), err
		},
	})
}

func TestStoreRequiresContentRevisionAndPreservesOldBytesOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resource.json")
	store := testStore(path)
	created, err := store.Create(context.Background(), testValue{Name: "base"})
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != revisionfile.Revision([]byte("{\"name\":\"base\"}\n")) {
		t.Fatalf("created revision = %q", created.Revision)
	}
	if _, err := store.Update(context.Background(), "", func(testValue) (testValue, error) {
		return testValue{Name: "blind"}, nil
	}); !errors.Is(err, ErrRevisionRequired) {
		t.Fatalf("missing revision error = %v", err)
	}
	if _, err := store.Update(context.Background(), created.Revision, func(testValue) (testValue, error) {
		return testValue{Name: "encode-error"}, nil
	}); err == nil {
		t.Fatal("expected injected encode failure")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\"name\":\"base\"}\n" {
		t.Fatalf("failed update changed persisted bytes: %q", data)
	}
}

func TestStoreAllowsOnlyOneConcurrentUpdatePerRevision(t *testing.T) {
	store := testStore(filepath.Join(t.TempDir(), "resource.json"))
	created, err := store.Create(context.Background(), testValue{Name: "base"})
	if err != nil {
		t.Fatal(err)
	}
	errs := make([]error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for index := range errs {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					errs[index] = fmt.Errorf("revision JSON update panic: %v", recovered)
				}
			}()
			<-start
			_, errs[index] = store.Update(context.Background(), created.Revision, func(testValue) (testValue, error) {
				return testValue{Name: fmt.Sprintf("writer-%d", index)}, nil
			})
		}(index)
	}
	close(start)
	wait.Wait()

	succeeded, conflicted := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, revisionfile.ErrRevisionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent update error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent results: succeeded=%d conflicted=%d errors=%v", succeeded, conflicted, errs)
	}
}
