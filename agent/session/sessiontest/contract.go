// Package sessiontest provides the reusable behavioral contract for external
// session.Store adapters.
package sessiontest

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/alfredxw/denova/agent/session"
)

type Factory func(testing.TB) session.Store

// RunStoreContract verifies the complete observable Store interface. Adapter
// authors should call it from their own test suite with a fresh Store factory.
func RunStoreContract(t *testing.T, factory Factory) {
	t.Helper()
	t.Run("append_replay_and_cas", func(t *testing.T) {
		store := factory(t)
		key := session.Key{Namespace: "contract", ID: "append"}
		log, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = log.Close() })

		first := session.Record{Kind: "contract.first", Version: 1, Data: json.RawMessage(`{"value":"one"}`)}
		second := session.Record{Kind: "contract.second", Version: 2, Data: json.RawMessage(`{"value":"two"}`)}
		revision, err := log.Append(context.Background(), 0, first, second)
		if err != nil || revision != 2 {
			t.Fatalf("append revision=%d error=%v", revision, err)
		}
		first.Data[0] = '['

		var got []session.Record
		stats, err := log.Replay(context.Background(), func(record session.Record) error {
			got = append(got, record)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if stats.RecordsRead != 2 || len(got) != 2 || got[0].Revision != 1 || got[1].Revision != 2 ||
			got[0].Kind != "contract.first" || string(got[0].Data) != `{"value":"one"}` {
			t.Fatalf("replay stats=%#v records=%#v", stats, got)
		}
		if _, err := log.Append(context.Background(), 1, session.Record{
			Kind: "contract.stale", Version: 1, Data: json.RawMessage(`null`),
		}); !errors.Is(err, session.ErrRevisionConflict) {
			t.Fatalf("stale append error=%v", err)
		}
	})

	t.Run("batch_validation_is_atomic", func(t *testing.T) {
		store := factory(t)
		log, err := store.Open(context.Background(), session.Key{Namespace: "contract", ID: "atomic"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = log.Close() })
		_, err = log.Append(context.Background(), 0,
			session.Record{Kind: "contract.valid", Version: 1, Data: json.RawMessage(`null`)},
			session.Record{Kind: "contract.invalid", Version: 0, Data: json.RawMessage(`null`)},
		)
		if !errors.Is(err, session.ErrInvalidRecord) {
			t.Fatalf("invalid batch error=%v", err)
		}
		stats, err := log.Replay(context.Background(), func(session.Record) error { return nil })
		if err != nil || stats.RecordsRead != 0 {
			t.Fatalf("invalid batch committed stats=%#v error=%v", stats, err)
		}
	})

	t.Run("committed_batch_survives_fresh_open", func(t *testing.T) {
		store := factory(t)
		key := session.Key{Namespace: "contract", ID: "fresh-open"}
		first, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		revision, err := first.Append(context.Background(), 0,
			session.Record{Kind: "contract.fresh.first", Version: 1, Data: json.RawMessage(`{"value":1}`)},
			session.Record{Kind: "contract.fresh.second", Version: 1, Data: json.RawMessage(`{"value":2}`)},
		)
		if err != nil || revision != 2 {
			t.Fatalf("append revision=%d error=%v", revision, err)
		}
		if err := first.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		var records []session.Record
		stats, err := reopened.Replay(context.Background(), func(record session.Record) error {
			records = append(records, record)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if stats.RecordsRead != 2 || len(records) != 2 ||
			records[0].Revision != 1 || records[0].Kind != "contract.fresh.first" || string(records[0].Data) != `{"value":1}` ||
			records[1].Revision != 2 || records[1].Kind != "contract.fresh.second" || string(records[1].Data) != `{"value":2}` {
			t.Fatalf("fresh replay stats=%#v records=%#v", stats, records)
		}
	})

	t.Run("exclusive_lease_and_close", func(t *testing.T) {
		store := factory(t)
		key := session.Key{Namespace: "contract", ID: "lease"}
		first, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := store.Open(ctx, key); !errors.Is(err, context.Canceled) {
			t.Fatalf("contended open error=%v", err)
		}
		if err := first.Close(); err != nil {
			t.Fatal(err)
		}
		reopened, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("cancellation_is_definite_no_commit", func(t *testing.T) {
		store := factory(t)
		log, err := store.Open(context.Background(), session.Key{Namespace: "contract", ID: "cancel"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = log.Close() })
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := log.Append(ctx, 0, session.Record{
			Kind: "contract.cancelled", Version: 1, Data: json.RawMessage(`null`),
		}); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled append error=%v", err)
		}
		stats, err := log.Replay(context.Background(), func(session.Record) error { return nil })
		if err != nil || stats.RecordsRead != 0 {
			t.Fatalf("cancelled append committed stats=%#v error=%v", stats, err)
		}
	})

	t.Run("catalog_list_and_idempotent_delete", func(t *testing.T) {
		store := factory(t)
		firstKey := session.Key{Namespace: "contract", ID: "catalog-a", Attributes: map[string]string{"project": "one"}}
		secondKey := session.Key{Namespace: "contract", ID: "catalog-b", Attributes: map[string]string{"project": "two"}}
		for _, key := range []session.Key{firstKey, secondKey} {
			log, err := store.Open(context.Background(), key)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := log.Append(context.Background(), 0, session.Record{
				Kind: "contract.catalog", Version: 1, Data: json.RawMessage(`{"value":true}`),
			}); err != nil {
				t.Fatal(err)
			}
			if err := log.Close(); err != nil {
				t.Fatal(err)
			}
		}
		listed, err := store.List(context.Background(), session.Selector{
			Namespace: "contract", Attributes: map[string]string{"project": "one"},
		})
		if err != nil || len(listed) != 1 {
			t.Fatalf("listed=%#v error=%v", listed, err)
		}
		canonical, _ := session.CanonicalKey(listed[0])
		want, _ := session.CanonicalKey(firstKey)
		if canonical != want {
			t.Fatalf("listed key=%s want=%s", canonical, want)
		}
		if err := store.Delete(context.Background(), firstKey); err != nil {
			t.Fatal(err)
		}
		if err := store.Delete(context.Background(), firstKey); err != nil {
			t.Fatalf("idempotent Delete error=%v", err)
		}
		listed, err = store.List(context.Background(), session.Selector{Namespace: "contract"})
		if err != nil || len(listed) != 1 || listed[0].ID != secondKey.ID {
			t.Fatalf("remaining=%#v error=%v", listed, err)
		}
		reopened, err := store.Open(context.Background(), firstKey)
		if err != nil {
			t.Fatal(err)
		}
		defer reopened.Close()
		stats, err := reopened.Replay(context.Background(), func(session.Record) error { return nil })
		if err != nil || stats.RecordsRead != 0 {
			t.Fatalf("deleted Session replay stats=%#v error=%v", stats, err)
		}
	})

	t.Run("delete_waits_for_exclusive_lease", func(t *testing.T) {
		store := factory(t)
		key := session.Key{Namespace: "contract", ID: "delete-lease"}
		log, err := store.Open(context.Background(), key)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := store.Delete(ctx, key); !errors.Is(err, context.Canceled) {
			t.Fatalf("contended Delete error=%v", err)
		}
		if err := log.Close(); err != nil {
			t.Fatal(err)
		}
		if err := store.Delete(context.Background(), key); err != nil {
			t.Fatal(err)
		}
	})
}
