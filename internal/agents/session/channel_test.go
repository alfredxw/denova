package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionChannelPersistsInMetadataAndReplay(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	seed := testRuntimeConfig("ide")
	created, err := store.CreateWithRuntimeConfig("Configuration conversation", seed, ChannelConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	if created.Channel != ChannelConfiguration {
		t.Fatalf("created channel = %q, want %q", created.Channel, ChannelConfiguration)
	}
	metas, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Channel != ChannelConfiguration {
		t.Fatalf("session metadata = %#v", metas)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	replayed, err := reopened.Get(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.Channel != ChannelConfiguration {
		t.Fatalf("replayed channel = %q, want %q", replayed.Channel, ChannelConfiguration)
	}
}

func TestGetOrCreateWithRuntimeConfigRejectsChannelMismatch(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	seed := testRuntimeConfig("ide")
	created, err := store.GetOrCreateWithRuntimeConfig("shared-id", seed, ChannelConfiguration)
	if err != nil {
		t.Fatal(err)
	}
	if created.Channel != ChannelConfiguration {
		t.Fatalf("created channel = %q", created.Channel)
	}
	if _, err := store.GetOrCreateWithRuntimeConfig("shared-id", seed, ChannelAgent); err == nil {
		t.Fatal("reopening a configuration session through the ordinary channel should fail")
	}
}

func TestSessionChannelDefaultsLegacyJournalToAgent(t *testing.T) {
	dir := t.TempDir()
	legacy := []byte("{\"type\":\"session\",\"id\":\"legacy\",\"created_at\":\"2026-01-01T00:00:00Z\"}\n")
	if err := os.WriteFile(filepath.Join(dir, "legacy.jsonl"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	sess, err := store.Get("legacy")
	if err != nil {
		t.Fatal(err)
	}
	if sess.Channel != ChannelAgent {
		t.Fatalf("legacy channel = %q, want %q", sess.Channel, ChannelAgent)
	}
	metas, err := store.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 || metas[0].Channel != ChannelAgent {
		t.Fatalf("legacy metadata = %#v", metas)
	}
}

func TestParseOptionalChannelDistinguishesOmissionFromAgent(t *testing.T) {
	omitted, err := ParseOptionalChannel(" ")
	if err != nil || omitted != "" {
		t.Fatalf("omitted channel = %q, err=%v", omitted, err)
	}
	agentChannel, err := ParseOptionalChannel("agent")
	if err != nil || agentChannel != ChannelAgent {
		t.Fatalf("agent channel = %q, err=%v", agentChannel, err)
	}
	if _, err := ParseOptionalChannel("unknown"); err == nil {
		t.Fatal("unsupported channel should fail")
	}
}
